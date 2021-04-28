package creditoutgate

import "fmt"

var (
	// ErrUnavailable 服务未初始化或注入依赖缺失。
	ErrUnavailable = fmt.Errorf("creditoutgate unavailable")
	// ErrBoardKey 板卡标识无效或为空。
	ErrBoardKey = fmt.Errorf("creditoutgate board key invalid")
	// ErrCreditKey 贷记键无效或为空。
	ErrCreditKey = fmt.Errorf("creditoutgate credit key invalid")
	// ErrBoardClosed 板卡/护栏已关闭，拒绝新 Enqueue。
	ErrBoardClosed = fmt.Errorf("creditoutgate board closed")
	// ErrGenDrift 世代漂移：Deliver 时当前板卡世代 > 绑定世代。
	ErrGenDrift = fmt.Errorf("creditoutgate generation drift")
	// ErrLeaseRevoked 投递租约已被吊销，拒绝 Deliver。
	ErrLeaseRevoked = fmt.Errorf("creditoutgate lease revoked")
	// ErrLeaseNotFound 未找到对应的投递租约。
	ErrLeaseNotFound = fmt.Errorf("creditoutgate lease not found")
	// ErrAmountConflict 同 key 不同金额的 Enqueue 冲突。
	ErrAmountConflict = fmt.Errorf("creditoutgate amount conflict")
	// ErrDualCredit 同一 key 发现两条已完成投递。
	ErrDualCredit = fmt.Errorf("creditoutgate dual credit detected")
	// ErrEpochToken epoch/token 不匹配（旧 token 重放）。
	ErrEpochToken = fmt.Errorf("creditoutgate epoch token mismatch")
	// ErrAmount 金额形态无效。
	ErrAmount = fmt.Errorf("creditoutgate amount invalid")
	// ErrOperator 操作者标识为空（审计必填）。
	ErrOperator = fmt.Errorf("creditoutgate operator required")
	// ErrAlreadyDelivered 已 Deliver，幂等路径返回此哨兵（非错误，由调用方决定）。
	ErrAlreadyDelivered = fmt.Errorf("creditoutgate already delivered")
)
