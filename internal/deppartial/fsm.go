package deppartial

import "account-assets.kucoin.net/funding/internal/fsm"

// ──────────────────────────────────────────────────
// 批次残余 FSM
// submitted ──residue_found──▸ residue_open
// residue_open ──reopen──▸ reopening
// reopening ──resolve_done──▸ closed
// reopening ──residue_found──▸ residue_open
// residue_open ──force_close──▸ closed
// ──────────────────────────────────────────────────

const (
	brEvResidueFound = "residue_found"
	brEvReopen = "reopen"
	brEvResolveDone = "resolve_done"
	brEvForceClose  = "force_close"
)

   func BatchResidueFSM() *fsm.Machine[BatchResidueState, string] {
	   return fsm.New([]fsm.Edge[BatchResidueState, string]{
		   {From: BRSubmitted, Event: brEvResidueFound, To: BRResidueOpen},
		  {From: BRResidueOpen, Event: brEvReopen, To: BRReopening},
		  {From: BRReopening, Event: brEvResolveDone, To: BRClosed},
		  {From: BRReopening, Event: brEvResidueFound, To: BRResidueOpen},
		   {From: BRResidueOpen, Event: brEvForceClose, To: BRClosed},
	   }, BRClosed)
   }

// ──────────────────────────────────────────────────
// 残余行 FSM
// open ──credit_repair──▸ credited
// open ──abandon──▸ abandoned
// open ──escalate──▸ escalated
// escalated ──credit_repair──▸ credited
// escalated ──abandon──▸ abandoned
// ──────────────────────────────────────────────────

const (
	resEvCreditRepair = "credit_repair"
	resEvAbandon      = "abandon"
	resEvEscalate = "escalate"
)

  func ResidueFSM() *fsm.Machine[ResidueState, string] {
	  return fsm.New([]fsm.Edge[ResidueState, string]{
		  {From: ResOpen, Event: resEvCreditRepair, To: ResCredited},
		  {From: ResOpen, Event: resEvAbandon, To: ResAbandoned},
		   {From: ResOpen, Event: resEvEscalate, To: ResEscalated},
		  {From: ResEscalated, Event: resEvCreditRepair, To: ResCredited},
		  {From: ResEscalated, Event: resEvAbandon, To: ResAbandoned},
	   }, ResCredited, ResAbandoned)
  }

// ──────────────────────────────────────────────────
// 租约 FSM
// held ──release──▸ released
// held ──expire──▸ expired
// ──────────────────────────────────────────────────

const (
	leaseEvRelease = "release"
	leaseEvExpire  = "expire"
)

   func LeaseFSM() *fsm.Machine[LeaseStatus, string] {
	  return fsm.New([]fsm.Edge[LeaseStatus, string]{
		  {From: LeaseHeld, Event: leaseEvRelease, To: LeaseReleased},
		  {From: LeaseHeld, Event: leaseEvExpire, To: LeaseExpired},
	  }, LeaseReleased, LeaseExpired)
  }

// ──────────────────────────────────────────────────
// 一致证明 FSM
// pending ──pass──▸ passed
// pending ──reject──▸ rejected
// ──────────────────────────────────────────────────

const (
	proofEvPass   = "pass"
	proofEvReject = "reject"
)

  func ProofFSM() *fsm.Machine[ProofStatus, string] {
	  return fsm.New([]fsm.Edge[ProofStatus, string]{
		  {From: ProofPending, Event: proofEvPass, To: ProofPassed},
		  {From: ProofPending,
			   Event: proofEvReject, To: ProofRejected},
	  }, ProofPassed, ProofRejected)
  }
