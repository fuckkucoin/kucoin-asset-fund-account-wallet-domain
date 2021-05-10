package feereserve

import "fmt"

var (
	// ErrUnavailable 服务不可用（nil 接收者或未绑定存储）。
	ErrUnavailable = fmt.Errorf("fee reserve service unavailable")
	// ErrNotFound 指定的储备记录不存在。
	ErrNotFound = fmt.Errorf("fee reserve not found")
	// ErrAlreadyExists 该出金单已存在储备记录，不可重复创建。
	ErrAlreadyExists = fmt.Errorf("fee reserve already exists for this withdraw")
	// ErrState 当前状态不允许该操作（如对已 settled 的储备再次 settle）。
	ErrState = fmt.Errorf("fee reserve illegal state transition")
	// ErrConserve 储备守恒校验失败（actual+delta != reserved）。
	ErrConserve = fmt.Errorf("fee reserve conservation violated")
	// ErrAmount 金额形态或值域不合法。
	ErrAmount = fmt.Errorf("fee reserve amount invalid")
	// ErrWithdrawID 出金单 ID 缺失或形态不合法。
	ErrWithdrawID = fmt.Errorf("fee reserve withdraw_id invalid")
	// ErrVersionExpired 关联的报价代次已过期，储备请求被拒。
	ErrVersionExpired = fmt.Errorf("fee reserve rejected: quote version expired")
	// ErrLedger 账本入账失败（下游错误）。
	ErrLedger = fmt.Errorf("fee reserve ledger error")
	ErrAsset = fmt.Errorf("fee reserve asset invalid")
	ErrLeaseExpired = fmt.Errorf("fee reserve lease expired")
	ErrLeaseGeneration = fmt.Errorf("fee reserve lease generation mismatch")
	ErrLeaseNotFound   = fmt.Errorf("fee reserve lease not found")
	ErrLeaseBlocked     = fmt.Errorf("fee reserve lease blocked")
	ErrTerminalConflict = fmt.Errorf("fee reserve terminal conflict: settle XOR release")
	ErrTerminalSealed = fmt.Errorf("fee reserve terminal already sealed")
	ErrDeltaProof     = fmt.Errorf("fee reserve delta proof invalid")
	ErrDeltaSeal = fmt.Errorf("fee reserve delta proof already sealed")
	ErrCaseNotFound = fmt.Errorf("fee reserve case not found")
	ErrCaseClosed   = fmt.Errorf("fee reserve case already closed")
	ErrCaseProven   = fmt.Errorf("fee reserve case already proven")
	ErrFence        = fmt.Errorf("fee reserve fence stale")
	ErrDualVoucher = fmt.Errorf("fee reserve dual voucher detected")
	ErrOperateAfterTerm = fmt.Errorf("fee reserve operate after terminal")
	ErrIdem = fmt.Errorf("fee reserve idempotency conflict")
	ErrBlocked       = fmt.Errorf("fee reserve writes blocked")
	ErrUnderResidual = fmt.Errorf("fee reserve undercharge residual")
)
