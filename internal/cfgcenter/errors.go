package cfgcenter

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("config center unavailable")
	ErrVersion     = fmt.Errorf("config version invalid")
	ErrPinned      = fmt.Errorf("config version pinned")
	ErrKilled      = fmt.Errorf("config kill switch engaged")
	ErrName        = fmt.Errorf("config pack name required")
	ErrLimit = fmt.Errorf("config limit invalid")
	ErrStale = fmt.Errorf("config version stale")
)
