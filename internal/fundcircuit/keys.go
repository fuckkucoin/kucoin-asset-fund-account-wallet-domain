package fundcircuit

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wdguard"
)

const (
	// Prefix 总线键空间。任何熔断、案件、探针键都必须带此前缀。
	Prefix = "fc:"
	// KeyGlobal 全局作用域键。
	KeyGlobal = Prefix + "global"
	// segUser / segAsset / segChain 作用域段名。
	segUser = "user"
	segAsset = "asset"
	segChain = "chain"
	// HopKeyPrefix 划转速度来源键前缀。划转域原本没有速度键，这里只作来源标注。
	HopKeyPrefix = "hop:"
)

// ScopeKey 拼作用域键。user/asset/chain 必须给目标；global 忽略目标。
// 功能：把用户、链、资产收敛到唯一 fc: 键，供熔断表、案件与探针共用。
// 重点：用户标识不允许竖线，避免与链、资产粘连；资产链统一大写。
// 风险点：作用域拼错会写出新键位，值班须 /ops/fundcircuit/circuits 核查。
  func ScopeKey(scope Scope, user, chain, asset string) (string, error) {
	   switch scope {
	   case ScopeGlobal, "":
		   return KeyGlobal, nil
	   case ScopeUser:
		   uid, err := validate.UserID(user)
		  if err != nil {
			  return "", ErrTarget
		  }
		  if strings.Contains(uid, "|") {
			  return "", ErrTarget
		   }
		   return Prefix + segUser + "|" + uid, nil
	  case ScopeAsset:
		   as, err := validate.Asset(asset)
		  if err != nil {
			   return "", ErrTarget
		  }
		   return Prefix + segAsset + "|" + as, nil
	   case ScopeChain:
		   ch, err := validate.Chain(chain)
		  if err != nil {
			  return "", ErrTarget
		  }
		   return Prefix + segChain + "|" + ch, nil
	   }
	   return "", ErrScope
   }

// IsKey 只认 fc: 前缀。dep:/wd: 等原域键一律否。
  func IsKey(key string) bool {
	   return strings.HasPrefix(strings.TrimSpace(key), Prefix)
   }

// GuardKey 写入前校验键位。非 fc: 键拒绝，避免总线反写入金/出金速度键。

   func GuardKey(key string) error {
	   key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrKeyPrefix
	   }
	  if usage.IsDepKey(key) || strings.HasPrefix(key, wdguard.AbuseKeyPrefix) || strings.HasPrefix(key, HopKeyPrefix) {
		   return fmt.Errorf("%w: %q", ErrForeignKey, key)
	  }
	  if !IsKey(key) {
		   return fmt.Errorf("%w: %q", ErrKeyPrefix, key)
	   }
	   return nil
  }

// ParseKey 拆回作用域与目标。非 fc: 键失败。
   func ParseKey(key string) (Scope, string, string, string, error) {
	  key = strings.TrimSpace(key)
	  if !IsKey(key) {
		  return "", "", "", "", ErrKeyPrefix
	   }
	   rest := strings.TrimPrefix(key,
		  Prefix)
	  if rest == "global" {
		  return ScopeGlobal, "", "", "", nil
	  }
	   parts := strings.SplitN(rest, "|", 2)
	   if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		  return "", "", "", "", ErrKeyPrefix
	  }
	   target := strings.TrimSpace(parts[1])
	   switch parts[0] {
	  case segUser:
		   return ScopeUser, target, "", "", nil
	   case segAsset:
		   return ScopeAsset, "", "", strings.ToUpper(target), nil
	   case segChain:
		  return ScopeChain, "", strings.ToUpper(target), "", nil
	   }
	   return "", "", "", "", ErrScope
  }

// KeysFor 一条信号命中的全部作用域键。缺目标的作用域自动跳过，global 恒在。
// 功能：跨入金/出金/划转的联动靠共用键实现——同一用户不同入口的信号汇到同一 fc:user 键。
// 重点：返回顺序为 user→asset→chain→global，闸门按此顺序先窄后宽求解。
// 风险点：用户为空时只会落到资产/链/全局键，个体刷量可能被稀释，故入口闸要求带用户。

   func KeysFor(sig AbuseSignal) []string {
	   out := make([]string, 0, 4)
	  if k, err := ScopeKey(ScopeUser, sig.UserID, "", ""); err == nil {
		  out = append(out, k)
	   }
	  if k, err := ScopeKey(ScopeAsset, "", "", sig.Asset); err == nil {
		   out = append(out, k)
	   }
	   if k, err := ScopeKey(ScopeChain, "", sig.Chain, ""); err == nil {
		  out = append(out, k)
	  }
	  out = append(out, KeyGlobal)
	   return out
   }

