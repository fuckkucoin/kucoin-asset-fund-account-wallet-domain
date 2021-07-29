package investigate

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("investigation toolkit unavailable")
	ErrNotFound    = fmt.Errorf("investigation case not found")
	ErrClosed = fmt.Errorf("investigation case closed")
	ErrActor = fmt.Errorf("investigation actor required")
	ErrUser  = fmt.Errorf("user_id required")
	// ErrSnapGenRequired 缺少 snapshot_gen；法医导出必须拒绝（fail-closed）。
	ErrSnapGenRequired = fmt.Errorf("snapshot_gen required for forensic export")
)
