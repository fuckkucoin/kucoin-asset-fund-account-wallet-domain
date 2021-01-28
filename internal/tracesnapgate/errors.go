package tracesnapgate

import "errors"

var (
	// ErrNoSnapshot 调查导出缺失 snapshot_gen；fail-closed，必须拒绝。
	ErrNoSnapshot = errors.New("trace_snapshot_gen required for forensic export")
	// ErrSealedAlready 快照已密封，不可重复 Seal。
	ErrSealedAlready = errors.New("trace snapshot already sealed")
	// ErrNotOpen 快照不在 open 状态，无法执行当前操作。
	ErrNotOpen = errors.New("trace snapshot not in open state")
	// ErrNotSealed 快照尚未密封，无法执行 Replay / Export。
	ErrNotSealed = errors.New("trace snapshot not sealed")
	// ErrDriftDetected 重放时摘要根不一致，已落 TraceDriftProof；不得返回活账视图。
	ErrDriftDetected = errors.New("trace snapshot drift detected: live state diverges from sealed digest")
	// ErrDigestMismatch 传入摘要与已密封摘要不符。
	ErrDigestMismatch = errors.New("digest mismatch against sealed snapshot root")
	// ErrNotFound 快照/证明/绑定记录不存在。
	ErrNotFound = errors.New("tracesnapgate record not found")
	// ErrCaseBound 调查案已绑定快照，不可重复绑定。
	ErrCaseBound = errors.New("investigate case already bound to a trace snapshot")
	// ErrInvalidUserID 用户 ID 为空。
	ErrInvalidUserID = errors.New("user_id required")
	// ErrInvalidAsset 资产代码为空。
	ErrInvalidAsset = errors.New("asset required")
	// ErrSuperseded 快照已被取代（superseded），不可导出。
	ErrSuperseded = errors.New("trace snapshot superseded: use latest sealed snapshot")
	// ErrLiveMode 请求携带 live 标记，结果为非法医视图，不得作为法证据。
	ErrLiveMode = errors.New("live mode result is non-forensic: snapshot_gen absent")
	// ErrEmptyHops 划转 hop 列表为空，无法生成摘要根。
	ErrEmptyHops = errors.New("transfer hops list is empty")
	// ErrStoreFull 内存存储已满（仅场景）。
	ErrStoreFull = errors.New("tracesnapgate store capacity exceeded")
)
