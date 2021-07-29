package isodebt

import "errors"

var (
	// ErrLeaseInvalid 租约状态非 fresh。
	ErrLeaseInvalid = errors.New("isodebt: ratio lease invalid")
	// ErrLeaseExpired 租约超过 ExpiresAt。
	ErrLeaseExpired = errors.New("isodebt: ratio lease expired")
	// ErrGenerationMismatch 租约世代与 Pair 当前世代不符。
	ErrGenerationMismatch = errors.New("isodebt: generation mismatch")
	// ErrRatioOverLimit DebtRatio 超 MaxDebtRatio，转入被阻断。
	ErrRatioOverLimit = errors.New("isodebt: debt ratio over limit, transfer-in blocked")
	// ErrDriftUnproven 漂移案例未完成证明，不得恢复 TransferInEnabled。
	ErrDriftUnproven = errors.New("isodebt: ratio drift case unproven, transfer-in disabled")
	// ErrMutexConflict 借记↔转入并发冲突，fail-closed。
	ErrMutexConflict = errors.New("isodebt: debit-transfer mutex conflict, fail-closed")
	// ErrInvalidInput 无效输入参数。
	ErrInvalidInput = errors.New("isodebt: invalid input")
	// ErrNotFound 目标对象不存在。
	ErrNotFound = errors.New("isodebt: not found")
)
