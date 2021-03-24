package claimmutex

import "errors"

var (
	// ErrSecondClaim 同 (chain,tx_hash,output_index) 已有胜出认领，拒绝第二次认领。
	ErrSecondClaim = errors.New("claimmutex: second claim on same output refused")
	// ErrMutexConflict 竞态互斥：同输出另一腿已经持有效果租约，当前操作被拒绝。
	ErrMutexConflict = errors.New("claimmutex: concurrent claim/auto-credit mutex conflict")
	// ErrProofMissing 效果证明缺失，不得将认领标记为完成。
	ErrProofMissing = errors.New("claimmutex: effect proof missing; claim cannot be marked complete")
	// ErrLeaseExpired 效果租约已超期，不接受后续操作。
	ErrLeaseExpired = errors.New("claimmutex: output effect lease expired")
	// ErrLeaseNotFound 效果租约不存在。
	ErrLeaseNotFound = errors.New("claimmutex: output effect lease not found")
	// ErrDupCaseExists 同输出已有未关闭的双贷案件。
	ErrDupCaseExists = errors.New("claimmutex: dup-credit case already open for output")
	// ErrDupCaseNotFound 双贷案件不存在。
	ErrDupCaseNotFound = errors.New("claimmutex: dup-credit case not found")
	// ErrBindNotFound AutoCreditBind 绑定不存在。
	ErrBindNotFound = errors.New("claimmutex: auto-credit bind not found")
	// ErrProofNotFound 效果证明不存在。
	ErrProofNotFound = errors.New("claimmutex: effect proof not found")
	// ErrInvalidRequest 请求参数非法。
	ErrInvalidRequest = errors.New("claimmutex: invalid request parameter")
	// ErrOutputKeyEmpty 输出键（chain+hash+index）不能为空。
	ErrOutputKeyEmpty = errors.New("claimmutex: output key (chain, tx_hash, output_index) must be non-empty")
	// ErrActorEmpty 操作方 actor_id 不能为空。
	ErrActorEmpty = errors.New("claimmutex: actor_id must be non-empty")
	// ErrGenerationMismatch 效果世代不匹配，操作被拒绝。
	ErrGenerationMismatch = errors.New("claimmutex: effect generation mismatch; output has advanced")
	// ErrCaseClosed 双贷案件已关闭，不接受更新。
	ErrCaseClosed = errors.New("claimmutex: dup-credit case already closed")
	// ErrProofClosed 效果证明已关闭。
	ErrProofClosed = errors.New("claimmutex: effect proof already closed")
)
