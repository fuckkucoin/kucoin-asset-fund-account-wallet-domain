package lendoccupygate

import "errors"

var (
	// ErrUnavailable 引擎或 Store 未正确初始化。
	ErrUnavailable = errors.New("lendoccupygate: engine unavailable")
	// ErrOrderRequired order_id 不能为空。
	ErrOrderRequired = errors.New("lendoccupygate: order_id required")
	// ErrUserRequired user_id 不能为空。
	ErrUserRequired = errors.New("lendoccupygate: user_id required")
	// ErrAmountInvalid 金额非正或形态错误。
	ErrAmountInvalid = errors.New("lendoccupygate: amount invalid")
	// ErrAssetRequired 资产代码不能为空。
	ErrAssetRequired = errors.New("lendoccupygate: asset required")
	// ErrGenAlreadyExists 同 order 已有 active gen（未消费），禁止重复签发。
	ErrGenAlreadyExists = errors.New("lendoccupygate: lend_gen already exists for order")
	// ErrGenNotFound 未找到 order 对应的 LendGen；未知 gen 拒绝释放。
	ErrGenNotFound = errors.New("lendoccupygate: lend_gen not found")
	// ErrGenConsumed LendGen 已被消费；本次尝试触发 LendDupReleaseProof。
	ErrGenConsumed = errors.New("lendoccupygate: lend_gen already consumed")
	// ErrLeaseNotFound 未找到 order 对应的 LendOccupyLease。
	ErrLeaseNotFound = errors.New("lendoccupygate: lease not found")
	// ErrLeaseNotPlaced 租约已终止，不可再驱动状态迁移。
	ErrLeaseNotPlaced = errors.New("lendoccupygate: lease not in placed state")
	// ErrInvalidTransition 状态迁移不合法（CAS 失败）。
	ErrInvalidTransition = errors.New("lendoccupygate: invalid state transition")
	// ErrHoldFailed hold 写入失败，触发半截补偿路径。
	ErrHoldFailed = errors.New("lendoccupygate: holdcat place failed; residue compensation required")
	// ErrDupProofNotFound 未找到指定 ProofID 的重复释放证明。
	ErrDupProofNotFound = errors.New("lendoccupygate: dup_release_proof not found")
	// ErrResidueProofNotFound 未找到指定 ProofID 的半截补偿证明。
	ErrResidueProofNotFound = errors.New("lendoccupygate: residue_proof not found")
	// ErrAlreadyProven 同 order/gen 已存在证明，禁止重复落地。
	ErrAlreadyProven = errors.New("lendoccupygate: proof already exists")
	// ErrOwnerMismatch user_id 与 gen 或租约的所有者不匹配。
	ErrOwnerMismatch = errors.New("lendoccupygate: user_id mismatch")
	// ErrAmountMismatch 请求金额与 gen 记录金额不匹配。
	ErrAmountMismatch = errors.New("lendoccupygate: amount mismatch between request and gen")
)
