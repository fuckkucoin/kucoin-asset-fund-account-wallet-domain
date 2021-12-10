package stmtexpiry

import "account-assets.kucoin.net/funding/internal/fsm"

const (
	EvIssue  = "issue"
	EvExpire = "expire"
	EvRevoke = "revoke"
	EvProve = "prove"
	EvClose = "close"
)

  func ExportFSM() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		   {From: string(LeaseBuilt), Event: EvIssue, To: string(LeaseIssued)},
		   {From: string(LeaseBuilt), Event: EvRevoke, To: string(LeaseRevoked)},
		  {From: string(LeaseIssued), Event: EvExpire, To: string(LeaseExpired)},
		   {From: string(LeaseIssued), Event: EvRevoke, To: string(LeaseRevoked)},
	   }, string(LeaseExpired), string(LeaseRevoked))
   }

   func DriftFSM() *fsm.Machine[string, string] {
	   return fsm.New([]fsm.Edge[string, string]{
		  {From: string(DriftOpen), Event: EvProve, To: string(DriftProven)},
		  {From: string(DriftProven),
			   Event: EvClose, To: string(DriftClosed)},
	   }, string(DriftClosed))
  }

// ValidateLeaseTransition 校验租约状态转换是否合法。
   func ValidateLeaseTransition(from LeaseState, ev string) (LeaseState, error) {
	  m := ExportFSM()
	  next, err := m.Apply(string(from), ev)
	  if err != nil {
		   return from, err
	   }
	   return LeaseState(next), nil
   }

// ValidateDriftTransition 校验漂移案例状态转换是否合法。
  func ValidateDriftTransition(from DriftState, ev string) (DriftState, error) {
	  m := DriftFSM()
	  next, err := m.Apply(string(from), ev)
	   if err != nil {
		  return from, err
	   }
	   return DriftState(next), nil
   }
