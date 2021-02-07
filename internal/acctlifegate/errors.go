package acctlifegate

import "errors"

var (
	// ErrLeaseNotFound 未找到指定租约。
	ErrLeaseNotFound = errors.New("acctlifegate: life lease not found")
	// ErrLeaseExpired 租约已超期，不可用于 Move。
	ErrLeaseExpired = errors.New("acctlifegate: life lease expired")
	// ErrLeaseRevoked 租约已被 Freeze/Close 吊销，必须重新获取。
	ErrLeaseRevoked = errors.New("acctlifegate: life lease revoked")
	// ErrLeaseUsed 租约已被消费，不可重复使用。
	ErrLeaseUsed = errors.New("acctlifegate: life lease already used")
	// ErrGenerationMismatch 提交的 generation 与当前 per-(user,book) 世代不匹配。
	ErrGenerationMismatch = errors.New("acctlifegate: life generation mismatch")
	// ErrMoveBindInFlight 存在 pending 的在途 Move 绑定，Freeze/Close 被阻止。
	ErrMoveBindInFlight = errors.New("acctlifegate: move bind in-flight, freeze/close blocked")
	// ErrMoveBindNotFound 未找到指定 MoveBind。
	ErrMoveBindNotFound = errors.New("acctlifegate: move bind not found")
	// ErrDriftBlockWrite 科目存在未关闭的漂移案件，借记操作被屏蔽。
	ErrDriftBlockWrite = errors.New("acctlifegate: debit blocked by open life drift case")
	// ErrDriftCaseNotFound 未找到指定漂移案件。
	ErrDriftCaseNotFound = errors.New("acctlifegate: life drift case not found")
	// ErrDriftProofIncomplete 漂移案件证明字段不完整，不可关闭。
	ErrDriftProofIncomplete = errors.New("acctlifegate: drift proof incomplete, cannot close")
	// ErrDriftNotProven 案件尚未经过双人确认，不可关闭。
	ErrDriftNotProven = errors.New("acctlifegate: drift case not yet proven")
	// ErrBookCannotDebit 科目当前状态不允许借记（frozen/closed）。
	ErrBookCannotDebit = errors.New("acctlifegate: book status does not allow debit")
	// ErrInvalidUserID 用户 ID 形态非法。
	ErrInvalidUserID = errors.New("acctlifegate: invalid user_id")
	// ErrInvalidBook 科目名称非法。
	ErrInvalidBook = errors.New("acctlifegate: invalid book")
	// ErrInvalidLease 租约参数非法。
	ErrInvalidLease = errors.New("acctlifegate: invalid lease parameter")
	// ErrStoreTxFailed 持久化事务失败（通常由 CAS 竞态导致）。
	ErrStoreTxFailed = errors.New("acctlifegate: store transaction failed")
	ErrUnavailable = errors.New("acctlifegate: gate unavailable")
	ErrNoAccount   = errors.New("acctlifegate: account service required")
	ErrNoEngine    = errors.New("acctlifegate: close epoch engine required")
)
