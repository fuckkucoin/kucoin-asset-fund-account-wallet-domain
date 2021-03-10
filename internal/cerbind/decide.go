package cerbind

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// DecideLine 对单条事实求值。
   func (e *Engine) DecideLine(policies []Policy, ln Line) (Decision, error) {
	   ln, err := prepareLine(ln)
	   if err != nil {
		  return Decision{}, err
	  }
	  var trace []string
	   var win *Policy
	  for i := range policies {
		   p := policies[i]
		   ok, why := consider(p, ln)
		  if !ok {
			  trace = append(trace, p.ID+":"+why)
			  continue
		  }
		  trace = append(trace, p.ID+":match")
		  if win == nil || better(p, *win) {
			   cp := p
			   win = &cp
		  }
	  }
	   dec := Decision{
		   Asset: ln.Asset, Chain: ln.Chain, Amount: ln.Amount, Dest: ln.Dest,
		  DestRisk: ln.DestRisk, Side: ln.Side, Purpose: ln.Purpose, Approved: ln.Approved,
		  Trace: trace,
	   }
	  if win == nil {
		   dec.Reason = "no-match"
		   return dec, nil
	   }
	   dec.PolicyID = win.ID
	   dec.BindMode = win.BindMode
	  if !win.ForceCold {
		   dec.Reason = "exempt:" + win.ID
		  return dec, nil
	   }
	  dec.Required = true
	   dec.Threshold = win.Threshold
	  dec.Shares = win.Shares
	  dec.Reason = "force:" + win.ID
	  if win.ID == polFloor {
		   applyFloorBump(&dec, ln)
	  }
	  if dec.Threshold < 2 {
		   return Decision{}, fmt.Errorf("%w: winner threshold", ErrPolicy)
	   }
	   if dec.Shares < dec.Threshold {
		  dec.Shares = dec.Threshold
	  }
	  return dec, nil
  }

