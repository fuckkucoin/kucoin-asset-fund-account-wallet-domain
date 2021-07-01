package fxonce

import "fmt"

// ────────────────────────────────────────────────────────────────────────────
// 效果闸与 CAS 错误
// ────────────────────────────────────────────────────────────────────────────

var (
	// ErrEffectConflict effect_key 已存在且终态不同，禁止覆盖。
	ErrEffectConflict = fmt.Errorf("fxonce: effect key conflict")
	// ErrEffectNotFound 无匹配 effect 记录。
	ErrEffectNotFound = fmt.Errorf("fxonce: effect not found")
	// ErrStalegen 投递世代已过期（expected_gen < applied_gen）。
	// 主切换或重投时触发；不得产生第二笔账本意图。
	ErrStalegen = fmt.Errorf("fxonce: stale delivery generation")
	// ErrDupgen 投递世代重复提交（expected_gen == applied_gen 且已 applied）。
	// 幂等回放路径；调用方直接返回胜者结果。
	ErrDupgen = fmt.Errorf("fxonce: duplicate delivery generation")
	// ErrPoisoned 效果记录已进毒丸隔离，不得再产生账本意图。
	// 需运营双人放行（ForceReplay）才能解除。
	ErrPoisoned = fmt.Errorf("fxonce: effect poisoned — manual release required")
	// ErrKindNotFund kind 未归类为资金类，不经过效果闸。
	// 非资金 kind 可旁路；调用层须自行决定旁路行为。
	ErrKindNotFund = fmt.Errorf("fxonce: kind is not a fund kind — gate bypass allowed")
	// ErrKindBlocked kind 已被 KindPolicy 显式封禁。
	ErrKindBlocked = fmt.Errorf("fxonce: kind blocked by policy")
	// ErrBadBusinessKey business_key 为空或形态非法。
	ErrBadBusinessKey = fmt.Errorf("fxonce: bad business key")
	// ErrBadEffectKey effect_key 形态非法（长度/字符）。
	ErrBadEffectKey = fmt.Errorf("fxonce: bad effect key")
	// ErrGenOverflow 世代计数溢出（超过 MaxGeneration）。
	ErrGenOverflow = fmt.Errorf("fxonce: generation counter overflow")
	// ErrStoreUnavailable 效果仓后端不可用（nil store / closed）。
	ErrStoreUnavailable = fmt.Errorf("fxonce: store unavailable")
	// ErrDupProofConflict dup_proof 主键冲突（同一 proof_key 已有记录）。
	ErrDupProofConflict = fmt.Errorf("fxonce: dup proof key conflict")
	// ErrForceReplayDenied 强制重放请求未满足双人审批或权限不足。
	ErrForceReplayDenied = fmt.Errorf("fxonce: force replay denied — dual approval required")
	// ErrWindowExpired 双花证明已超过查询保留窗口（默认 90 天）。
	ErrWindowExpired = fmt.Errorf("fxonce: dup proof window expired")
)

// RejectCode 拒绝/双花码，用于 DupProof.RejectCode 和 DeliveryGen.RejectReason。

type RejectCode string

const (
	// RejectStalegen 旧世代被新世代覆盖拒绝。
	RejectStalegen RejectCode = "stale_gen"
	// RejectDupgen 同世代重复提交（已 applied）。
	RejectDupgen RejectCode = "dup_gen"
	// RejectPoisoned 效果已毒丸隔离。
	RejectPoisoned RejectCode = "poisoned"
	// RejectKindBlocked kind 被策略封禁。
	RejectKindBlocked RejectCode = "kind_blocked"
	// RejectHTTPRateLimit HTTP 限速；不计入资金 exactly-once，不产生 dup proof。
	// 此码专供非资金链路调试，严禁与资金双花封堵逻辑混用。
	RejectHTTPRateLimit RejectCode = "http_rate_limit"
	// RejectHandlerError handler 返回业务错误（非 rate limit），触发退回重试。
	RejectHandlerError RejectCode = "handler_error"
)
