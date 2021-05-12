package feeresgate

import "fmt"

var (
	ErrEngineNil  = fmt.Errorf("feeresgate engine not initialized")
	ErrReqInvalid = fmt.Errorf("feeresgate request invalid")
	ErrConcurrentBind = fmt.Errorf("feeresgate concurrent terminal bind")
	ErrProofExport    = fmt.Errorf("feeresgate proof export failed")
	ErrWorkerDisabled = fmt.Errorf("feeresgate worker disabled")
	ErrScanEmpty = fmt.Errorf("feeresgate scan returned no results")
)
