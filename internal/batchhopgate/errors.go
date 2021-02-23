package batchhopgate

import "fmt"

var (
	// ErrUnavailable 服务或存储层未初始化。
	ErrUnavailable = fmt.Errorf("batchhopgate: service unavailable")
	// ErrUserInvalid 用户 ID 非法（空值或包含非法字符）。
	ErrUserInvalid = fmt.Errorf("batchhopgate: user id invalid")
	// ErrAssetInvalid 资产标识非法。
	ErrAssetInvalid = fmt.Errorf("batchhopgate: asset invalid")
	// ErrStaleGeneration CAS 世代竞争失败；当前世代与期望不符。调用方须重读世代后重试。
	ErrStaleGeneration = fmt.Errorf("batchhopgate: stale generation: concurrent batch hop detected")
	// ErrConcurrentBatch 同用户同资产存在未闭合残留案，禁止并行 BatchHop。
	ErrConcurrentBatch = fmt.Errorf("batchhopgate: concurrent batch hop blocked: open residual exists")
	// ErrResidualNotFound 残留案 ID 不存在。
	ErrResidualNotFound = fmt.Errorf("batchhopgate: residual case not found")
	// ErrLeaseNotFound 续跳租约 ID 不存在。
	ErrLeaseNotFound = fmt.Errorf("batchhopgate: resume lease not found")
	// ErrLeaseExpired 续跳令牌已超过 TTL，须重新申请。
	ErrLeaseExpired = fmt.Errorf("batchhopgate: resume lease expired")
	// ErrLeaseConflict 同一残留案已存在活跃续跳租约，不允许重复申请。
	ErrLeaseConflict = fmt.Errorf("batchhopgate: resume lease conflict: active lease already exists")
	// ErrResumeIndexInvalid 续跳索引不合法（小于 FailIndex 或超过 TotalItems）。
	ErrResumeIndexInvalid = fmt.Errorf("batchhopgate: resume index invalid: must start at fail index")
	// ErrResumeConflict 续跳幂等键冲突（同 idem 键对应不同参数）。
	ErrResumeConflict = fmt.Errorf("batchhopgate: resume idem conflict")
	// ErrResumeDoubleExec 续跳尝试对已成功 hop 索引二次执行，被拒绝。
	ErrResumeDoubleExec = fmt.Errorf("batchhopgate: resume double execution rejected: hop already succeeded")
	// ErrProofDigestMismatch 占用证明 Digest 校验不通过，须重建证明。
	ErrProofDigestMismatch = fmt.Errorf("batchhopgate: proof digest mismatch: proof integrity check failed")
	// ErrProofNotFound 指定的 HopOccupyProof 不存在。
	ErrProofNotFound = fmt.Errorf("batchhopgate: occupy proof not found")
	// ErrResidualStatusInvalid 残留案状态不允许当前操作（如在 closed 状态发起续跳）。
	ErrResidualStatusInvalid = fmt.Errorf("batchhopgate: residual case status invalid for this operation")
	// ErrNoTokenProvided 续跳请求未携带世代令牌。
	ErrNoTokenProvided = fmt.Errorf("batchhopgate: no generation token provided for resume")
	// ErrTokenGenerationMismatch 续跳世代令牌与当前残留案世代不符。
	ErrTokenGenerationMismatch = fmt.Errorf("batchhopgate: generation token mismatch")
	// ErrConfirmMissing 四眼确认未满足（需要第二确认人）。
	ErrConfirmMissing = fmt.Errorf("batchhopgate: four-eye confirmation required")
	// ErrSameConfirmer 两次确认为同一人，四眼确认不满足。
	ErrSameConfirmer = fmt.Errorf("batchhopgate: same confirmer for both eyes: four-eye confirmation failed")
)

// ErrBatchFail 包装 BatchHop 逐项失败错误，携带失败索引。
  func ErrBatchFail(index int, cause error) error {
	  return fmt.Errorf("batchhopgate: batch hop failed at item %d: %w", index, cause)
  }
