package assetworker

import "fmt"

var (
	ErrPump        = fmt.Errorf("asset-worker pump unavailable")
	ErrHandler     = fmt.Errorf("asset-worker handler required")
	ErrDupAttach   = fmt.Errorf("asset-worker duplicate attach")
	ErrNotAttached = fmt.Errorf("asset-worker handler not attached")
	ErrKind        = fmt.Errorf("asset-worker job kind unknown")
	ErrRestore  = fmt.Errorf("asset-worker restore failed")
	ErrAssembly = fmt.Errorf("asset-worker assembly failed")
)
