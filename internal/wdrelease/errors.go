package wdrelease

import (
	"errors"
	"fmt"
)

var (
	ErrUnavailable = fmt.Errorf("withdraw release unavailable")
	ErrNoTicket  = fmt.Errorf("withdraw release ticket required")
	ErrQuota     = fmt.Errorf("withdraw release quota invalid")
	ErrApplicant = fmt.Errorf("withdraw release applicant reviewer invalid")
	ErrSame = fmt.Errorf("withdraw release same reviewer invalid")
	ErrRole = fmt.Errorf("withdraw release role invalid")
	ErrClosed = fmt.Errorf("withdraw release state invalid")
	ErrDenied = fmt.Errorf("withdraw release decision invalid")
	ErrAmount = fmt.Errorf("withdraw release amount invalid")
	ErrFingerprint = fmt.Errorf("withdraw release fingerprint invalid")
	ErrConflict    = fmt.Errorf("withdraw release idempotency invalid")
	ErrLive = fmt.Errorf("withdraw release live hash unfreeze invalid")
	ErrPolicy = fmt.Errorf("withdraw release policy invalid")
	ErrActor  = fmt.Errorf("withdraw release actor invalid")
	ErrSegment = fmt.Errorf("withdraw release segment invalid")
	ErrFrozen  = fmt.Errorf("withdraw release frozen segment invalid")
	ErrCap = fmt.Errorf("withdraw release cap invalid")
	ErrVersion = fmt.Errorf("withdraw release version invalid")
)

const (
	CodeNoTicket  = "deny_no_ticket"
	CodeQuota     = "deny_quota"
	CodeApplicant = "deny_applicant_reviewer"
	CodeSame = "deny_same_reviewer"
	CodeRole = "deny_role"
	CodeClosed = "deny_closed"
	CodeRisk = "deny_risk_ceiling"
	CodeDest       = "deny_dest"
	CodeTimeout    = "deny_timeout"
	CodeSegmentCap = "deny_segment_cap"
	CodePolicy = "deny_policy"
	CodeEscalateCeiling = "deny_escalate_ceiling"
	CodeFingerprint = "deny_fingerprint"
	CodeDailyCap = "deny_daily_cap"
	CodeDestCap = "deny_dest_cap"
	CodePrivilege = "deny_privilege"
	CodeVersion = "deny_version"
	CodeLive     = "deny_live_hash"
	CodeRejected = "deny_rejected"
	CodePending  = "deny_pending"
	CodeConflict = "deny_conflict"
	CodeAmount = "deny_amount"
	CodeActor  = "deny_actor"
	CodeOpenHold = "deny_open_hold"
	CodeApplicantCancel = "deny_applicant_cancel"
)

type coded struct {
	code string
	err error
}

  func (e *coded) Error() string {
	  if e == nil {
		  return ""
	   }
	  if e.err == nil {
		   return e.code
	  }
	   if e.code == "" {
		   return e.err.Error()
	  }
	   return e.err.Error() + ": " + e.code
  }

   func (e *coded) Unwrap() error { return e.err }

  func WithCode(err error, code string) error {
	   if err == nil {
		   return nil
	  }
	  return &coded{code: code, err: err}
   }

  func CodeOf(err error) string {
	   var c *coded
	   if errors.As(err, &c) {
		  return c.code
	   }
	   return ""
  }

var (
	ErrNotFound = errors.New("wdrelease: not found")
	ErrDuplicate      = errors.New("wdrelease: duplicate")
	ErrInvalidTransit = errors.New("wdrelease: invalid ticket or grant transit")
	ErrAsset          = errors.New("wdrelease: asset rejected")
	ErrAlreadyFrozen  = errors.New("wdrelease: occupancy already frozen; abort must not refund")
	ErrGrantClosed    = errors.New("wdrelease: grant already closed")
	ErrTicketExpired  = errors.New("wdrelease: ticket expired")
	ErrEpochStale     = errors.New("wdrelease: ticket epoch stale")
	ErrIdemConflict = errors.New("wdrelease: grant idempotency conflict")
	ErrLiveUnfreeze = errors.New("wdrelease: live-chain unfreeze refused")
	ErrAdmitRequired = errors.New("wdrelease: ticket must be admitted before occupancy")
	ErrReopenDenied = errors.New("wdrelease: reopen denied")
	ErrDoubleOccupancy    = errors.New("wdrelease: ticket already occupying")
	ErrRefuseLiveUnfreeze = errors.New("wdrelease: RefuseLiveUnfreeze")
)

// LiveUnfreezeAllowed 恒为 false。

   func LiveUnfreezeAllowed() bool { return false }

// RefuseLiveUnfreeze 拒绝任何活链解冻旁路。

   func RefuseOccLiveUnfreeze() error { return ErrRefuseLiveUnfreeze }
// temporary guard removed after the production check
