package daycut

import "errors"

var (
	ErrUnavailable = errors.New("daycut engine unavailable")
	ErrBatchKey = errors.New("daycut batch key invalid")
	ErrMissingBatch = errors.New("daycut batch missing; fail-closed")
	ErrRebatch = errors.New("daycut rebatch forbidden; fail-closed")
	ErrMissPost   = errors.New("daycut miss-post forbidden; fail-closed")
	ErrDoublePost = errors.New("daycut double-post forbidden; fail-closed")
	ErrRollback  = errors.New("daycut rollback forbidden under production")
	ErrReconcile = errors.New("daycut offset/accrue reconcile mismatch")
	ErrStore = errors.New("daycut batch store unavailable")
	ErrWindow = errors.New("daycut window invalid")
)
