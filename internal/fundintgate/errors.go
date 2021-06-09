package fundintgate

import "fmt"

var (
	// ErrUnavailable 引擎或仓储不可用。
	ErrUnavailable = fmt.Errorf("fundintgate: engine unavailable")
	// ErrNoLease Send/SendResolved 未携带 int_gate_lease_id。
	ErrNoLease = fmt.Errorf("fundintgate: send requires unexpired int_gate_lease")
	// ErrLeaseNotFound 租约不存在。
	ErrLeaseNotFound = fmt.Errorf("fundintgate: int_gate_lease not found")
	// ErrLeaseExpired 租约已过 TTL。
	ErrLeaseExpired = fmt.Errorf("fundintgate: int_gate_lease expired")
	// ErrLeaseRevoked 租约已被 Halt / 换闸 / 禁提抬升吊销。
	ErrLeaseRevoked = fmt.Errorf("fundintgate: int_gate_lease revoked")
	// ErrLeaseConsumed 租约已消费（sent），禁止二次 SpotSend。
	ErrLeaseConsumed = fmt.Errorf("fundintgate: int_gate_lease already consumed")
	// ErrLeaseNotGated 租约尚未进入 gated，禁止绑定 Send。
	ErrLeaseNotGated = fmt.Errorf("fundintgate: int_gate_lease not in gated state")
	// ErrLeaseNotBound 租约尚未 bound，禁止落 sent。
	ErrLeaseNotBound = fmt.Errorf("fundintgate: int_gate_lease not bound")
	// ErrGateDenied GateFunc 拒绝放行，不得签发租约。
	ErrGateDenied = fmt.Errorf("fundintgate: gate denied")
	// ErrGateHalted 闸处于 Halt，旧租约立即失效。
	ErrGateHalted = fmt.Errorf("fundintgate: gate halted")
	// ErrGateImplSwap 闸实现指纹已变（换闸）。
	ErrGateImplSwap = fmt.Errorf("fundintgate: gate implementation swapped")
	// ErrForbidWithdrawRaised 禁提标志已抬升。
	ErrForbidWithdrawRaised = fmt.Errorf("fundintgate: forbid-withdraw flag raised")
	// ErrFingerprintDrift 闸指纹与租约钉扎不一致。
	ErrFingerprintDrift = fmt.Errorf("fundintgate: gate fingerprint drift")
	// ErrGenerationDrift 闸世代已抬升。
	ErrGenerationDrift = fmt.Errorf("fundintgate: gate generation drift")
	// ErrResolveDrift 对手解析结果相对租约钉扎发生漂移。
	ErrResolveDrift = fmt.Errorf("fundintgate: counterpart resolve drift")
	// ErrAmountDrift 金额或资产相对租约钉扎发生漂移。
	ErrAmountDrift = fmt.Errorf("fundintgate: amount or asset drift")
	// ErrInvalidTransition 状态机非法迁移。
	ErrInvalidTransition = fmt.Errorf("fundintgate: invalid FSM transition")
	// ErrInvalidReq 请求字段非法。
	ErrInvalidReq = fmt.Errorf("fundintgate: invalid request")
	// ErrIdem 缺少幂等键。
	ErrIdem = fmt.Errorf("fundintgate: idempotency key required")
	// ErrMinAmount 低于最小金额（保持 fundint.ErrMinAmount 语义，本包镜像）。
	ErrMinAmount = fmt.Errorf("fundintgate: internal transfer below min amount")
	// ErrResolveFail 对手解析失败（保持既有拒绝语义）。
	ErrResolveFail = fmt.Errorf("fundintgate: counterpart resolve failed")
	// ErrSelfTransfer 自转拒绝。
	ErrSelfTransfer = fmt.Errorf("fundintgate: self transfer forbidden")
	// ErrProofNotFound 漂移证明不存在。
	ErrProofNotFound = fmt.Errorf("fundintgate: drift proof not found")
	// ErrAuditNotFound 审计行不存在。
	ErrAuditNotFound = fmt.Errorf("fundintgate: audit row not found")
	// ErrGenNotFound 闸世代记录不存在。
	ErrGenNotFound = fmt.Errorf("fundintgate: gate generation not found")
	// ErrSpotSendBlocked fail-closed：复验失败不得调用 SpotSend。
	ErrSpotSendBlocked = fmt.Errorf("fundintgate: spot send blocked by fail-closed lease bind")
	// ErrStore 仓储错误包装根。
	ErrStore = fmt.Errorf("fundintgate: store error")
)

// WrapStore 包装仓储错误并保留原始链。
   func WrapStore(op string, err error) error {
	   return fmt.Errorf("%w: %s: %w", ErrStore, op, err)
   }
