package fundcircuit

import "errors"

var (
	// ErrPath 入口不在受管清单内。
	ErrPath = errors.New("fundcircuit path invalid")
	// ErrScope 作用域非法。
	ErrScope = errors.New("fundcircuit scope invalid")
	// ErrTarget 作用域目标缺失（user/asset/chain 必须给目标）。
	ErrTarget = errors.New("fundcircuit scope target required")
	// ErrSignalKind 信号种类未登记。
	ErrSignalKind = errors.New("fundcircuit signal kind invalid")
	// ErrSignal 信号字段不足（缺目标或权重为负）。
	ErrSignal = errors.New("fundcircuit signal invalid")
	// ErrSeverity 分档非法。
	ErrSeverity = errors.New("fundcircuit severity invalid")
	// ErrKeyPrefix 键位不是 fc: 前缀。dep:/wd: 键只能作为来源引用，不得当作熔断键写入。
	ErrKeyPrefix = errors.New("fundcircuit key must use fc: prefix")
	// ErrForeignKey 试图用 fc: 以外的域键写熔断表。
	ErrForeignKey = errors.New("fundcircuit refuses foreign velocity key")
	// ErrBridgeKey 桥接的原域键形不合法。
	ErrBridgeKey = errors.New("fundcircuit bridge key invalid")
	// ErrPolicy 策略字段非法。
	ErrPolicy = errors.New("fundcircuit policy invalid")
	// ErrPolicyVersion 策略代次不存在或非法。
	ErrPolicyVersion = errors.New("fundcircuit policy version invalid")
	// ErrUnfreezeForbidden 熔断矩阵不含解冻动作；筛查中或广播中的单不得自动解冻。
	ErrUnfreezeForbidden = errors.New("fundcircuit refuses auto-unfreeze of in-flight order")
	// ErrCircuitOpen 熔断器处于拦截态，新写被挡。
	ErrCircuitOpen = errors.New("fundcircuit open: fund abuse circuit blocked this write")
	// ErrCircuitMissing 熔断键不存在。
	ErrCircuitMissing = errors.New("fundcircuit circuit not found")
	// ErrCircuitState 状态跃迁不在边表内。
	ErrCircuitState = errors.New("fundcircuit state transition denied")
	// ErrProbeBusy 半开期已有在途探针。
	ErrProbeBusy = errors.New("fundcircuit half-open probe already in flight")
	// ErrProbeBudget 半开探针配额用尽。
	ErrProbeBudget = errors.New("fundcircuit half-open probe budget exhausted")
	// ErrProbeState 探针不在半开态或已结论。
	ErrProbeState = errors.New("fundcircuit probe state invalid")
	// ErrProbeMissing 探针不存在。
	ErrProbeMissing = errors.New("fundcircuit probe not found")
	// ErrProbeToken 探针令牌不匹配。
	ErrProbeToken = errors.New("fundcircuit probe token mismatch")
	// ErrHalfOpenOnly 只有半开态才能签发探针。
	ErrHalfOpenOnly = errors.New("fundcircuit probe requires half_open state")
	// ErrCaseMissing 案件不存在。
	ErrCaseMissing = errors.New("fundcircuit case not found")
	// ErrCaseState 案件状态跃迁非法。
	ErrCaseState = errors.New("fundcircuit case state transition denied")
	// ErrDualControl 单人 Clear 被拒；解熔断须双人授权。
	ErrDualControl = errors.New("fundcircuit clear requires dual control")
	// ErrActor 缺操作人或用了 system 账号。
	ErrActor = errors.New("fundcircuit actor required")
	// ErrReason 缺操作原因。
	ErrReason = errors.New("fundcircuit reason required")
	// ErrStore 持久化不可用。读写失败按失败即关处理。
	ErrStore = errors.New("fundcircuit store unavailable")
	// ErrBus 总线未装配。
	ErrBus = errors.New("fundcircuit bus unavailable")
)
