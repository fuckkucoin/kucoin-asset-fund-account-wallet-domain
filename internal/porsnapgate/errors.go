package porsnapgate

import "fmt"

// sentinel errors — 禁止在比较时 wrap，用 errors.Is。
var (
	// ErrNoGen 导出缺少 snap_gen 绑定。
	ErrNoGen = fmt.Errorf("porsnapgate: formal operation requires snap_gen binding")
	// ErrGenSealed 该世代已封印，不可二次封印。
	ErrGenSealed = fmt.Errorf("porsnapgate: snap_gen already sealed")
	// ErrGenNotFound 请求的 snap_gen 不存在。
	ErrGenNotFound = fmt.Errorf("porsnapgate: snap_gen not found")
	// ErrGenExpired 世代已过期，不允许公证或证明。
	ErrGenExpired = fmt.Errorf("porsnapgate: snap_gen expired")
	// ErrGenRevoked 世代已被撤销。
	ErrGenRevoked = fmt.Errorf("porsnapgate: snap_gen revoked")
	// ErrAttestNotFound 公证记录不存在。
	ErrAttestNotFound = fmt.Errorf("porsnapgate: attestation not found")
	// ErrAttestBound 公证已绑定世代，禁止修改绑定。
	ErrAttestBound = fmt.Errorf("porsnapgate: attestation already bound to gen")
	// ErrAttestRevoked 公证已撤销，不可再使用。
	ErrAttestRevoked = fmt.Errorf("porsnapgate: attestation revoked")
	// ErrAttestPublished 公证已发布，不可二次发布。
	ErrAttestPublished = fmt.Errorf("porsnapgate: attestation already published")
	// ErrDriftProofConflict 相同 gen 已存在漂移证明，禁止覆盖。
	ErrDriftProofConflict = fmt.Errorf("porsnapgate: drift proof already exists for this gen")
	// ErrLiveMode live 模式下禁止写入公证表。
	ErrLiveMode = fmt.Errorf("porsnapgate: live mode read must not write attestation table")
	// ErrRootMismatch Merkle 根不一致。
	ErrRootMismatch = fmt.Errorf("porsnapgate: root mismatch on replay/compare")
	// ErrTotalsFingerprint totals 指纹不一致。
	ErrTotalsFingerprint = fmt.Errorf("porsnapgate: totals fingerprint mismatch")
	// ErrStoreMark store 水位不一致，活账已变动。
	ErrStoreMark = fmt.Errorf("porsnapgate: store watermark advanced — live ledger mutated after seal")
	// ErrInvalidTransition 状态机非法迁移。
	ErrInvalidTransition = fmt.Errorf("porsnapgate: invalid FSM transition")
	// ErrEmpty 空快照或世代字段缺失。
	ErrEmpty = fmt.Errorf("porsnapgate: snapshot gen fields are empty")
	// ErrStore 仓储层错误包装。
	ErrStore = fmt.Errorf("porsnapgate: store error")
)

// WrapStore 包装仓储错误并保留原始链。

  func WrapStore(op string, err error) error {
	  return fmt.Errorf("%w: %s: %w", ErrStore, op, err)
   }
