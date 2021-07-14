package hmacwritelease

import "fmt"

var (
	// ErrNonceReplay 持久 nonce 防重放：该 key+nonce 组合已被使用，不得再发租约。
	ErrNonceReplay = fmt.Errorf("hmacwritelease: nonce replay detected in persistent store")
	// ErrLeaseNotFound 租约不存在；WriteCheck fail-closed 拒绝。
	ErrLeaseNotFound = fmt.Errorf("hmacwritelease: lease not found")
	// ErrLeaseExpired 租约已过 TTL；WriteCheck fail-closed 拒绝。
	ErrLeaseExpired = fmt.Errorf("hmacwritelease: lease expired")
	// ErrLeaseRevoked 租约已吊销（Halt 翻转或密钥吊销）；WriteCheck 拒绝。
	ErrLeaseRevoked = fmt.Errorf("hmacwritelease: lease revoked")
	// ErrHaltGenMismatch 租约签发后 Halt 世代已翻转；WriteCheck 拒绝。
	ErrHaltGenMismatch = fmt.Errorf("hmacwritelease: halt generation mismatch")
	// ErrLeaseConsumed 租约已被消费，不可二次使用。
	ErrLeaseConsumed = fmt.Errorf("hmacwritelease: lease already consumed")
	// ErrIllegalTransition 非法状态流转；调用方不得吞掉。
	ErrIllegalTransition = fmt.Errorf("hmacwritelease: illegal state transition")
	// ErrInvalidInput 入参校验失败（空字段、形态错误等）。
	ErrInvalidInput = fmt.Errorf("hmacwritelease: invalid input")
	// ErrHalted 服务处于 Halt 状态，拒绝 VerifyAndLease。
	ErrHalted = fmt.Errorf("hmacwritelease: service halted")
	// ErrNoLeaseForWrite 资金写提交未携带租约；fail-closed 拒绝。
	ErrNoLeaseForWrite = fmt.Errorf("hmacwritelease: no lease provided for fund write")
)
