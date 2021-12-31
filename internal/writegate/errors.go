package writegate

import "fmt"

var (
	// ErrTokenInvalid 令牌校验失败（撤销 / 过期 / 代次不符）。
	ErrTokenInvalid = fmt.Errorf("writegate: token invalid")
	// ErrScopeMismatch 写作用域与令牌授权不匹配。
	ErrScopeMismatch = fmt.Errorf("writegate: scope mismatch")
	// ErrUnknownScope 写作用域名称不在允许集合内。
	ErrUnknownScope = fmt.Errorf("writegate: unknown write scope")
	// ErrProbeLocked 当前交易对处于熔断器半开探针状态，禁止资金写入。
	ErrProbeLocked = fmt.Errorf("writegate: probe mutex locked — half-open probe tick must not enter fund writes")
	// ErrGateUnavailable WriteGate 或 PoisonEngine 为 nil。
	ErrGateUnavailable = fmt.Errorf("writegate: gate unavailable")
)
