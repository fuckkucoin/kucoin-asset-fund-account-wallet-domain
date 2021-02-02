package airimportgate

import "fmt"

var (
	// ErrUnavailable 服务未初始化。
	ErrUnavailable = fmt.Errorf("airimportgate: service unavailable")
	// ErrLeaseNotFound 租约不存在。
	ErrLeaseNotFound = fmt.Errorf("airimportgate: import lease not found")
	// ErrSessionNotFound Session 不存在。
	ErrSessionNotFound = fmt.Errorf("airimportgate: session not found")
	// ErrImportGenExpired air_import_gen 已过期。
	ErrImportGenExpired = fmt.Errorf("airimportgate: import gen expired")
	// ErrImportGenInvalid air_import_gen 形态非法。
	// ErrBlobReplay 密封 blob 摘要已被导入过（重放攻击）。
	ErrBlobReplay = fmt.Errorf("airimportgate: sealed blob replay rejected")
	// ErrBlobDigestMismatch blob 摘要不匹配。
	ErrBlobDigestMismatch = fmt.Errorf("airimportgate: blob digest mismatch")
	// ErrQuorumDrift 实际签名人数低于 Threshold（人数漂移）。
	ErrQuorumDrift = fmt.Errorf("airimportgate: quorum drift rejected")
	// ErrInvalidActor 操作人非法。
	ErrInvalidActor = fmt.Errorf("airimportgate: actor invalid")
	// ErrInvalidTransition CAS 状态跃迁非法。
	ErrInvalidTransition = fmt.Errorf("airimportgate: invalid state transition")
	// ErrLeaseExpired 租约已过期。
	ErrLeaseExpired = fmt.Errorf("airimportgate: import lease expired")
	// ErrLeaseTerminal 租约已在终态，不可操作。
	ErrLeaseTerminal = fmt.Errorf("airimportgate: lease already terminal")
	// ErrConflictNotFound 冲突证明不存在。
	ErrConflictNotFound = fmt.Errorf("airimportgate: conflict proof not found")
	// ErrDigestNotFound 摘要记录不存在。
	ErrDigestNotFound = fmt.Errorf("airimportgate: blob digest record not found")
	// ErrBindNotFound 法定人数绑定记录不存在。
	ErrBindNotFound = fmt.Errorf("airimportgate: quorum bind not found")
	// ErrEmptySessionID Session ID 为空。
	ErrEmptySessionID = fmt.Errorf("airimportgate: session_id required")
	// ErrEmptyBlob 密封 blob 为空。
	ErrEmptyBlob = fmt.Errorf("airimportgate: sealed blob empty")
)
