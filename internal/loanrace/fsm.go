package loanrace

import "account-assets.kucoin.net/funding/internal/fsm"

// ──────────────────────────────────────────────────
// 贷款竞态 FSM
// active ──overdue_detected──▸ overdue
// overdue ──freeze_applied──▸ frozen
// frozen ──repay_start──▸ repaying
// frozen ──writeoff_applied──▸ written_off
// repaying ──repay_done──▸ closed
// repaying ──repay_fail──▸ frozen
// written_off ──(terminal)
// closed ──(terminal)
// ──────────────────────────────────────────────────

const (
	lrEvOverdueDetected = "overdue_detected"
	lrEvFreezeApplied = "freeze_applied"
	lrEvRepayStart = "repay_start"
	lrEvRepayDone  = "repay_done"
	lrEvRepayFail = "repay_fail"
	lrEvWriteoffApplied = "writeoff_applied"
)

   func LoanRaceFSM() *fsm.Machine[LoanRaceState, string] {
	  return fsm.New([]fsm.Edge[LoanRaceState, string]{
		  {From: LRActive, Event: lrEvOverdueDetected, To: LROverdue},
		  {From: LROverdue, Event: lrEvFreezeApplied, To: LRFrozen},
		  {From: LRFrozen, Event: lrEvRepayStart, To: LRRepaying},
		   {From: LRFrozen, Event: lrEvWriteoffApplied, To: LRWrittenOff},
		  {From: LRRepaying,
			   Event: lrEvRepayDone, To: LRClosed},
		   {From: LRRepaying, Event: lrEvRepayFail, To: LRFrozen},
	  }, LRClosed, LRWrittenOff)
   }

// ──────────────────────────────────────────────────
// 冻结租约 FSM
// acquired ──apply──▸ applied
// acquired ──expire──▸ expired
// applied ──release──▸ released
// applied ──expire──▸ expired
// ──────────────────────────────────────────────────

const (
	leaseEvApply   = "apply"
	leaseEvRelease = "release"
	leaseEvExpire = "expire"
)

   func FreezeLeaseFSM() *fsm.Machine[LeaseState, string] {
	   return fsm.New([]fsm.Edge[LeaseState, string]{
		   {From: LeaseAcquired, Event: leaseEvApply, To: LeaseApplied},
		   {From: LeaseAcquired,
			  Event: leaseEvExpire, To: LeaseExpired},
		   {From: LeaseApplied, Event: leaseEvRelease, To: LeaseReleased},
		   {From: LeaseApplied, Event: leaseEvExpire, To: LeaseExpired},
	  }, LeaseReleased, LeaseExpired)
   }

// ──────────────────────────────────────────────────
// PayGate FSM
// open ──clear──▸ cleared
// open ──block──▸ blocked
// cleared ──close──▸ closed
// ──────────────────────────────────────────────────

const (
	gateEvClear = "clear"
	gateEvBlock = "block"
	gateEvClose = "close"
)

   func PayGateFSM() *fsm.Machine[GateState, string] {
	  return fsm.New([]fsm.Edge[GateState, string]{
		  {From: GateOpen, Event: gateEvClear, To: GateCleared},
		   {From: GateOpen, Event: gateEvBlock, To: GateBlocked},
		   {From: GateCleared,
			  Event: gateEvClose, To: GateClosed},
	  }, GateClosed, GateBlocked)
   }

// ──────────────────────────────────────────────────
// Writeoff FSM
// pending ──approve──▸ approved
// pending ──reject──▸ rejected
// approved ──apply──▸ applied
// ──────────────────────────────────────────────────

const (
	woEvApprove = "approve"
	woEvReject  = "reject"
	woEvApply = "apply"
)

  func WriteoffFSM() *fsm.Machine[WriteoffState, string] {
	  return fsm.New([]fsm.Edge[WriteoffState, string]{
		   {From: WOPending, Event: woEvApprove, To: WOApproved},
		  {From: WOPending, Event: woEvReject, To: WORejected},
		  {From: WOApproved,
			   Event: woEvApply, To: WOApplied},
	  }, WOApplied, WORejected)
  }

// ──────────────────────────────────────────────────
// UnfreezeProof FSM
// pending ──validate──▸ valid
// pending ──reject──▸ rejected
// ──────────────────────────────────────────────────

const (
	proofEvValidate = "validate"
	proofEvReject = "reject"
)

  func UnfreezeProofFSM() *fsm.Machine[ProofState, string] {
	  return fsm.New([]fsm.Edge[ProofState, string]{
		  {From: ProofPending, Event: proofEvValidate, To: ProofValid},
		   {From: ProofPending, Event: proofEvReject, To: ProofRejected},
	  }, ProofValid, ProofRejected)
   }
