package cerbind

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	polCritical = "cer-pol-critical"
	polHot      = "cer-pol-hot"
	polBTC  = "cer-pol-btc"
	polUSDT = "cer-pol-usdt"
	polFloor = "cer-pol-floor"
)

  func seedPolicies(now time.Time) []Policy {
	  return []Policy{
		   {
			  ID: polCritical, Name: "sanctioned destination", Asset: "*", Chain: "*",
			   Purpose: PurposeAny, MinAmount: "0", DestRisk: RiskCritical, Side: SideAny,
			  ForceCold: true, Threshold: 3, Shares: 5, BindMode: ModeOne,
			  Priority: 200,
			  Enabled:  true, CreatedAt: now, UpdatedAt: now,
		  },
		   {
			  ID: polHot, Name: "hot inventory exemption", Asset: "*", Chain: "*",
			  Purpose: PurposeWithdraw, MinAmount: "0", DestRisk: RiskAny, Side: SideHot,
			   ForceCold: false, Threshold: 0, Shares: 0, BindMode: ModeOne,
			  Priority: 100, Enabled: true, CreatedAt: now, UpdatedAt: now,
		  },
		   {
			   ID: polBTC, Name: "large btc cold", Asset: "BTC", Chain: "*",
			   Purpose: PurposeWithdraw, MinAmount: "5", DestRisk: RiskAny, Side: SideCold,
			   ForceCold: true, Threshold: 3, Shares: 5, BindMode: ModeOne,
			   Priority: 60, Enabled: true, CreatedAt: now, UpdatedAt: now,
		  },
		  {
			   ID: polUSDT, Name: "large usdt cold", Asset: "USDT", Chain: "*",
			   Purpose: PurposeWithdraw, MinAmount: "1000000", DestRisk: RiskAny, Side: SideCold,
			  ForceCold: true, Threshold: 3, Shares: 5, BindMode: ModeOne,
			  Priority: 60, Enabled: true, CreatedAt: now, UpdatedAt: now,
		  },
		  {
			  ID: polFloor, Name: "cold withdraw floor", Asset: "*", Chain: "*",
			  Purpose: PurposeWithdraw, MinAmount: "10000", DestRisk: RiskAny, Side: SideCold,
			  ForceCold: true, Threshold: 2, Shares: 3, BindMode: ModeOne,
			   Priority: 20, Enabled: true, CreatedAt: now, UpdatedAt: now,
		   },
	  }
   }

