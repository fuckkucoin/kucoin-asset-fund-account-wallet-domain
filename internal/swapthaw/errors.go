package swapthaw

import "errors"

var (
	// ErrLeaseNotFound 效果令牌 / 冻额世代记录不存在。
	ErrLeaseNotFound = errors.New("swapthaw: freeze lease not found")
	// ErrLeaseExpired 效果令牌已过期，本次 failUnfreeze/settle 须拒绝。
	ErrLeaseExpired = errors.New("swapthaw: freeze lease expired")
	// ErrLeaseConflict failUnfreeze 或 settle 携带的令牌与库中记录不符。
	ErrLeaseConflict = errors.New("swapthaw: freeze lease token mismatch")
	// ErrThawMutexDenied 互斥锁已被另一条终态腿占据，本次操作被拒。
	ErrThawMutexDenied = errors.New("swapthaw: thaw mutex denied; another terminal leg already claimed")
	// ErrDoubleRelease 在已 settle 或已 thawed 的订单上再次触发解冻——双释事故。
	ErrDoubleRelease = errors.New("swapthaw: double release detected; ThawCase must be opened")
	// ErrThawCaseNotFound ThawCase 记录不存在。
	ErrThawCaseNotFound = errors.New("swapthaw: thaw case not found")
	// ErrThawCaseProofMissing ThawCase 未通过证明校验，禁止关闭。
	ErrThawCaseProofMissing = errors.New("swapthaw: thaw case proof missing; cannot close without verified proof")
	// ErrThawCaseOpen 存在未关闭 ThawCase，新 Quote 冻额被阻断。
	ErrThawCaseOpen = errors.New("swapthaw: open thaw case blocks new freeze for this user")
	// ErrSettleBindNotFound SettleBind 记录不存在。
	ErrSettleBindNotFound = errors.New("swapthaw: settle bind not found")
	// ErrSettleBindConflict SettleBind 已存在且指向不同终态腿。
	ErrSettleBindConflict = errors.New("swapthaw: settle bind conflict; order already has a terminal leg")
	// ErrGenerationCAS 世代 CAS 写失败，说明有并发写路径抢先修改了世代。
	ErrGenerationCAS = errors.New("swapthaw: freeze generation CAS conflict")
	// ErrOrphanFrozen 孤儿 Frozen——冻额无对应活跃 Lease，需人工审核。
	ErrOrphanFrozen = errors.New("swapthaw: orphan frozen detected; no active lease covers this order")
	// ErrProofInvalid 证明内容校验失败（金额不匹配或签名错误）。
	ErrProofInvalid = errors.New("swapthaw: thaw proof invalid")
)