// KeysForReq 入口闸求解顺序。与 KeysFor 同构，入参来自网关请求。
  func KeysForReq(req GateReq) []string {
	  return KeysFor(AbuseSignal{UserID: req.UserID, Chain: req.Chain, Asset: req.Asset})
   }

// BridgeDep 桥接入金速度键。键形由 usage 约束为 dep:{user}|{chain}|{asset}，本函数不改写该键。
// 功能：把 r1 的入金刷充键接成总线信号，跨路径联动才有入金这一侧的输入。
// 重点：SourceKey 保留原键原文；权重缺省取 dep 基准权重。
// 风险点：把非 dep: 键喂进来会返回 ErrBridgeKey，调用方不得当成放行。
   func BridgeDep(key string, weight int, note string) (AbuseSignal, error) {
	   user, chain, asset, err := usage.ParseDepKey(key)
	   if err != nil {
		   return AbuseSignal{}, fmt.Errorf("%w: %v", ErrBridgeKey, err)
	   }
	  if weight <= 0 {
		   weight = SigDepVelocity.BaseWeight()
	   }
	   return AbuseSignal{
		  Kind: SigDepVelocity,
		   Path: PathDepositObserve, UserID: user,
		   Chain: strings.ToUpper(chain), Asset: strings.ToUpper(asset),
		   Weight: weight, SourceKey: strings.TrimSpace(key), Note: note,
	   }, nil
  }

// BridgeWithdraw 桥接出金刷提键。键形由 wdguard 约束为 wd:{user}:{asset}，本函数不改写该键。
  func BridgeWithdraw(key string, weight int, note string) (AbuseSignal, error) {
	   if err := wdguard.RequireAbuseKey(key); err != nil {
		   return AbuseSignal{}, fmt.Errorf("%w: %v", ErrBridgeKey, err)
	   }
	  rest := strings.TrimPrefix(strings.TrimSpace(key), wdguard.AbuseKeyPrefix)
	  parts := strings.SplitN(rest, ":", 2)
	  user := strings.TrimSpace(parts[0])
	   asset := strings.ToUpper(strings.TrimSpace(parts[1]))
	  if weight <= 0 {
		  weight = SigWdVelocity.BaseWeight()
	  }
	  return AbuseSignal{
		  Kind: SigWdVelocity, Path: PathWithdrawRequest, UserID: user, Asset: asset,
		  Weight: weight,
		   SourceKey: strings.TrimSpace(key), Note: note,
	   }, nil
   }

// HopSourceKey 划转来源键。形如 hop:{user}|{asset}，只作信号标注，不落占用表。

   func HopSourceKey(user, asset string) (string, error) {
	  uid, err := validate.UserID(user)
	  if err != nil {
		   return "", ErrTarget
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return "", ErrTarget
	   }
	  return HopKeyPrefix + uid + "|" + as, nil
   }

// BridgeHop 桥接划转 hop 信号。划转域没有历史速度键，这里生成来源标注键。
  func BridgeHop(user, asset string, weight int, note string) (AbuseSignal, error) {
	  src, err := HopSourceKey(user, asset)
	  if err != nil {
		  return AbuseSignal{}, fmt.Errorf("%w: hop", ErrBridgeKey)
	   }
	  if weight <= 0 {
		   weight = SigHopVelocity.BaseWeight()
	   }
	   return AbuseSignal{
		   Kind: SigHopVelocity, Path: PathTransferHop, UserID: strings.TrimSpace(user),
		  Asset: strings.ToUpper(strings.TrimSpace(asset)), Weight: weight,
		  SourceKey: src, Note: note,
	   }, nil
   }

// SourceUntouched 说明桥接不改写原域键：给读面与回归夹具一个显式断言点。
// 原键仍由 usage / wdguard 各自维护，总线只读取键形并生成 fc: 键。

   func SourceUntouched(key string) bool {
	  key = strings.TrimSpace(key)
	   if key == "" {
		   return false
	   }
	  return !IsKey(key)
   }
