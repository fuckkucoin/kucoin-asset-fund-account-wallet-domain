// 账本领域错误。传输层只做映射，不在这里拼 HTTP 码。
// 作者: Kinsley
// 日期: 2021.01.05
package ledger

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrInsufficient = errors.New("insufficient available balance")
	ErrNegative     = errors.New("balance would go negative")
	ErrIdempotency  = errors.New("idempotency conflict")
	ErrRiskRejected = errors.New("risk rejected")
	ErrBooksDrift          = errors.New("books drift: new movements blocked until reconcile is clean")
	ErrPrecision           = errors.New("amount exceeds 18 decimal places")
	ErrPending             = errors.New("chain tx not yet confirmed")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different payload")
	ErrScientific          = errors.New("scientific notation amounts are rejected")
	ErrStaleQuote          = errors.New("spot ticker too stale to quote")
	ErrSlippage            = errors.New("price moved beyond slippage budget")
	ErrBusy              = errors.New("lock busy")
	ErrIllegalTransition = errors.New("illegal state transition")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrRolledBack        = errors.New("transfer already rolled back")
	ErrUnknownDirection  = errors.New("unknown transfer direction")
	ErrMissingChain = errors.New("chain required for on-chain movement")
	ErrMissingAddress = errors.New("to_address required for withdraw")
	ErrMissingIdempotency = errors.New("idempotency_key required")
	ErrAssetChain         = errors.New("asset is not listed on this chain")
	// 链上交易仍可能有效。单据已有哈希时严禁解冻。
	ErrSubmittedLive = errors.New("cannot unfreeze on-chain transfer while chain tx is live")
	// 同一链、同一哈希、同一输出下标已经挂在另一张单上。严禁再入账或再广播。
	ErrDuplicateHash       = errors.New("chain tx hash already credited or in flight")
	// 围栏令牌与单据不符。租约已丢的执行体不得再改这张单。
	ErrStaleFence          = errors.New("stale fencing token")
	// 科目行上的余额已经不是本次读到的快照。严禁用内存里的旧余额覆盖。
	ErrStaleBook           = errors.New("stale book balance")
	ErrObservationMismatch = errors.New("chain observation amount or asset mismatch")
	ErrKYTDenied = errors.New("kyt denied")
	// 筛查不可用。出金必须停在冻结之前或回滚冻结，严禁接着签名广播。
	ErrKYTUnavailable = errors.New("kyt unavailable")
	// 筛查要求人工复核。冻结保持，不得当成失败解冻，也不得广播。
	ErrKYTReview      = errors.New("kyt review")
	ErrInvalidAddress = errors.New("invalid address book entry")
	ErrAccountFrozen = errors.New("account frozen")
	ErrCannotCancel = errors.New("transfer cannot be cancelled")
	ErrSelfTransfer   = errors.New("cannot send to self")
	ErrCounterparty   = errors.New("counterparty user_id required")
	ErrKYCRequired    = errors.New("kyc level too low")
	ErrInvalidKYC     = errors.New("invalid kyc level")
	ErrAmountOverflow = errors.New("amount exceeds ledger magnitude")
	ErrDepositAddress = errors.New("deposit address does not belong to user")
	ErrAddressFormat = errors.New("address format invalid for chain")
	ErrMakerChecker  = errors.New("kyt decision requires a distinct ops actor")
	ErrKYTNotPending = errors.New("transfer is not awaiting kyt review")
	ErrOwnInbound = errors.New("cannot withdraw to own deposit address")
	ErrAddressPoison = errors.New("withdraw address looks like own inbound")
	ErrPassThrough = errors.New("withdraw too close to a recent deposit")
	ErrWithdrawCooling = errors.New("withdraw locked after account unfreeze")
	ErrWithdrawForbidden = errors.New("user forbidden to withdraw")
	ErrHotInventory      = errors.New("hot wallet inventory too low to broadcast")
	ErrCoverStranded     = errors.New("cover undo failed; refuse new movement until cover is reversed")
	ErrFreezeGate  = errors.New("freeze gate rejected")
	ErrUnavailable = errors.New("ledger service unavailable")
)
