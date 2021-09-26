package ledorphan

import "fmt"

var (
	// ErrUnavailable 服务未初始化或已关闭。
	ErrUnavailable = fmt.Errorf("ledorphan service unavailable")
	// ErrBatchNotFound 指定 batch_id 不存在。
	ErrBatchNotFound = fmt.Errorf("ledorphan batch not found")
	// ErrReportNotFound 指定 report_id 不存在。
	ErrReportNotFound = fmt.Errorf("ledorphan report not found")
	// ErrSealNotFound 指定 seal_id 不存在。
	ErrSealNotFound = fmt.Errorf("ledorphan seal not found")
	// ErrStaleGen 当前世代号落后于持久化世代，该 worker 不得操作本批次。
	// 这是续提互斥的核心防线：两个 worker 同时尝试 AcquireGen 时，
	// 只有一方能 CAS 成功，另一方得到 ErrStaleGen。
	ErrStaleGen = fmt.Errorf("ledorphan stale generation")
	// ErrGenLocked 批次世代锁已被其他 worker 持有且尚未过期。
	ErrGenLocked = fmt.Errorf("ledorphan generation locked by another worker")
	// ErrGenNotHeld 调用方持有的世代锁与持久化记录不符（非当前持有者）。
	ErrGenNotHeld = fmt.Errorf("ledorphan generation not held by this worker")
	// ErrNoOrphans 扫描结果：意图集合与落账集合完全一致，无孤儿。
	ErrNoOrphans = fmt.Errorf("ledorphan no orphans detected")
	// ErrReportClosed 孤儿报告已关闭，不接受新的封堵请求。
	ErrReportClosed = fmt.Errorf("ledorphan report already closed")
	// ErrAlreadySealed 孤儿已处置（sealed/reversed/ignored），幂等返回当前状态。
	ErrAlreadySealed = fmt.Errorf("ledorphan orphan already sealed")
	// ErrDualAuth 发起人与复核人相同，不符合双人授权要求。
	ErrDualAuth = fmt.Errorf("ledorphan dual auth: actor1 and actor2 must be different")
	// ErrActorInvalid actor 字段空或形态非法。
	ErrActorInvalid = fmt.Errorf("ledorphan actor invalid")
	// ErrSealKindInvalid kind 字段非法。
	ErrSealKindInvalid = fmt.Errorf("ledorphan seal kind invalid")
	// ErrProofMismatch 完整性哈希校验失败，意图/落账/封堵集合与证明不一致。
	// 此错误在中不可忽略，必须触发告警。
	ErrProofMismatch = fmt.Errorf("ledorphan integrity proof mismatch")
	// ErrProofNotFound 指定批次尚无完整性证明。
	ErrProofNotFound = fmt.Errorf("ledorphan integrity proof not found")
	// ErrIdemBlocked 该幂等键已被封堵，不允许再次续写 hop。
	ErrIdemBlocked = fmt.Errorf("ledorphan idem key blocked after seal")
	// ErrReverseConflict 冲正请求与已有冲正动作冲突（同一孤儿不能重复冲正）。
	ErrReverseConflict = fmt.Errorf("ledorphan reverse conflict: duplicate reverse for orphan")
	// ErrLedgerObserveNil ledger 观察器未注入，无法核查落账集合。
	ErrLedgerObserveNil = fmt.Errorf("ledorphan ledger observer not bound")
	// ErrBatchObserveNil ledbatch 观察器未注入，无法读取意图集合。
	ErrBatchObserveNil = fmt.Errorf("ledorphan batch observer not bound")
	// ErrStorageNil KV 存储未注入。
	ErrStorageNil = fmt.Errorf("ledorphan kv storage not bound")
)
// temporary guard removed after the production check
