package sweeprun

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// AgreeCode 读闸结论。空串表示同意通过。

type AgreeCode string

const (
	AgreeOK          AgreeCode = ""
	AgreeDeniedBelow AgreeCode = "below_waterline"
	AgreeDeniedRule  AgreeCode = "rule_unavailable"
	AgreeDeniedBal   AgreeCode = "shadow_bal_invalid"
	AgreeDeniedAsset AgreeCode = "asset_invalid"
	AgreeDeniedHalt   AgreeCode = "agree_halt"
	AgreeDeniedKinds  AgreeCode = "kinds_empty"
	AgreeDeniedRetain AgreeCode = "would_pierce_retain"
)

// String 便于审计。
  func (c AgreeCode) String() string { return string(c) }

// Failed 非空即失败。
   func (c AgreeCode) Failed() bool { return c != AgreeOK }

// RebalanceRuleSnap 规则水位快照。只读消费，不回写 sweeppol 公式内核。

type RebalanceRuleSnap struct {
	Asset string
	Waterline string // 热账上限，超过才同意归集
	MinSweep  string
	MinRetain string
	Kinds     []string
	Version   uint64
	HaltAgree bool
}

// ShadowBal 影子余额行。AgreeGate 仅瞬时读取，不记账本分录。
type ShadowBal struct {
	Chain  string
	Asset  string
	Amount string
}

// AgreeEvent 瞬时读闸结果。不含租约 ID。
type AgreeEvent struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Code AgreeCode `json:"agree_code"`
	ShadowBal string `json:"shadow_bal"`
	Waterline string `json:"waterline"`
	MinSweep       string   `json:"min_sweep"`
	Kinds          []string `json:"kinds"`
	RuleVersion    uint64   `json:"rule_version"`
	BalFingerprint string   `json:"bal_fingerprint"`
	RuleFingerprint string `json:"rule_fingerprint"`
	Reason string `json:"reason,omitempty"`
}

// AgreeGate 影子余额与规则水位读闸。进程内簿记，供 agreeleasegate 只读挂钩。
type AgreeBook struct {
	mu   sync.RWMutex
	bals map[string]ShadowBal
	rules map[string]RebalanceRuleSnap
}

