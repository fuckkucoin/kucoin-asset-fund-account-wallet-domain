// 本进程 KYT 规则：黑名单和金额档。
// 作者: Likeo
// 日期: 2020.11.30
package kyt

import (
	"context"
	"fmt"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// Rules 确定性筛查。黑名单和金额档先于 mode 兜底；mode=deny 是硬闸，不被 review 档盖掉。
type Rules struct {
	Mode Decision
	DenyAddress map[string]struct{}
	DenyChain   map[string]struct{}
	ReviewAbove decimal.Decimal
	DenyAbove   decimal.Decimal
	hasReviewBand bool
	hasDenyBand bool
}

// NewRules 从进程配置装配。金额档解析失败直接返回错误，禁止带着坏配置启动。
   func NewRules(cfg config.KYTConfig) (*Rules, error) {
	   mode, err := parseMode(cfg.Mode)
	   if err != nil {
		  return nil, err
	  }
	   r := &Rules{
		  Mode: mode,
		  DenyAddress: map[string]struct{}{},
		   DenyChain:   map[string]struct{}{},
	  }
	   for _, a := range cfg.DenyAddresses {
		   a = normalizeAddr(a)
		   if a != "" {
			  r.DenyAddress[a] = struct{}{}
		   }
	   }
	   for _, c := range cfg.DenyChains {
		   c = strings.ToUpper(strings.TrimSpace(c))
		   if c != "" {
			   r.DenyChain[c] = struct{}{}
		   }
	  }
	   if strings.TrimSpace(cfg.ReviewAmount) != "" {
		   d, err := decimal.NewFromString(strings.TrimSpace(cfg.ReviewAmount))
		   if err != nil {
			  return nil, fmt.Errorf("kyt review_amount: %w", err)
		  }
		   if d.IsPositive() {
			   r.ReviewAbove = d
			  r.hasReviewBand = true
		  }
	  }
	   if strings.TrimSpace(cfg.DenyAmount) != "" {
		   d, err := decimal.NewFromString(strings.TrimSpace(cfg.DenyAmount))
		   if err != nil {
			  return nil, fmt.Errorf("kyt deny_amount: %w", err)
		   }
		  if d.IsPositive() {
			   r.DenyAbove = d
			   r.hasDenyBand = true
		  }
	   }
	  return r, nil
  }

// Screen 进程内规则。空接收者 = 筛子没挂上，返回 ErrUnavailable，禁止默认 allow。
// 顺序：链/地址/资产/正数金额 → deny 名单与金额档 → 全局 mode=deny（压过 review 档）→ review 档 → mode。
// 大额停在 review 时资金仍冻着；mode=deny 必须先触发，让调用方回滚而不是挂冻结。
  func (r *Rules) Screen(_ context.Context, req Request) (Decision, string, error) {
	   if r == nil {
		   return "", "", ErrUnavailable
	  }
	   chain := strings.ToUpper(strings.TrimSpace(req.Chain))
	   if chain == "" || !supportedChain(chain) {
		  return Deny, "invalid chain", nil
	   }
	  if _, ok := r.DenyChain[chain]; ok {
		  return Deny, "chain denied", nil
	   }
	  addr := normalizeAddr(req.ToAddress)
	  if addr == "" {
		   return Deny, "invalid address", nil
	   }
	  if wallet.IsEVM(chain) && (len(addr) != 42 || !strings.HasPrefix(addr, "0x")) {
		   return Deny, "invalid address", nil
	   }
	   if _, ok := r.DenyAddress[addr]; ok {
		  return Deny, "address denied", nil
	  }
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   if !wallet.AssetOnChain(asset, chain) {
		   return Deny, "invalid asset", nil
	   }
	   amt,
		  err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	   if err != nil || !amt.IsPositive() || amt.Exponent() < -18 {
		   return Deny, "invalid amount", nil
	  }
	  if r.hasDenyBand && amt.GreaterThanOrEqual(r.DenyAbove) {
		  return Deny, "amount above deny band", nil
	  }
	  // 全局 deny 是熔断闸门，必须压过 review 金额档，否则大额单会停在冻结而不是回滚。
	   if r.Mode == Deny {
		  return Deny, "mode deny", nil
	   }
	   if r.hasReviewBand && amt.GreaterThanOrEqual(r.ReviewAbove) {
		  return Review, "amount above review band", nil
	  }
	   mode := r.Mode
	   if mode == "" {
		   mode = Allow
	   }
	  return mode, string(mode), nil
   }

  func supportedChain(chain string) bool {
	   for _, c := range wallet.SupportedChains() {
		   if c == chain {
			  return true
		   }
	  }
	   return false
  }

   func normalizeAddr(s string) string {
	   s = strings.TrimSpace(s)
	   if strings.HasPrefix(strings.ToLower(s), "0x") {
		  return strings.ToLower(s)
	  }
	  return s
  }
