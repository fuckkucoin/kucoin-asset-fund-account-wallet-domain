package sweeprun

import "time"

type PlanState string

const (
	PlanStateDraft PlanState = "draft"
	PlanStateGated PlanState = "gated"
	PlanStateArmed PlanState = "armed"
	PlanStateRunning PlanState = "running"
	PlanStatePartial PlanState = "partial"
	PlanStateCompensating PlanState = "compensating"
	PlanStateCompensated PlanState = "compensated"
	PlanStateSettled  PlanState = "settled"
	PlanStateHalted   PlanState = "halted"
	PlanStateRejected PlanState = "rejected"
)

  func (s PlanState) Terminal() bool {
	  return s == PlanStateSettled || s == PlanStateCompensated || s == PlanStateRejected
   }

type LegState string

const (
	LegPending      LegState = "pending"
	LegProofOpen    LegState = "proof_open"
	LegCeremonyWait LegState = "ceremony_wait"
	LegReady        LegState = "ready"
	LegExecuting    LegState = "executing"
	LegDone         LegState = "done"
	LegFailed       LegState = "failed"
	LegCompensating LegState = "compensating"
	LegCompensated LegState = "compensated"
	LegSkipped     LegState = "skipped"
)

// Quiet 表示拍子不应再推这条腿。补偿中的原腿等反向腿，自身不再前进。
  func (s LegState) Quiet() bool {
	   switch s {
	  case LegDone, LegSkipped, LegFailed, LegCompensating, LegCompensated:
		   return true
	  default:
		  return false
	   }
  }

   func (s LegState) Terminal() bool {
	  return s == LegSkipped || s == LegFailed || s == LegCompensated
  }

type RefBind struct {
	CeremonyID string `json:"ceremony_id,omitempty"`
	AirgapID   string `json:"airgap_id,omitempty"`
	SignID        string `json:"sign_id,omitempty"`
	CeremonyPhase string `json:"ceremony_phase,omitempty"`
	AirgapPhase   string `json:"airgap_phase,omitempty"`
	SignState string `json:"sign_state,omitempty"`
	BoundBy string    `json:"bound_by,omitempty"`
	BoundAt time.Time `json:"bound_at,omitempty"`
}

   func (r RefBind) Bound() bool {
	  return r.CeremonyID != "" && r.AirgapID != "" && r.SignID != ""
  }

   func (r RefBind) Clear() bool {
	   return r.CeremonyID == "" && r.AirgapID == "" && r.SignID == ""
   }

type SweepLeg struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	Seq    int    `json:"seq"`
	Kind    string `json:"kind"`
	SrcKind string `json:"src_kind,omitempty"`
	From    string `json:"from"`
	To string `json:"to"`
	Amount     string   `json:"amount"`
	Cold       bool     `json:"cold"`
	State      LegState `json:"state"`
	ParentID   string   `json:"parent_id,omitempty"`
	Compensate bool     `json:"compensate,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Refs RefBind `json:"refs,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (l SweepLeg) Copy() SweepLeg {
	   cp := l
	  return cp
  }

