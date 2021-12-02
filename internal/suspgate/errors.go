package suspgate

import "fmt"

var (
	// ErrAsset 资产符号无效（仅允许 USDT/USDC）。
	ErrAsset = fmt.Errorf("suspgate: asset invalid (allowed: USDT, USDC)")
	// ErrActor 操作员标识为空或不合法。
	ErrActor = fmt.Errorf("suspgate: actor must be non-empty")
	// ErrFourEyes 四眼审核须两个不同操作员。
	ErrFourEyes = fmt.Errorf("suspgate: four-eyes requires two distinct actors")
	// ErrCaseNotFound 失配案件不存在。
	ErrCaseNotFound = fmt.Errorf("suspgate: mismatch case not found")
	// ErrGenNotFound 暂停世代记录不存在。
	ErrGenNotFound = fmt.Errorf("suspgate: suspense generation not found")
	// ErrLeaseNotFound 清障租约不存在。
	ErrLeaseNotFound = fmt.Errorf("suspgate: clearance lease not found")
	// ErrGenConflict 并发 Suspend 与 Clear 竞争，CAS 失败；调用方须重试。
	ErrGenConflict = fmt.Errorf("suspgate: gen cas conflict; please retry")
	// ErrGenAlreadyCleared 目标世代已被清障，不可重复消费。
	ErrGenAlreadyCleared = fmt.Errorf("suspgate: gen already cleared")
	// ErrLeaseExpired 放行租约已超时。
	ErrLeaseExpired = fmt.Errorf("suspgate: clearance lease expired")
	// ErrLeaseConsumed 放行租约已被消费，不可复用。
	ErrLeaseConsumed = fmt.Errorf("suspgate: clearance lease already consumed")
	// ErrLeaseInvalid 放行租约状态无效（非 issued）。
	ErrLeaseInvalid = fmt.Errorf("suspgate: clearance lease invalid state")
	// ErrWithdrawBlocked 出金写路径被暂停世代挡写（fail-closed）。
	ErrWithdrawBlocked = fmt.Errorf("suspgate: withdraw blocked by active suspense generation")
	// ErrLeaseRequired 策略要求携带 clearance_lease，但未提供或已失效。
	ErrLeaseRequired = fmt.Errorf("suspgate: clearance lease required but missing or expired")
	// ErrIllegalTransition 非法 FSM 状态跃迁。
	ErrIllegalTransition = fmt.Errorf("suspgate: illegal state transition")
	// ErrInvalidInput 请求参数不合法。
	ErrInvalidInput = fmt.Errorf("suspgate: invalid input")
	// ErrUnavailable 服务或仓储不可用。
	ErrUnavailable = fmt.Errorf("suspgate: service unavailable")
)