// DecideSnap 对整批取最严的强制结果。全部豁免才算不强制。

  func (e *Engine) DecideSnap(policies []Policy, snap Snapshot) (Decision, error) {
	  if len(snap.Lines) == 0 {
		   return Decision{}, fmt.Errorf("%w: empty snapshot", ErrBatch)
	   }
	   var best Decision
	  var traces []string
	   for _, ln := range snap.Lines {
		  dec, err := e.DecideLine(policies, ln)
		   if err != nil {
			   return Decision{}, err
		   }
		  traces = append(traces, dec.Trace...)
		   if !dec.Required {
			  continue
		   }
		   if !best.Required || stricter(dec, best) {
			  best = dec
		   }
	   }
	  if !best.Required {
		  return Decision{Reason: "no-line-required", Trace: traces, BindMode: ModeOne}, nil
	   }
	  best.Trace = traces
	   return best, nil
   }

  func stricter(a, b Decision) bool {
	  if a.Threshold != b.Threshold {
		  return a.Threshold > b.Threshold
	  }
	  if a.Shares != b.Shares {
		  return a.Shares > b.Shares
	  }
	   return a.PolicyID < b.PolicyID
  }

  func prepareLine(ln Line) (Line, error) {
	   asset, err := validateAssetLine(ln.Asset)
	   if err != nil {
		   return Line{}, err
	  }
	  chain, err := starOrChain(ln.Chain)
	  if err != nil {
		  return Line{}, err
	  }
	   if chain == "*" {
		  return Line{}, fmt.Errorf("%w: chain", ErrPolicy)
	   }
	   amt, err := money.Parse(strings.TrimSpace(ln.Amount))
	   if err != nil || !amt.IsPositive() {
		   return Line{}, ErrAmount
	  }
	  dest := strings.TrimSpace(ln.Dest)
	  if dest == "" || len(dest) > 191 {
		   return Line{}, fmt.Errorf("%w: dest", ErrBatch)
	   }
	  purpose, err := normPurpose(ln.Purpose)
	  if err != nil {
		   return Line{}, err
	  }
	  side, err := normSide(ln.Side)
	   if err != nil {
		   return Line{}, err
	   }
	  risk := classifyRisk(dest, ln.DestRisk)
	  if ln.LimitHit {
		   risk = bumpRisk(risk)
	   }
	   if riskRank(risk) == 0 {
		  return Line{}, fmt.Errorf("%w: risk", ErrPolicy)
	  }
	   refs, err := normRefs(ln.ItemRefs)
	   if err != nil {
		   return Line{}, err
	   }
	   ln.Asset,
		  ln.Chain, ln.Amount = asset, chain, amt.String()
	  ln.Dest, ln.DestRisk, ln.Side, ln.Purpose = dest, risk, side, purpose
	  ln.ItemRefs = refs
	  ln.LimitRef = clip(strings.TrimSpace(ln.LimitRef),
		   128)
	  return ln, nil
  }

   func validateAssetLine(s string) (string, error) {
	   got, err := validateStarAsset(s)
	  if err != nil {
		   return "", err
	  }
	  if got == "*" {
		   return "", fmt.Errorf("%w: asset", ErrPolicy)
	   }
	  return got, nil
   }

  func validateStarAsset(s string) (string, error) {
	  return starOrAsset(s)
   }

   func classifyRisk(dest, given string) string {
	   g := strings.ToLower(strings.TrimSpace(given))
	  switch g {
	   case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		   return g
	   case "med":
		   return RiskMedium
	   case "sanctioned":
		   return RiskCritical
	  }
	  d := strings.ToLower(strings.TrimSpace(dest))
	  switch {
	   case strings.HasPrefix(d, "sanc:"):
		   return RiskCritical
	  case strings.HasPrefix(d, "new:"):
		   return RiskHigh
	  case strings.HasPrefix(d, "wl:"):
		  return RiskLow
	  default:
		   return RiskMedium
	  }
   }

   func bumpRisk(s string) string {
	   r := riskRank(s) + 1
	   if r > 4 {
		  r = 4
	   }
	  return riskName(r)
   }

   func normRefs(in []string) ([]string, error) {
	  if len(in) == 0 {
		  return nil, nil
	   }
	   seen := map[string]struct{}{}
	   out := make([]string, 0, len(in))
	   for _, raw := range in {
		   ref := strings.TrimSpace(raw)
		   if ref == "" {
			   continue
		   }
		  if len(ref) > 128 || strings.ContainsAny(ref, " \t|") {
			  return nil, fmt.Errorf("%w: item ref", ErrBatch)
		  }
		   if _, ok := seen[ref]; ok {
			  continue
		   }
		  seen[ref] = struct{}{}
		   out = append(out, ref)
	  }
	  sortStrings(out)
	  return out, nil
   }

// applyFloorBump 只加成出厂底档。席位显式策略保持原阈值。
// 不变量：加成后份额不少于阈值，且不超过 9。

  func applyFloorBump(dec *Decision, ln Line) {
	  if dec == nil || !dec.Required {
		   return
	  }
	   amt, err := money.Parse(ln.Amount)
	   if err != nil {
		  return
	   }
	   need := 0
	   switch ln.Chain {
	  case "BTC":
		   floor, _ := money.Parse("10")
		  if amt.Cmp(floor) >= 0 {
			  need = 3
		  }
	  case "ETH":
		  floor, _ := money.Parse("500")
		  if amt.Cmp(floor) >= 0 {
			  need = 3
		   }
	   }
	   if need == 0 || dec.Threshold >= need {
		   return
	  }
	   dec.Threshold = need
	  if dec.Shares < need {
		  dec.Shares = need
	  }
	  if dec.Shares > 9 {
		  dec.Shares = 9
	  }
	   dec.Reason = dec.Reason + "+chain-floor"
   }

   func sortStrings(in []string) {
	   for i := 1; i < len(in); i++ {
		   j := i
		  for j > 0 && in[j] < in[j-1] {
			  in[j], in[j-1] = in[j-1], in[j]
			   j--
		   }
	  }
  }
