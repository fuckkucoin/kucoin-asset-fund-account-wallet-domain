package sweeprun

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

  func canonicalLink(prev, planID, legID string, seq, legSeq int, kind, from, to, amount, state, ceremony, airgap, sign, ceremonyPhase, airgapPhase, signState, actor string) string {
	   return strings.Join([]string{
		  prev,
		   planID,
		  legID,
		  strconv.Itoa(seq),
		  strconv.Itoa(legSeq),
		  kind,
		  from,
		   to,
		  amount,
		  state,
		   ceremony,
		   airgap,
		  sign,
		  ceremonyPhase,
		  airgapPhase,
		  signState,
		  actor,
	  }, "|")
  }

   func hashCanonical(canon string) string {
	  sum := sha256.Sum256([]byte(canon))
	  return hex.EncodeToString(sum[:])
   }

  func sealLink(prev, planID, legID string, seq, legSeq int, kind, from, to, amount, state, ceremony, airgap, sign, ceremonyPhase, airgapPhase, signState, actor string, at time.Time) ProofLink {
	  canon := canonicalLink(prev, planID, legID, seq, legSeq, kind, from, to, amount, state, ceremony, airgap, sign, ceremonyPhase, airgapPhase, signState, actor)
	   return ProofLink{
		   PlanID: planID, Seq: seq, Prev: prev, Hash: hashCanonical(canon),
		  LegID: legID, LegSeq: legSeq, Kind: kind, From: from, To: to, Amount: amount,
		   State: state, CeremonyID: ceremony, AirgapID: airgap, SignID: sign,
		   CeremonyPhase: ceremonyPhase, AirgapPhase: airgapPhase, SignState: signState,
		   Actor: actor, At: at,
	   }
  }

  func fingerprint(chain, asset string, legs []SweepLeg) string {
	  var b strings.Builder
	   b.WriteString(chain)
	  b.WriteByte('|')
	  b.WriteString(asset)
	  for i := range legs {
		   lg := &legs[i]
		   if lg.Compensate {
			   continue
		   }
		   fmt.Fprintf(&b, "|%d|%s|%s|%s|%s", lg.Seq, lg.Kind, lg.From, lg.To, lg.Amount)
	   }
	  sum := sha256.Sum256([]byte(b.String()))
	  return hex.EncodeToString(sum[:])
  }

// VerifyLinks 重算整链。冷端进入 ready 及之后的环必须带齐三引用。

   func VerifyLinks(links []ProofLink) error {
	   if len(links) == 0 {
		   return fmt.Errorf("%w: empty", ErrChainBreak)
	  }
	   seen := map[string]struct{}{}
	  for i := range links {
		   lk := links[i]
		   if lk.Seq != i {
			  return fmt.Errorf("%w: seq %d at %d", ErrChainBreak, lk.Seq, i)
		   }
		  prev := lk.Prev
		  if i == 0 {
			  if prev != genesisPrev {
				   return fmt.Errorf("%w: genesis prev", ErrChainBreak)
			  }
		  } else if prev != links[i-1].Hash {
			   return fmt.Errorf("%w: prev mismatch at %d", ErrChainBreak, i)
		   }
		   want := sealLink(prev, lk.PlanID, lk.LegID, lk.Seq, lk.LegSeq, lk.Kind, lk.From, lk.To, lk.Amount, lk.State, lk.CeremonyID, lk.AirgapID, lk.SignID, lk.CeremonyPhase, lk.AirgapPhase, lk.SignState, lk.Actor, lk.At)
		   if want.Hash != lk.Hash {
			   return fmt.Errorf("%w: hash at %d", ErrChainBreak, i)
		  }
		  if _, ok := seen[lk.Hash]; ok {
			   return fmt.Errorf("%w: duplicate hash", ErrChainBreak)
		   }
		   seen[lk.Hash] = struct{}{}
		  if i > 0 && links[i].PlanID != links[0].PlanID {
			  return fmt.Errorf("%w: plan id drift", ErrChainBreak)
		  }
		  if err := coldLinkOK(lk); err != nil {
			  return err
		  }
	   }
	   return nil
   }

  func coldLinkOK(lk ProofLink) error {
	   if lk.From != "cold" && lk.To != "cold" {
		   if lk.CeremonyID != "" || lk.AirgapID != "" || lk.SignID != "" || lk.CeremonyPhase != "" || lk.AirgapPhase != "" || lk.SignState != "" {
			   if lk.State == "genesis" {
				  return nil
			   }
			  return fmt.Errorf("%w: ref on warm leg", ErrRefNotCold)
		  }
		  return nil
	   }
	   switch lk.State {
	  case "ref_bound", string(LegReady), string(LegExecuting), string(LegDone), string(LegCompensated):
		  if lk.CeremonyID == "" || lk.AirgapID == "" || lk.SignID == "" {
			   return fmt.Errorf("%w: cold link missing ref", ErrColdRef)
		   }
		   if err := phasesFrozen(lk); err != nil {
			  return err
		   }
	   }
	   return nil
   }

// phasesFrozen 冷环必须带上绑定时的阶段。空阶段或非激活阶段不能靠编号蒙混。
  func phasesFrozen(lk ProofLink) error {
	   if lk.CeremonyPhase != ceremonyPhaseOK {
		   return fmt.Errorf("%w: ceremony phase not frozen", ErrColdRef)
	   }
	  if _, ok := airgapPhaseOK[lk.AirgapPhase]; !ok {
		   return fmt.Errorf("%w: airgap phase not frozen", ErrColdRef)
	   }
	  if _, ok := signStateOK[lk.SignState]; !ok {
		   return fmt.Errorf("%w: sign state not frozen", ErrColdRef)
	   }
	   return nil
   }

   func (o *Orch) linksOf(planID string) []ProofLink {
	   src := o.links[planID]
	   out := make([]ProofLink, len(src))
	  copy(out, src)
	  return out
   }

   func (o *Orch) appendLink(p *SweepPlan, legID string, legSeq int, kind, from, to, amount, state, actor string, refs RefBind, now time.Time) error {
	   if p == nil {
		   return ErrUnavailable
	  }
	  seq := len(o.links[p.ID])
	  prev := genesisPrev
	  if seq > 0 {
		  prev = o.links[p.ID][seq-1].Hash
		   if p.Head != prev {
			  return fmt.Errorf("%w: head drift", ErrChainBreak)
		   }
	  }
	  if actor == "" {
		   actor = laneActor
	   }
	   lk := sealLink(prev,
		  p.ID, legID, seq, legSeq, kind, from, to, amount, state, refs.CeremonyID, refs.AirgapID, refs.SignID, refs.CeremonyPhase, refs.AirgapPhase, refs.SignState, actor, now)
	  chain := append(append([]ProofLink{}, o.links[p.ID]...), lk)
	  if err := VerifyLinks(chain); err != nil {
		   return err
	   }
	  o.links[p.ID] = chain
	  p.Head = lk.Hash
	  p.UpdatedAt = now
	   return nil
  }
