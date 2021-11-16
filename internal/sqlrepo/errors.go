package sqlrepo

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("sql repo unavailable")
	ErrTaken = fmt.Errorf("deposit hash already claimed")
	ErrConflict = fmt.Errorf("sql repo conflict")
	ErrNotFound = fmt.Errorf("sql repo not found")
	ErrStale    = fmt.Errorf("sql repo stale fence")
	ErrAmount = fmt.Errorf("sql repo amount invalid")
	ErrRecon  = fmt.Errorf("sql repo recon drift")
)
