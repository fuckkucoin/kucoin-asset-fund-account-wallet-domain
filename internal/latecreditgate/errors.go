package latecreditgate

import "errors"

var (
	// ErrFenceRequired 围栏校验失败：epoch=0 或 token 为空时拒绝任何写路径推进状态。
	// 写路径（Reserve/Reclaim/Resolve/AutoCredit）均须携带有效 epoch>0 且 token 非空的围栏凭证。
	ErrFenceRequired = errors.New("latecreditgate: fence required; epoch must be >0 and token must be non-empty")
	// ErrFenceEpochZero epoch=0 是明确非法围栏凭证（legacy read-only 才允许省略）。
	ErrFenceEpochZero = errors.New("latecreditgate: fence epoch=0 is not permitted on write paths")
	// ErrFenceTokenEmpty token 为空是明确非法围栏凭证。
	ErrFenceTokenEmpty = errors.New("latecreditgate: fence token empty is not permitted on write paths")
	// ErrOccupyConflict 同 output_key 已处于 occupied 状态，不允许重复占用。
	ErrOccupyConflict = errors.New("latecreditgate: output_key already occupied; concurrent reclaim occupy conflict")
	// ErrCreditBlocked 同 output_key 当前处于 occupied 状态，credit 路径 fail-closed。
	// 此错误触发时会同步开 LateCreditRaceProof。
	ErrCreditBlocked = errors.New("latecreditgate: credit blocked; output_key occupied by reclaim; race proof opened")
	// ErrGenNotFound 指定 output_key 尚未建立 late_credit_gen 记录。
	ErrGenNotFound = errors.New("latecreditgate: late_credit_gen not found for output_key")
	// ErrGenRollback 禁止世代回拨：新世代必须 ≥ 当前世代。
	ErrGenRollback = errors.New("latecreditgate: generation rollback is forbidden; new_gen must be >= current_gen")
	// ErrGenMismatch 世代令牌不匹配（CAS 保护）。
	ErrGenMismatch = errors.New("latecreditgate: generation mismatch; state has advanced since last read")
	// ErrRaceEventNotFound 竞态事件 ID 不存在。
	ErrRaceEventNotFound = errors.New("latecreditgate: race event not found")
	// ErrRaceEventAlreadyAcked 竞态事件已处于 acked 状态，不可重复 ack。
	ErrRaceEventAlreadyAcked = errors.New("latecreditgate: race event already acked")
	// ErrProofNotFound 竞态证明不存在。
	ErrProofNotFound = errors.New("latecreditgate: race proof not found")
	// ErrOutputKeyEmpty output_key 三元组不完整（chain/tx_hash 不可为空）。
	ErrOutputKeyEmpty = errors.New("latecreditgate: output_key (chain, tx_hash, output_index) must be non-empty")
	// ErrActorEmpty 操作方 actor_id 不可为空。
	ErrActorEmpty = errors.New("latecreditgate: actor_id must be non-empty")
	// ErrInvalidRequest 请求参数非法（通用校验失败）。
	ErrInvalidRequest = errors.New("latecreditgate: invalid request parameter")
	// ErrStateInvalid 状态机转换非法。
	ErrStateInvalid = errors.New("latecreditgate: invalid state transition")
	// ErrAlreadyCredited 该 output_key 已成功贷记，不得重复贷记。
	ErrAlreadyCredited = errors.New("latecreditgate: output_key already credited; double-credit prevented")
	// ErrAlreadyReclaimed 该 output_key 已完成回收，不得再贷记。
	ErrAlreadyReclaimed = errors.New("latecreditgate: output_key already reclaimed")
	// ErrRaceStillOpen 该 output_key 存在未解决竞态，须先通过 ResolveRace 关闭。
	ErrRaceStillOpen = errors.New("latecreditgate: race still open for output_key; resolve first")
)
