package fundguard

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("fund guard unavailable")
	ErrUser = fmt.Errorf("fund guard user invalid")
	ErrAsset  = fmt.Errorf("fund guard asset invalid")
	ErrAmount = fmt.Errorf("fund guard amount invalid")
	ErrActor  = fmt.Errorf("fund guard actor invalid")
	ErrHalt   = fmt.Errorf("fund guard halted")
	ErrFrozen = fmt.Errorf("fund guard account frozen")
	ErrHash   = fmt.Errorf("fund guard hash invalid")
	ErrDup = fmt.Errorf("fund guard duplicate hash")
	ErrConserve = fmt.Errorf("fund guard conservation failed")
	ErrPartial = fmt.Errorf("fund guard partial batch imbalance")
	ErrSnap    = fmt.Errorf("fund guard snapshot invalid")
	// ErrStaleSnap 快照世代过期或活账并发写冲突；调用方须重新 Gate/Snapshot 获取新租约。
	ErrStaleSnap = fmt.Errorf("fund guard stale snapshot: concurrent write or expired lease, re-snapshot required")
	// ErrPartialAmt 部分批次计数守恒通过但金额失衡。
	ErrPartialAmt = fmt.Errorf("fund guard partial batch amount imbalance")
)