type SweepPlan struct {
	ID          string `json:"id"`
	Idem        string `json:"idempotency_key"`
	Fingerprint string `json:"fingerprint"`
	Chain  string    `json:"chain"`
	Asset  string    `json:"asset"`
	State  PlanState `json:"state"`
	Actor  string    `json:"actor"`
	Reason string    `json:"reason,omitempty"`
	Code   string    `json:"code,omitempty"`
	Held   string    `json:"held,omitempty"`
	Head   string    `json:"head,omitempty"`
	ColdDeadline time.Time `json:"cold_deadline,omitempty"`
	ColdWait string `json:"cold_wait,omitempty"`
	Legs      []SweepLeg `json:"legs,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (p SweepPlan) Copy() SweepPlan {
	   cp := p
	   if p.Legs != nil {
		  cp.Legs = make([]SweepLeg, len(p.Legs))
		   for i := range p.Legs {
			  cp.Legs[i] = p.Legs[i].Copy()
		   }
	   }
	  return cp
  }

  func (p *SweepPlan) legByID(id string) *SweepLeg {
	   if p == nil {
		   return nil
	   }
	   for i := range p.Legs {
		   if p.Legs[i].ID == id {
			   return &p.Legs[i]
		   }
	  }
	  return nil
   }

  func (p *SweepPlan) activeLeg() *SweepLeg {
	   if p == nil {
		  return nil
	   }
	   for i := range p.Legs {
		   if p.Legs[i].State.Quiet() {
			  continue
		   }
		  return &p.Legs[i]
	  }
	   return nil
  }

  func (p *SweepPlan) needsCold() bool {
	  if p == nil {
		   return false
	   }
	  for i := range p.Legs {
		  if p.Legs[i].Cold && !p.Legs[i].Compensate && !p.Legs[i].State.Terminal() && p.Legs[i].State != LegDone && p.Legs[i].State != LegCompensating {
			   if !p.Legs[i].Refs.Bound() {
				  return true
			  }
		   }
	   }
	  return false
   }

   func (p *SweepPlan) coldSatisfied() bool {
	  for i := range p.Legs {
		   lg := &p.Legs[i]
		   if !lg.Cold {
			  continue
		  }
		   if lg.State.Terminal() || lg.State == LegDone || lg.State == LegCompensating || lg.State == LegCompensated {
			  continue
		   }
		   if !lg.Refs.Bound() {
			  return false
		  }
	  }
	   return true
  }

  func (p *SweepPlan) forwardOpen() bool {
	   for i := range p.Legs {
		   lg := &p.Legs[i]
		   if lg.Compensate {
			   continue
		  }
		   if lg.State != LegDone && lg.State != LegSkipped && lg.State != LegFailed && lg.State != LegCompensating && lg.State != LegCompensated {
			   return true
		  }
	  }
	   return false
  }

  func (p *SweepPlan) doneForward() []string {
	   var ids []string
	  for i := range p.Legs {
		  lg := &p.Legs[i]
		  if lg.Compensate {
			  continue
		  }
		  if lg.State == LegDone {
			   ids = append(ids, lg.ID)
		   }
	  }
	  return ids
  }

type ProofLink struct {
	PlanID string `json:"plan_id"`
	Seq    int    `json:"seq"`
	Prev   string `json:"prev"`
	Hash string `json:"hash"`
	LegID  string `json:"leg_id"`
	LegSeq int    `json:"leg_seq"`
	Kind   string `json:"kind"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Amount     string `json:"amount"`
	State      string `json:"state"`
	CeremonyID string `json:"ceremony_id,omitempty"`
	AirgapID string `json:"airgap_id,omitempty"`
	SignID        string `json:"sign_id,omitempty"`
	CeremonyPhase string `json:"ceremony_phase,omitempty"`
	AirgapPhase string `json:"airgap_phase,omitempty"`
	SignState string    `json:"sign_state,omitempty"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
}

   func (l ProofLink) Copy() ProofLink { return l }

type CompRec struct {
	ID        string `json:"id"`
	PlanID    string `json:"plan_id"`
	ParentLeg string `json:"parent_leg"`
	CompLeg string `json:"comp_leg"`
	Amount    string    `json:"amount"`
	State     string    `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type RefVerdict struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
	Phase  string `json:"ceremony_phase,omitempty"`
	Airgap string `json:"airgap_phase,omitempty"`
	Sign   string `json:"sign_state,omitempty"`
}

type TickReport struct {
	Advanced int `json:"advanced"`
	Waited int      `json:"waited"`
	Halted bool     `json:"halted"`
	Plans  []string `json:"plans,omitempty"`
}

const (
	genesisPrev = "GENESIS"
	laneActor   = "sweep-orch"
	kindPlan    = "plan"
	kindComp    = "compensate"
)

   func clipReason(s string) string {
	   if len(s) <= 240 {
		  return s
	   }
	  return s[:240]
   }