// normalizePolicy 清洗并拒绝无法执行的策略。
// 做什么：统一资产、链、风险和绑定方式。
// 不读取限额服务，LimitRef 原样截断保存。
// 不变量：启用的强制策略阈值≥2 且份额≥阈值；金额窗口不颠倒。

  func normalizePolicy(p Policy, actor string, now time.Time) (Policy, error) {
	   p.ID = strings.TrimSpace(p.ID)
	  p.Name = clip(strings.TrimSpace(p.Name),
		   128)
	   if p.ID == "" || len(p.ID) > 64 || p.Name == "" {
		  return Policy{}, fmt.Errorf("%w: id and name", ErrPolicy)
	  }
	  if !idToken(p.ID) {
		   return Policy{}, fmt.Errorf("%w: id", ErrPolicy)
	   }
	   asset, err := starOrAsset(p.Asset)
	   if err != nil {
		   return Policy{}, err
	   }
	   chain, err := starOrChain(p.Chain)
	  if err != nil {
		   return Policy{}, err
	   }
	   purpose, err := normPurpose(p.Purpose)
	   if err != nil {
		   return Policy{}, err
	  }
	  side, err := normSide(p.Side)
	  if err != nil {
		   return Policy{}, err
	  }
	   risk, err := normPolicyRisk(p.DestRisk)
	  if err != nil {
		  return Policy{}, err
	   }
	  mode, err := normMode(p.BindMode)
	  if err != nil {
		   return Policy{}, err
	   }
	  minAmt, err := normMin(p.MinAmount)
	  if err != nil {
		   return Policy{}, err
	  }
	   maxAmt,
		  err := normMax(p.MaxAmount)
	   if err != nil {
		   return Policy{}, err
	   }
	   if maxAmt != "" {
		   mx, _ := money.Parse(maxAmt)
		  mn, _ := money.Parse(minAmt)
		  if mx.Cmp(mn) <= 0 {
			   return Policy{}, fmt.Errorf("%w: amount window", ErrPolicy)
		  }
	  }
	   if p.Priority < 0 || p.Priority > 1000 {
		  return Policy{}, fmt.Errorf("%w: priority", ErrPolicy)
	   }
	  if p.MaxAgeSec < 0 || p.MaxAgeSec > 7*24*3600 {
		  return Policy{}, fmt.Errorf("%w: max age", ErrPolicy)
	  }
	  if p.ForceCold {
		  if p.Threshold < 2 || p.Shares < p.Threshold || p.Shares > 9 {
			   return Policy{}, fmt.Errorf("%w: threshold", ErrPolicy)
		   }
	  } else if p.Threshold != 0 || p.Shares != 0 {
		   return Policy{}, fmt.Errorf("%w: exemption has no quorum", ErrPolicy)
	  }
	   if _, err := validate.Actor(actor); err != nil && actor != "" {
		   return Policy{}, ErrActor
	  }
	  p.Asset, p.Chain, p.Purpose, p.Side = asset, chain, purpose, side
	  p.DestRisk, p.BindMode = risk, mode
	   p.MinAmount, p.MaxAmount = minAmt, maxAmt
	  p.LimitRef = clip(strings.TrimSpace(p.LimitRef), 128)
	  p.UpdatedBy = clip(strings.TrimSpace(actor), 64)
	  if p.CreatedAt.IsZero() {
		   p.CreatedAt = now
	  }
	   p.UpdatedAt = now
	   return p, nil
  }

   func starOrAsset(s string) (string, error) {
	   s = strings.ToUpper(strings.TrimSpace(s))
	   if s == "" || s == "*" {
		  return "*", nil
	  }
	  got, err := validate.Asset(s)
	   if err != nil {
		  return "", fmt.Errorf("%w: asset", ErrPolicy)
	   }
	   return got, nil
   }

   func starOrChain(s string) (string, error) {
	   s = strings.ToUpper(strings.TrimSpace(s))
	   if s == "" || s == "*" {
		  return "*", nil
	  }
	   got, err := validate.Chain(s)
	   if err != nil {
		   return "", fmt.Errorf("%w: chain", ErrPolicy)
	   }
	   return got, nil
   }

  func normPurpose(s string) (string, error) {
	   s = strings.ToLower(strings.TrimSpace(s))
	  if s == "" {
		   s = PurposeWithdraw
	  }
	  switch s {
	   case PurposeWithdraw, PurposeAny, "sweep":
		   return s, nil
	   default:
		  return "", fmt.Errorf("%w: purpose", ErrPolicy)
	  }
  }

   func normSide(s string) (string, error) {
	   s = strings.ToLower(strings.TrimSpace(s))
	  if s == "" {
		  s = SideCold
	  }
	   switch s {
	  case SideCold, SideWarm, SideHot, SideAny:
		   return s, nil
	   default:
		   return "", fmt.Errorf("%w: side", ErrPolicy)
	   }
   }

  func normPolicyRisk(s string) (string, error) {
	   s = strings.ToLower(strings.TrimSpace(s))
	   if s == "" {
		  return RiskAny, nil
	   }
	   switch s {
	  case RiskAny,
		   RiskLow, RiskMedium, RiskHigh, RiskCritical,
		  "ge:low", "ge:medium", "ge:high", "ge:critical":
		   return s, nil
	  default:
		  return "", fmt.Errorf("%w: dest risk", ErrPolicy)
	   }
  }

   func normMode(s string) (string, error) {
	   s = strings.ToLower(strings.TrimSpace(s))
	  if s == "" {
		   return ModeOne, nil
	   }
	  switch s {
	   case ModeOne, ModeMany, "1:1", "n:1":
		  if s == "1:1" {
			  return ModeOne, nil
		  }
		   if s == "n:1" {
			   return ModeMany, nil
		   }
		   return s, nil
	   default:
		   return "", fmt.Errorf("%w: bind mode", ErrPolicy)
	   }
  }

   func normMin(s string) (string, error) {
	   s = strings.TrimSpace(s)
	   if s == "" {
		   return "0", nil
	  }
	  a, err := money.ParseBook(s)
	   if err != nil || a.Cmp(money.Zero()) < 0 {
		  return "", fmt.Errorf("%w: min", ErrAmount)
	  }
	  return a.String(), nil
   }

  func normMax(s string) (string, error) {
	  s = strings.TrimSpace(s)
	   if s == "" {
		   return "", nil
	  }
	  a, err := money.Parse(s)
	  if err != nil || !a.IsPositive() {
		   return "", fmt.Errorf("%w: max", ErrAmount)
	   }
	   return a.String(), nil
   }

  func idToken(s string) bool {
	  for _, r := range s {
		   if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			   continue
		   }
		  return false
	   }
	  return true
   }

