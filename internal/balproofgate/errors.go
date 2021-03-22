package balproofgate

import "fmt"

var (
	// ErrUnavailable 服务或存储未初始化。
	ErrUnavailable = fmt.Errorf("balproofgate: service unavailable")
	// ErrUserInvalid 用户非法。
	ErrUserInvalid = fmt.Errorf("balproofgate: user id invalid")
	// ErrBookInvalid 科目非法。
	ErrBookInvalid = fmt.Errorf("balproofgate: book invalid")
	// ErrAssetInvalid 资产非法。
	ErrAssetInvalid = fmt.Errorf("balproofgate: asset invalid")
	// ErrNonceInvalid nonce 空或超长。
	ErrNonceInvalid = fmt.Errorf("balproofgate: nonce invalid")
	// ErrAmountInvalid 金额无法按账本精度解析。
	ErrAmountInvalid = fmt.Errorf("balproofgate: amount invalid")
	// ErrSecret 签发密钥缺失。
	ErrSecret = fmt.Errorf("balproofgate: hmac secret missing")
	// ErrStaleGen CAS 世代冲突，调用方应读最新世代后重试。
	ErrStaleGen = fmt.Errorf("balproofgate: stale generation: concurrent issue won CAS")
	// ErrConcurrentIssue 同科目并发签发互斥，未抢到世代。
	ErrConcurrentIssue = fmt.Errorf("balproofgate: concurrent issue mutex: generation bump failed")
	// ErrLeaseNotFound 租约不存在。
	ErrLeaseNotFound = fmt.Errorf("balproofgate: proof lease not found")
	// ErrLeaseNotBound 租约尚未 bound，不得消费。
	ErrLeaseNotBound = fmt.Errorf("balproofgate: proof lease not bound")
	// ErrLeaseTerminal 租约已终态。
	ErrLeaseTerminal = fmt.Errorf("balproofgate: proof lease already terminal")
	// ErrLeaseExpired TTL 到期，旧 Signature 不得放行。
	ErrLeaseExpired = fmt.Errorf("balproofgate: proof lease expired: signature must not pass")
	// ErrRevoked 已吊销，旧 Signature 不得放行。
	ErrRevoked = fmt.Errorf("balproofgate: proof lease revoked: signature must not pass")
	// ErrSig 签名与规范串不符。
	ErrSig = fmt.Errorf("balproofgate: signature mismatch")
	// ErrLiveDrift 活账 Available/Frozen/观察世代相对签发快照已变。
	ErrLiveDrift = fmt.Errorf("balproofgate: live balance drifted after issue")
	// ErrDriftBlocked 存在未关闭漂移证明，同科目闸消费挡写。
	ErrDriftBlocked = fmt.Errorf("balproofgate: open drift proof blocks consume")
	// ErrDriftNotFound 漂移证明不存在。
	ErrDriftNotFound = fmt.Errorf("balproofgate: drift proof not found")
	// ErrDriftAlreadyClosed 漂移案已关闭。
	ErrDriftAlreadyClosed = fmt.Errorf("balproofgate: drift proof already closed")
	// ErrProofDigestMismatch 漂移证明摘要重算不一致。
	ErrProofDigestMismatch = fmt.Errorf("balproofgate: drift proof digest mismatch")
	// ErrOperatorInvalid 操作员标识非法。
	ErrOperatorInvalid = fmt.Errorf("balproofgate: operator invalid")
	// ErrRevokeReason 吊销原因缺失。
	ErrRevokeReason = fmt.Errorf("balproofgate: revoke reason required")
	// ErrObserveUnavailable 活账只读观察失败，签发与复核均失败关闭。
	ErrObserveUnavailable = fmt.Errorf("balproofgate: live book observe unavailable")
	// ErrIsolationViolation 试图把 por/stmt 根写入本包证明字段。
	ErrIsolationViolation = fmt.Errorf("balproofgate: isolation violation: por/stmt roots forbidden")
)
