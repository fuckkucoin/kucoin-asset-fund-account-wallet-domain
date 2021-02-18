package availcas

import "errors"

var (
	// ErrLeaseNotFound 租约不存在或已被清理。
	ErrLeaseNotFound = errors.New("availcas: lease not found")
	// ErrLeaseExpired 租约已过期，必须申请新租约后重试。
	ErrLeaseExpired = errors.New("availcas: lease expired")
	// ErrLeaseRevoked 租约已被撤销（资产余额发生非意图变更）。
	ErrLeaseRevoked = errors.New("availcas: lease revoked")
	// ErrLeaseBound 租约已绑定到已接受意图，不可二次绑定。
	ErrLeaseBound = errors.New("availcas: lease already bound")
	// ErrIntentNotFound 意图记录不存在。
	ErrIntentNotFound = errors.New("availcas: intent not found")
	// ErrIntentFinalized 意图已终态（accepted/rejected），不可重复提交。
	ErrInsufficientAvailable = errors.New("availcas: insufficient available balance")
	// ErrAmountNonPositive 扣减额必须为正数。
	ErrAmountNonPositive = errors.New("availcas: debit amount must be positive")
	// ErrUserRequired 用户 ID 不得为空。
	ErrUserRequired = errors.New("availcas: user_id required")
	// ErrAssetRequired 资产代码不得为空。
	ErrAssetRequired = errors.New("availcas: asset required")
	// ErrBookRequired 账本位不得为空。
	ErrBookRequired = errors.New("availcas: book required")
	// ErrStoreUnavailable 持久化层未就绪。
	ErrStoreUnavailable = errors.New("availcas: store unavailable")
	// ErrProofNotFound 竞态证明不存在。
	ErrProofNotFound = errors.New("availcas: race proof not found")
	// ErrDuplicateIntent 同幂等键的意图已存在，返回已有记录。
	ErrDuplicateIntent = errors.New("availcas: duplicate intent key")
)

// RejectCode 意图拒绝原因码，落入 RaceProof.RejectCode。
type RejectCode string

const (
	RejectCodeCASConflict RejectCode = "cas_conflict"
	RejectCodeLeaseExpired RejectCode = "lease_expired"
	RejectCodeLeaseRevoked RejectCode = "lease_revoked"
	RejectCodeInsufficientAvail RejectCode = "insufficient_avail"
	RejectCodeDuplicateIdem     RejectCode = "duplicate_idem"
)
