package sweeprun

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	ceremonyPhaseOK = "activate"
	airgapKindSweep = "sweep"
)

var airgapPhaseOK = map[string]struct{}{
	"sealed":   {},
	"exported": {},
	"imported": {},
	"closed": {},
}

var signStateOK = map[string]struct{}{
	"approved": {},
	"signed":   {},
}

// CeremonyView 只带复核需要的字段，不包含分片材料。
type CeremonyView struct {
	ID string
	Phase string
	Shares    int
	Threshold int
	Actors []string
}

// AirgapView 气隙会话的只读投影。
type AirgapView struct {
	ID    string
	Phase string
	Kind  string
	Asset string
	Amount      string
	PayloadHash string
}

// SignView 签章管线任务的只读投影。
type SignView struct {
	ID    string
	State string
	Chain string
	Amount    string
	ExpiresAt time.Time
}

// RefSource 三个编号的只读查询。查不到必须返回错误，不得返回空视图充数。
type RefSource interface {
	Ceremony(id string) (CeremonyView, error)
	Airgap(id string) (AirgapView, error)
	Sign(id string) (SignView, error)
}

  func distinctLower(in []string) int {
	   seen := map[string]struct{}{}
	   for _, a := range in {
		  a = strings.ToLower(strings.TrimSpace(a))
		  if a == "" || a == "system" {
			  continue
		  }
		   seen[a] = struct{}{}
	   }
	   return len(seen)
   }

   func amountEqual(a, b string) bool {
	  aa, err := money.Parse(a)
	   if err != nil {
		  return false
	   }
	   bb, err := money.Parse(b)
	  if err != nil {
		  return false
	  }
	  return aa.Cmp(bb) == 0
   }

   func judgeCeremony(v CeremonyView) (string, bool) {
	  if strings.TrimSpace(v.ID) == "" {
		   return "ceremony id empty", false
	   }
	   if v.Phase != ceremonyPhaseOK {
		   return "ceremony phase " + v.Phase, false
	   }
	   if v.Threshold < 2 || v.Shares < v.Threshold {
		   return "ceremony quorum short", false
	  }
	   if distinctLower(v.Actors) < 2 {
		   return "ceremony actors short", false
	  }
	  return "", true
  }

   func judgeAirgap(v AirgapView, asset, amount string) (string, bool) {
	  if strings.TrimSpace(v.ID) == "" {
		   return "airgap id empty", false
	   }
	  if _, ok := airgapPhaseOK[v.Phase]; !ok {
		   return "airgap phase " + v.Phase, false
	   }
	  if strings.TrimSpace(v.PayloadHash) == "" {
		  return "airgap hash empty", false
	  }
	  kind := strings.ToLower(strings.TrimSpace(v.Kind))
	  switch kind {
	   case airgapKindSweep, "cold", "rebalance":
	  default:
		   return "airgap kind " + kind, false
	   }
	   if !strings.EqualFold(v.Asset, asset) {
		   return "airgap asset mismatch", false
	   }
	  if !amountEqual(v.Amount, amount) {
		   return "airgap amount mismatch", false
	   }
	   return "", true
  }

   func judgeSign(v SignView, chain, amount string, now time.Time) (string, bool) {
	  if strings.TrimSpace(v.ID) == "" {
		   return "sign id empty", false
	  }
	   if _, ok := signStateOK[v.State]; !ok {
		   return "sign state " + v.State, false
	   }
	   if v.ExpiresAt.IsZero() || !v.ExpiresAt.After(now) {
		  return "sign expired", false
	   }
	  if !strings.EqualFold(strings.TrimSpace(v.Chain), chain) {
		   return "sign chain mismatch", false
	   }
	   if !amountEqual(v.Amount, amount) {
		   return "sign amount mismatch", false
	   }
	   return "", true
   }

   func (o *Orch) judgeRefs(p *SweepPlan, leg *SweepLeg, refs RefBind, now time.Time) (RefVerdict, error) {
	   if o == nil || o.refs == nil {
		  return RefVerdict{OK: false, Reason: "ref source unavailable"}, fmt.Errorf("%w: ref source unavailable", ErrColdRef)
	   }
	   cer, err := o.refs.Ceremony(refs.CeremonyID)
	   if err != nil {
		  return RefVerdict{}, fmt.Errorf("%w: ceremony: %v", ErrColdRef, err)
	   }
	  air, err := o.refs.Airgap(refs.AirgapID)
	   if err != nil {
		   return RefVerdict{}, fmt.Errorf("%w: airgap: %v", ErrColdRef, err)
	   }
	   sig, err := o.refs.Sign(refs.SignID)
	   if err != nil {
		  return RefVerdict{}, fmt.Errorf("%w: sign: %v", ErrColdRef, err)
	  }
	  out := RefVerdict{Phase: cer.Phase, Airgap: air.Phase, Sign: sig.State}
	  if why, ok := judgeCeremony(cer); !ok {
		   out.Reason = why
		  return out, fmt.Errorf("%w: %s", ErrColdRef, why)
	   }
	   if why, ok := judgeAirgap(air, p.Asset, leg.Amount); !ok {
		  out.Reason = why
		   return out, fmt.Errorf("%w: %s", ErrColdRef, why)
	   }
	   if why, ok := judgeSign(sig, p.Chain, leg.Amount, now); !ok {
		  out.Reason = why
		   return out, fmt.Errorf("%w: %s", ErrColdRef, why)
	   }
	   if err := o.refReuse(p, leg.ID, refs); err != nil {
		   out.Reason = err.Error()
		  return out, err
	  }
	  out.OK = true
	  return out, nil
  }

// refReuse 气隙会话和签章任务在同一计划内只能绑一条腿。仪式可以共用。

   func (o *Orch) refReuse(p *SweepPlan, legID string, refs RefBind) error {
	   for i := range p.Legs {
		  lg := &p.Legs[i]
		  if lg.ID == legID {
			  continue
		  }
		   if refs.AirgapID != "" && lg.Refs.AirgapID == refs.AirgapID {
			   return fmt.Errorf("%w: airgap reused", ErrColdRef)
		   }
		  if refs.SignID != "" && lg.Refs.SignID == refs.SignID {
			  return fmt.Errorf("%w: sign reused", ErrColdRef)
		   }
	  }
	  return nil
  }
