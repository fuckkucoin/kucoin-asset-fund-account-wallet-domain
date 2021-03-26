package coolwingate

import "fmt"

var (
	ErrEngineNil = fmt.Errorf("coolwingate engine not initialized")
	ErrUserRequired = fmt.Errorf("coolwingate user_id required")
	ErrWithdrawRequired = fmt.Errorf("coolwingate withdraw_id required")
	ErrReasonRequired    = fmt.Errorf("coolwingate reason required")
	ErrBindTokenRequired = fmt.Errorf("coolwingate bind_token required")
	ErrLeaseNotFound     = fmt.Errorf("coolwingate lease not found")
	ErrBindNotFound = fmt.Errorf("coolwingate bind not found")
	ErrCaseNotFound = fmt.Errorf("coolwingate case not found")
	ErrProofNotFound = fmt.Errorf("coolwingate proof not found")
	ErrLeaseExpired = fmt.Errorf("coolwingate lease expired")
	ErrBindRevoked  = fmt.Errorf("coolwingate preflight bind revoked")
	ErrBindConsumed = fmt.Errorf("coolwingate preflight bind already consumed")
	ErrBindExpired = fmt.Errorf("coolwingate preflight bind expired")
	ErrGenMismatch = fmt.Errorf("coolwingate cool generation mismatch")
	ErrCacheBypass = fmt.Errorf("coolwingate local cache bypass rejected; use ledger")
	ErrLockWindowActive  = fmt.Errorf("coolwingate withdraw lock window active")
	ErrCaseNotProven     = fmt.Errorf("coolwingate case must be proven before close")
	ErrCaseAlreadyClosed = fmt.Errorf("coolwingate case already closed")
	ErrFSMTransition = fmt.Errorf("coolwingate FSM transition not allowed")
	ErrWorkerDisabled = fmt.Errorf("coolwingate worker disabled")
	ErrDuplicateBind  = fmt.Errorf("coolwingate duplicate preflight bind for withdraw")
	ErrConcurrentMark = fmt.Errorf("coolwingate concurrent mark during require window")
	ErrBroadcastBlocked = fmt.Errorf("coolwingate broadcast blocked: bind revoked after mark")
)
