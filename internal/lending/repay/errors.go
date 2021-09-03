package repay

import "errors"

var (
	ErrUnavailable = errors.New("repay offset unavailable")
	ErrAmount      = errors.New("repay amount invalid")
	ErrExcess    = errors.New("repay exceeds owed")
	ErrLiability = errors.New("liability not found")
	ErrIdem      = errors.New("repay idempotency key required")
)