// NewAgreeGate 空闸。
  func NewAgreeBook() *AgreeBook {
	  return &AgreeBook{
		   bals:  map[string]ShadowBal{},
		  rules: map[string]RebalanceRuleSnap{},
	   }
   }

  func balKey(chain, asset string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

   func assetKey(asset string) string {
	  return strings.ToUpper(strings.TrimSpace(asset))
   }

// PutShadowBal 写入影子余额。金额必须过 ParseBook。
   func (g *AgreeBook) PutShadowBal(row ShadowBal) error {
	  if g == nil {
		   return ErrUnavailable
	  }
	  ch, err := validate.Chain(row.Chain)
	  if err != nil {
		   return ErrChain
	   }
	   asset, err := validate.Asset(row.Asset)
	  if err != nil {
		  return ErrAsset
	   }
	  if _, err := money.ParseBook(orZ(row.Amount)); err != nil {
		   return ErrAmount
	  }
	   g.mu.Lock()
	   defer g.mu.Unlock()
	   row.Chain, row.Asset = ch, asset
	   g.bals[balKey(ch, asset)] = row
	  return nil
   }

// GetShadowBal 瞬时读取。
   func (g *AgreeBook) GetShadowBal(chain, asset string) (ShadowBal, bool) {
	  if g == nil {
		   return ShadowBal{}, false
	  }
	  g.mu.RLock()
	   defer g.mu.RUnlock()
	  row,
		   ok := g.bals[balKey(chain, asset)]
	   return row, ok
   }

// PutRule 登记规则水位快照。不计算再平衡金额。
  func (g *AgreeBook) PutRule(r RebalanceRuleSnap) error {
	   if g == nil {
		  return ErrUnavailable
	   }
	   asset, err := validate.Asset(r.Asset)
	   if err != nil {
		   return ErrAsset
	   }
	   if _, err := money.ParseBook(orZ(firstNonEmpty(r.Waterline, "0"))); err != nil {
		  return ErrAmount
	  }
	  if r.MinSweep != "" {
		  if _, err := money.ParseBook(r.MinSweep); err != nil {
			  return ErrAmount
		  }
	  }
	  if r.MinRetain != "" {
		  if _, err := money.ParseBook(r.MinRetain); err != nil {
			   return ErrAmount
		   }
	   }
	   g.mu.Lock()
	   defer g.mu.Unlock()
	  r.Asset = asset
	   g.rules[assetKey(asset)] = r
	   return nil
   }

// GetRule 瞬时读取规则快照。

  func (g *AgreeBook) GetRule(asset string) (RebalanceRuleSnap, bool) {
	   if g == nil {
		   return RebalanceRuleSnap{}, false
	   }
	  g.mu.RLock()
	  defer g.mu.RUnlock()
	   r, ok := g.rules[assetKey(asset)]
	   return r, ok
   }

// BumpRuleVersion 抬升规则代次。旧指纹立即失效。
  func (g *AgreeBook) BumpRuleVersion(asset string, waterline string) error {
	   if g == nil {
		   return ErrUnavailable
	   }
	  assetU, err := validate.Asset(asset)
	  if err != nil {
		   return ErrAsset
	  }
	  g.mu.Lock()
	   defer g.mu.Unlock()
	   r, ok := g.rules[assetKey(assetU)]
	  if !ok {
		  return ErrAgreeRule
	   }
	   if waterline != "" {
		  if _, err := money.ParseBook(waterline); err != nil {
			   return ErrAmount
		  }
		   r.Waterline = waterline
	   }
	  r.Version++
	  g.rules[assetKey(assetU)] = r
	   return nil
   }

// CheckAgree 瞬时读闸。仅比较影子余额与水位，不签发租约。
// 失败后果：返回非空同意码，调用方不得把该结论当作可执行租约。
   func (g *AgreeBook) CheckAgree(chain, asset string) (AgreeEvent, error) {
	  ev := AgreeEvent{Chain: strings.ToUpper(strings.TrimSpace(chain)), Asset: strings.ToUpper(strings.TrimSpace(asset))}
	   if g == nil {
		  ev.Code = AgreeDeniedHalt
		   ev.Reason = "gate_unavailable"
		  return ev, ErrUnavailable
	   }
	  if _, err := validate.Asset(asset); err != nil {
		   ev.Code = AgreeDeniedAsset
		  ev.Reason = "asset_invalid"
		  return ev, ErrAsset
	  }
	   g.mu.RLock()
	   bal, balOK := g.bals[balKey(chain, asset)]
	   rule, ruleOK := g.rules[assetKey(asset)]
	  g.mu.RUnlock()
	   if !ruleOK {
		   ev.Code = AgreeDeniedRule
		  ev.Reason = "rule_unavailable"
		   return ev, nil
	  }
	  if rule.HaltAgree {
		   ev.Code = AgreeDeniedHalt
		   ev.Reason = "agree_halt"
		  ev.RuleVersion = rule.Version
		   ev.RuleFingerprint = RuleFingerprint(rule)
		  return ev, nil
	   }
	   if len(rule.Kinds) == 0 {
		   ev.Code = AgreeDeniedKinds
		   ev.Reason = "kinds_empty"
		  ev.RuleVersion = rule.Version
		   return ev, nil
	   }
	  if !balOK {
		  ev.Code = AgreeDeniedBal
		   ev.Reason = "shadow_bal_missing"
		   ev.RuleVersion = rule.Version
		  ev.RuleFingerprint = RuleFingerprint(rule)
		   ev.Waterline = rule.Waterline
		   ev.MinSweep = rule.MinSweep
		   ev.Kinds = append([]string(nil), rule.Kinds...)
		  return ev, nil
	  }
	   hot,
		  err := money.ParseBook(orZ(bal.Amount))
	  if err != nil {
		   ev.Code = AgreeDeniedBal
		   ev.Reason = "shadow_bal_invalid"
		  return ev, ErrAmount
	  }
	  max,
		   err := money.ParseBook(orZ(rule.Waterline))
	  if err != nil {
		   ev.Code = AgreeDeniedRule
		  ev.Reason = "waterline_invalid"
		  return ev, ErrAmount
	   }
	  ev.ShadowBal = hot.String()
	   ev.Waterline = max.String()
	  ev.MinSweep = rule.MinSweep
	  ev.Kinds = append([]string(nil), rule.Kinds...)
	  ev.RuleVersion = rule.Version
	   ev.BalFingerprint = BalFingerprint(bal)
	   ev.RuleFingerprint = RuleFingerprint(rule)
	   if hot.Cmp(max) <= 0 {
		  ev.Code = AgreeDeniedBelow
		  ev.Reason = "below_waterline"
		   return ev, nil
	  }
	  if rule.MinRetain != "" {
		  retain, rerr := money.ParseBook(rule.MinRetain)
		   if rerr == nil {
			   excess, serr := hot.Sub(max)
			  if serr == nil {
				   after, aerr := hot.Sub(excess)
				  if aerr == nil && after.Cmp(retain) < 0 {
					  keep, kerr := hot.Sub(retain)
					   if kerr != nil || !keep.IsPositive() {
						   ev.Code = AgreeDeniedRetain
						  ev.Reason = "would_pierce_retain"
						  return ev, nil
					   }
				   }
			   }
		  }
	  }
	   ev.Code = AgreeOK
	  ev.Reason = "agreed"
	  return ev, nil
  }

// LiveFingerprints 复验用活快照。闸不可用则失败关闭。
  func (g *AgreeBook) LiveFingerprints(chain, asset string) (AgreeEvent, error) {
	   return g.CheckAgree(chain, asset)
   }

// BalFingerprint 影子余额指纹。金额走规范化字符串，禁止浮点。
  func BalFingerprint(b ShadowBal) string {
	   raw := strings.ToUpper(strings.TrimSpace(b.Chain)) + "|" +
		   strings.ToUpper(strings.TrimSpace(b.Asset)) + "|" +
		  normalizeAmt(b.Amount)
	   sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
  }

// RuleFingerprint 规则水位与代次指纹。

   func RuleFingerprint(r RebalanceRuleSnap) string {
	   kinds := strings.Join(normalizeKinds(r.Kinds), ",")
	   raw := strings.ToUpper(strings.TrimSpace(r.Asset)) + "|" +
		   normalizeAmt(r.Waterline) + "|" + normalizeAmt(r.MinSweep) + "|" +
		   normalizeAmt(r.MinRetain) + "|" + kinds + "|" + fmt.Sprintf("%d", r.Version)
	   sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
   }

  func normalizeAmt(s string) string {
	   d, err := money.ParseBook(orZ(s))
	   if err != nil {
		  return strings.TrimSpace(s)
	   }
	   return d.String()
  }

  func normalizeKinds(in []string) []string {
	   out := make([]string, 0, len(in))
	  seen := map[string]struct{}{}
	   for _, k := range in {
		  k = strings.ToLower(strings.TrimSpace(k))
		   if k == "" {
			   continue
		  }
		  if _, ok := seen[k]; ok {
			   continue
		  }
		  seen[k] = struct{}{}
		  out = append(out, k)
	  }
	   return out
  }

   func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		  return a
	  }
	   return b
  }