type skip struct {
	id string
	reason string
}

// consider 判断策略是否覆盖这条事实，并给出跳过原因。
// 做什么：逐项资产、链、用途、库存侧、风险和金额窗。
// 风险 ge: 表示不低于该档；精确档只等于该档。
// 不变量：未启用的策略不参与胜出。
  func consider(p Policy, ln Line) (bool, string) {
	   if !p.Enabled {
		   return false, "disabled"
	  }
	  if p.Asset != "*" && p.Asset != ln.Asset {
		  return false, "asset"
	   }
	   if p.Chain != "*" && p.Chain != ln.Chain {
		   return false, "chain"
	  }
	  if p.Purpose != PurposeAny && p.Purpose != ln.Purpose {
		  return false, "purpose"
	   }
	  if p.Side != SideAny && p.Side != ln.Side {
		  return false, "side"
	  }
	  if !riskCovers(p.DestRisk, ln.DestRisk) {
		   return false, "risk"
	   }
	   amt, err := money.Parse(ln.Amount)
	   if err != nil {
		   return false, "amount"
	  }
	  minA, err := money.ParseBook(p.MinAmount)
	   if err != nil {
		  return false, "min"
	  }
	  if amt.Cmp(minA) < 0 {
		  return false, "below-min"
	  }
	   if p.MaxAmount != "" {
		  maxA, err := money.Parse(p.MaxAmount)
		  if err != nil {
			  return false, "max"
		   }
		   if amt.Cmp(maxA) >= 0 {
			  return false, "above-max"
		  }
	   }
	  return true, "match"
   }

   func riskCovers(policyRisk, got string) bool {
	   if policyRisk == "" || policyRisk == RiskAny {
		   return true
	  }
	  gr := riskRank(got)
	   if gr == 0 {
		   return false
	  }
	   if strings.HasPrefix(policyRisk, "ge:") {
		  return gr >= riskRank(strings.TrimPrefix(policyRisk, "ge:"))
	  }
	   return got == policyRisk
  }

   func riskRank(s string) int {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case RiskLow:
		  return 1
	  case RiskMedium, "med":
		   return 2
	   case RiskHigh:
		  return 3
	   case RiskCritical, "sanctioned":
		   return 4
	   default:
		   return 0
	   }
   }

  func riskName(rank int) string {
	  switch rank {
	   case 1:
		   return RiskLow
	   case 2:
		  return RiskMedium
	   case 3:
		   return RiskHigh
	   case 4:
		   return RiskCritical
	   default:
		  return ""
	  }
   }

// specificity 用于同先级时的稳定选择：精确维度多于通配。
  func specificity(p Policy) int {
	  n := 0
	   if p.Asset != "*" {
		  n += 8
	  }
	   if p.Chain != "*" {
		  n += 4
	   }
	   if p.Side != SideAny {
		  n += 2
	  }
	  if p.DestRisk != RiskAny && !strings.HasPrefix(p.DestRisk, "ge:") {
		  n += 2
	  }
	   if strings.HasPrefix(p.DestRisk, "ge:") {
		  n++
	   }
	  if p.Purpose != PurposeAny {
		  n++
	   }
	   if p.ForceCold {
		  n++
	  }
	   return n
  }

  func better(a, b Policy) bool {
	  if a.Priority != b.Priority {
		   return a.Priority > b.Priority
	   }
	  if specificity(a) != specificity(b) {
		   return specificity(a) > specificity(b)
	  }
	  if a.ForceCold != b.ForceCold {
		  return a.ForceCold
	   }
	  if a.Threshold != b.Threshold {
		   return a.Threshold > b.Threshold
	  }
	  return a.ID < b.ID
  }
