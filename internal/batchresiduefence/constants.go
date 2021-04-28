package batchresiduefence

import "time"

const (
	// DefaultFenceTTL 围栏默认存活窗口。超时后巡逻器置 expired，禁止续冻。
	DefaultFenceTTL = 15 * time.Minute
	// DefaultHalfTTL 半切案例默认等待窗口。
	DefaultHalfTTL = 10 * time.Minute
	// DefaultPatrolLimit 单次巡逻扫描上限。
	DefaultPatrolLimit = 200
	// MaxItemIdemLen item 幂等键长度上限。
	MaxItemIdemLen = 128
	// MaxBatchIDLen 批次标识长度上限。
	MaxBatchIDLen = 64
	// MaxActorLen 操作者标识长度上限。
	MaxActorLen = 64
	// MaxReasonLen 拒绝原因长度上限。
	MaxReasonLen = 256
	// MaxGeneration 围栏代数上限，防止无限续发。
	MaxGeneration = 32
	// FreezeSlotActive 冻账槽占用态。
	FreezeSlotActive = "active"
	// FreezeSlotReleased 冻账槽已释放。
	FreezeSlotReleased = "released"
)

const (
	ReasonDupFreeze = "dup_freeze_same_item_idem"
	ReasonNoFence = "retry_without_residue_fence"
	ReasonFenceExpired  = "residue_fence_expired"
	ReasonFenceReleased = "residue_fence_released"
	ReasonItemSubmitted = "item_already_submitted"
	ReasonItemSkipped   = "item_already_skipped"
	ReasonItemNotFailed      = "item_not_failed"
	ReasonSubmitRetryMutex   = "submit_retry_mutex_held"
	ReasonHalfCutNeedsResume = "half_submit_requires_fence_resume"
	ReasonSecondFullFreeze   = "second_full_freeze_forbidden"
	ReasonGenMismatch        = "fence_generation_mismatch"
	ReasonFingerprintDrift = "freeze_fingerprint_drift"
	ReasonBatchMismatch    = "fence_batch_mismatch"
	ReasonFailClosed       = "fail_closed_no_fence"
)

const (
	AuditIssueFence = "issue_fence"
	AuditFreezeFence = "freeze_fence"
	AuditReleaseFence = "release_fence"
	AuditExpireFence = "expire_fence"
	AuditDupProof    = "dup_freeze_proof"
	AuditHalfOpen    = "half_submit_open"
	AuditHalfResume  = "half_submit_resume"
	AuditHalfAbandon  = "half_submit_abandon"
	AuditRetryReject  = "retry_reject"
	AuditSubmitAssert = "submit_assert"
	AuditMutexAcquire = "mutex_acquire"
	AuditMutexRelease = "mutex_release"
	AuditPatrolExpire = "patrol_expire"
	AuditExportProof = "export_proof"
)

const (
	TableResidueFences = "wdbatch_residue_fences"
	TableDupFreezeProofs = "wdbatch_dup_freeze_proofs"
	TableHalfSubmitCases = "wdbatch_half_submit_cases"
	TableFreezeSlots = "wdbatch_freeze_slots"
	TableAuditEvents = "wdbatch_residue_fence_audits"
	TableMutexHolds = "wdbatch_submit_retry_mutex"
)
// temporary guard removed after the production check
