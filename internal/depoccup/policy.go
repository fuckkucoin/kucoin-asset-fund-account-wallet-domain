package depoccup

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Query struct {
	KYC string
	Asset string
	Source Source
	Chain string
}

  func policyKey(kyc, asset string, src Source, chain string) string {
	   return kyc + "|" + asset + "|" + string(src) + "|" + chain
  }

   func (p Policy) key() string {
	  return policyKey(p.KYC, p.Asset, p.Source, p.Chain)
   }

   func specificity(p Policy) int {
	  sc := 0
	  if p.KYC != "*" {
		  sc += 8
	  }
	  if p.Asset != "*" {
		   sc += 4
	   }
	  if p.Source != "*" && p.Source != "" {
		   sc += 2
	  }
	  if p.Chain != "*" {
		   sc += 1
	   }
	  return sc
  }

  func matches(p Policy, q Query) bool {
	  if !p.Enabled {
		   return false
	  }
	  if p.KYC != "*" && p.KYC != q.KYC {
		   return false
	   }
	  if p.Asset != "*" && p.Asset != q.Asset {
		  return false
	   }
	   if p.Source != "*" && p.Source != "" && p.Source != q.Source {
		  return false
	   }
	  if p.Chain != "*" && p.Chain != q.Chain {
		  return false
	  }
	   return true
   }

  func better(a, b Policy) bool {
	   sa,
		   sb := specificity(a), specificity(b)
	  if sa != sb {
		   return sa > sb
	   }
	   if a.Priority != b.Priority {
		   return a.Priority > b.Priority
	  }
	  if a.Version != b.Version {
		   return a.Version > b.Version
	  }
	  if a.RatioBPS != b.RatioBPS {
		  return a.RatioBPS > b.RatioBPS
	  }
	   return a.MaxWindow > b.MaxWindow
   }

   func (s *Service) Resolve(q Query) (Policy, int, error) {
	   if s == nil {
		  return Policy{}, 0, ErrUnavailable
	  }
	  kyc, err := NormKYC(q.KYC)
	   if err != nil || kyc == "*" {
		  return Policy{}, 0, ErrKYC
	   }
	   asset, err := NormAsset(q.Asset)
	  if err != nil || asset == "*" {
		  return Policy{}, 0, ErrAsset
	  }
	  src, err := ParseSource(string(q.Source))
	  if err != nil || src == "*" {
		  return Policy{}, 0, ErrSource
	   }
	   chain, err := NormChain(q.Chain)
	   if err != nil || chain == "*" {
		  return Policy{}, 0, ErrChain
	   }
	  q = Query{KYC: kyc, Asset: asset, Source: src, Chain: chain}
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   hit, ok := s.bestLocked(q)
	  if !ok {
		  return Policy{}, 0, ErrPolicyMiss
	  }
	   return hit, specificity(hit), nil
   }

  func (s *Service) bestLocked(q Query) (Policy, bool) {
	   var best *Policy
	   for _, p := range s.policies {
		  if p == nil || !matches(*p, q) {
			  continue
		  }
		  if best == nil || better(*p, *best) {
			  cp := p.copy()
			  best = &cp
		   }
	   }
	   if best == nil {
		  return Policy{}, false
	   }
	  return best.copy(), true
  }

   func (s *Service) Explain(q Query, tier Tier) (MatchHit, error) {
	  hit, _, err := s.Resolve(q)
	  if err != nil {
		  return MatchHit{}, err
	  }
	   userTier, err := ParseTier(string(tier))
	   if err != nil {
		  userTier = TierT0
	   }
	   eff := raiseFloor(userTier, hit.TierFloor)
	   bps := ApplyTierBPS(hit.RatioBPS,
		  eff)
	   if q.Source == SourceMixer {
		  bps = MaxRatioBPS
	  }
	  return MatchHit{
		  Policy: hit,
		   Specificity: specificity(hit),
		   AppliedBPS: bps,
		   Reason:     explainReason(hit, eff, bps),
	  }, nil
   }

  func explainReason(p Policy, tier Tier, bps int) string {
	   return fmt.Sprintf("kyc=%s asset=%s source=%s chain=%s spec=%d tier=%s bps=%d window=%s",
		  p.KYC,
		  p.Asset, p.Source, p.Chain, specificity(p), tier, bps, p.MaxWindow)
  }

   func (s *Service) ListPolicies(kyc, asset, source, chain string) []Policy {
	   if s == nil {
		   return nil
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Policy, 0)
	   for _, p := range s.policies {
		  if p == nil {
			  continue
		   }
		   if kyc != "" && !strings.EqualFold(p.KYC, kyc) && p.KYC != "*" {
			  continue
		  }
		   if asset != "" && !strings.EqualFold(p.Asset, asset) && p.Asset != "*" {
			  continue
		  }
		  if source != "" && !strings.EqualFold(string(p.Source), source) && p.Source != "*" {
			   continue
		   }
		   if chain != "" && !strings.EqualFold(p.Chain, chain) && p.Chain != "*" {
			  continue
		  }
		   out = append(out, p.copy())
	  }
	   sort.Slice(out, func(i, j int) bool {
		  if out[i].Priority != out[j].Priority {
			   return out[i].Priority > out[j].Priority
		   }
		   return out[i].ID < out[j].ID
	   })
	  return out
   }

   func validatePolicy(p Policy) error {
	   kyc, err := NormKYC(p.KYC)
	   if err != nil {
		   return err
	  }
	  asset, err := NormAsset(p.Asset)
	  if err != nil {
		  return err
	   }
	   src, err := ParseSource(string(p.Source))
	  if err != nil {
		   return err
	  }
	  chain, err := NormChain(p.Chain)
	   if err != nil {
		   return err
	   }
	   if kyc == "*" && asset == "*" && (src == "*" || src == "") && chain == "*" {
		  return ErrPolicy
	  }
	  if p.RatioBPS < 0 || p.RatioBPS > MaxRatioBPS {
		  return ErrRatio
	  }
	  if src == SourceMixer && p.RatioBPS < MaxRatioBPS {
		  return ErrRatio
	  }
	  if p.MaxWindow < MinWindowCap || p.MaxWindow > MaxWindowCap {
		   return ErrWindow
	  }
	   if p.MinWindow < 0 || p.MinWindow > p.MaxWindow {
		   return ErrWindow
	  }
	   if p.TierFloor != "" {
		  if _, err := ParseTier(string(p.TierFloor)); err != nil {
			  return err
		   }
	   }
	  _ = kyc
	   _ = asset
	  _ = chain
	  return nil
   }

  func (s *Service) UpsertPolicy(in PolicyIn) (Policy, error) {
	  if s == nil {
		   return Policy{}, ErrUnavailable
	  }
	   actor := strings.TrimSpace(in.Actor)
	  if actor == "" {
		   return Policy{}, ErrActor
	  }
	  kyc, err := NormKYC(in.KYC)
	  if err != nil {
		  return Policy{}, err
	  }
	   asset, err := NormAsset(in.Asset)
	   if err != nil {
		   return Policy{}, err
	   }
	   src,
		   err := ParseSource(in.Source)
	  if err != nil {
		  return Policy{}, err
	  }
	  chain, err := NormChain(in.Chain)
	   if err != nil {
		   return Policy{}, err
	   }
	   floor := TierT0
	   if strings.TrimSpace(in.TierFloor) != "" {
		   floor, err = ParseTier(in.TierFloor)
		   if err != nil {
			   return Policy{}, err
		  }
	   }
	  enabled := true
	   if in.Enabled != nil {
		   enabled = *in.Enabled
	   }
	  row := Policy{
		   KYC: kyc, Asset: asset, Source: src, Chain: chain,
		   RatioBPS: in.RatioBPS, MaxWindow: time.Duration(in.MaxSec) * time.Second,
		  MinWindow: time.Duration(in.MinSec) * time.Second, TierFloor: floor,
		   Priority: 100,
		  Enabled: enabled, Note: strings.TrimSpace(in.Note),
	  }
	  row.MaxSec = int(row.MaxWindow / time.Second)
	   row.MinSec = int(row.MinWindow / time.Second)
	   if err := validatePolicy(row); err != nil {
		  return Policy{}, err
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  key := row.key()
	   prev := s.byKey[key]
	  ver := 1
	   prevOn := false
	   if prev != nil {
		  ver = prev.Version + 1
		   prevOn = prev.Enabled
		   prev.Enabled = false
	   }
	   row.Version = ver
	   row.ID = fmt.Sprintf("pol-%s-v%d", shortKey(key), ver)
	   if err := validatePolicy(row); err != nil {
		   if prev != nil {
			   prev.Enabled = prevOn
		   }
		   return Policy{}, err
	   }
	   cp := row
	   s.policies = append(s.policies, &cp)
	   s.byKey[key] = &cp
	   if err := s.savePolicy(&cp); err != nil {
		   if prev != nil {
			  prev.Enabled = prevOn
			   s.byKey[key] = prev
		   } else {
			   delete(s.byKey, key)
		   }
		   s.policies = s.policies[:len(s.policies)-1]
		   s.saveErr = err.Error()
		   return Policy{}, err
	  }
	  s.auditLocked(Audit{
		  LotID: row.ID, UserID: actor, Event: "policy_upsert", ToState: LotState(row.KYC),
		  Amount: fmt.Sprintf("%d", row.RatioBPS), Actor: actor,
	   })
	   return cp.copy(), nil
  }

   func shortKey(key string) string {
	  r := strings.NewReplacer("|", "-", "*", "any")
	  out := r.Replace(key)
	   if len(out) > 80 {
		   return out[:80]
	  }
	   return out
  }
