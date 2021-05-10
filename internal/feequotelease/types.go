package feequotelease

import "time"

// LeaseState 报价租约 FSM 状态。
// FSM: quoted → applied / expired / revoked
type LeaseState string

const (
	// LeaseStateQuoted 已签发，等待 ApplyBound 消费。
	LeaseStateQuoted LeaseState = "quoted"
	// LeaseStateApplied Apply 成功消费，租约终结。
	LeaseStateApplied LeaseState = "applied"
	// LeaseStateExpired TTL 超时，租约自然失效。
	LeaseStateExpired LeaseState = "expired"
	// LeaseStateRevoked Schedule/DefaultTiers 变更后主动吊销。
	LeaseStateRevoked LeaseState = "revoked"
)

// FeeQuoteLease 手续费报价租约。
// QuoteLease 签发时绑定 sch_gen（Schedule 世代序号）与 tiers 指纹，
// ApplyBound 时必须复验二者；若 sch_gen 已抬升或指纹不匹配，
// 则视为档位漂移，拒绝扣费并落 FeeQuoteDriftProof。
// 表：fee_quote_leases

type FeeQuoteLease struct {
	// ID 租约唯一标识（UUID）。
	ID string `json:"id"`
	// Kind 手续费种类（withdraw / deposit / transfer / hop / earn_redeem）。
	Kind string `json:"kind"`
	// Asset 资产代码（如 BTC / USDT / * ）。
	Asset string `json:"asset"`
	// Gross 报价本金金额（原始字符串，精度由 money 包保证）。
	Gross string `json:"gross"`
	// Volume 30 日名义交易量（用于分档，可为空表示不分档）。
	Volume string `json:"volume,omitempty"`
	// QuotedBPS 报价时选定的 BPS 值。
	QuotedBPS int `json:"quoted_bps"`
	// QuotedFee 报价费用金额。
	QuotedFee string `json:"quoted_fee"`
	// QuotedNet 报价净额。
	QuotedNet string `json:"quoted_net"`
	// ScheduleID 绑定的 Schedule.ID。
	ScheduleID string `json:"schedule_id"`
	// SchGen 签发时的 Schedule 世代序号（ScheduleGeneration.Gen）。
	SchGen int64 `json:"sch_gen"`
	// TiersFingerprint 签发时的 tiers 指纹（SHA-256 hex，空串表示未分档）。
	TiersFingerprint string `json:"tiers_fingerprint,omitempty"`
	// State 租约 FSM 状态。
	State LeaseState `json:"state"`
	// ExpiresAt 租约过期时刻（TTL 决定）。
	ExpiresAt time.Time `json:"expires_at"`
	// QuotedAt 签发时刻。
	QuotedAt time.Time `json:"quoted_at"`
	// AppliedAt Apply 消费时刻；为空表示未消费。
	AppliedAt *time.Time `json:"applied_at,omitempty"`
	// ExpiredAt 过期时刻；为空表示未过期。
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	// RevokedAt 吊销时刻；为空表示未吊销。
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// RevokeReason 吊销原因（Schedule 变更时写入 kind|asset|new_sch_gen）。
	RevokeReason string `json:"revoke_reason,omitempty"`
	// IdempotencyKey 幂等键（调用方生成，ApplyBound 重复提交时返回相同结果）。
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// ScheduleGeneration 档位表世代记录。
// 每次 PutSchedule（含 DefaultTiers 变更）均抬升 gen；
// 吊销以本 gen 作为新旧：old_gen < current_gen 的 lease 需被吊销。
// 表：fee_sch_generations
type ScheduleGeneration struct {
	// ID 世代记录唯一标识（UUID）。
	ID string `json:"id"`
	// Kind 手续费种类。
	Kind string `json:"kind"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Gen 单调递增世代序号（从 1 开始）。
	Gen int64 `json:"gen"`
	// ScheduleID 触发本次世代的 Schedule.ID。
	ScheduleID string `json:"schedule_id"`
	// BPS 本次生效 BPS。
	BPS int `json:"bps"`
	// TiersFingerprint 本次生效 tiers 指纹（若适用）。
	TiersFingerprint string `json:"tiers_fingerprint,omitempty"`
	// PutAt 本次 Put 发生时刻。
	PutAt time.Time `json:"put_at"`
	// PutBy 操作来源标识（ops user / system）。
	PutBy string `json:"put_by,omitempty"`
}

// FeeQuoteDriftProof 手续费报价档位漂移事故证明。
// 当 ApplyBound 检测到 sch_gen 已抬升或 tiers 指纹不匹配时，
// 拒绝扣费并追加一条不可变的漂移证明。只追加，不修改。
// 表：fee_quote_proofs

type FeeQuoteDriftProof struct {
	// ID 证明唯一标识（UUID）。
	ID string `json:"id"`
	// Kind 漂移证明类型。
	Kind DriftProofKind `json:"kind"`
	// LeaseID 对应的 FeeQuoteLease.ID。
	LeaseID string `json:"lease_id"`
	// FeeKind 手续费种类。
	FeeKind string `json:"fee_kind"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Gross 原始本金。
	Gross string `json:"gross"`
	// BindSchGen 租约绑定时的世代序号。
	BindSchGen int64 `json:"bind_sch_gen"`
	// CurrentSchGen Apply 时当前世代序号。
	CurrentSchGen int64 `json:"current_sch_gen"`
	// BindFingerprint 租约绑定时的 tiers 指纹。
	BindFingerprint string `json:"bind_fingerprint,omitempty"`
	// CurrentFingerprint Apply 时当前 tiers 指纹。
	CurrentFingerprint string `json:"current_fingerprint,omitempty"`
	// BindBPS 租约绑定时的 BPS。
	BindBPS int `json:"bind_bps"`
	// CurrentBPS Apply 时当前 BPS。
	CurrentBPS int `json:"current_bps"`
	// Evidence 事故现场摘要（JSON 片段）。
	Evidence string `json:"evidence"`
	// DetectedAt 检测时刻。
	DetectedAt time.Time `json:"detected_at"`
	// ExportedAt 已导出时刻；空表示未导出。
	ExportedAt *time.Time `json:"exported_at,omitempty"`
}

// DriftProofKind 漂移事故证明类型。
type DriftProofKind string

const (
	// DriftSchGenAdvanced sch_gen 已抬升：Apply 时 current_sch_gen > bind_sch_gen。
	DriftSchGenAdvanced DriftProofKind = "sch_gen_advanced"
	// DriftFingerprintMismatch tiers 指纹不匹配（相同 sch_gen 但指纹变化，理论上不应发生）。
	DriftFingerprintMismatch DriftProofKind = "fingerprint_mismatch"
	// DriftLeaseExpired 租约已过期。
	DriftLeaseExpired DriftProofKind = "lease_expired"
	// DriftLeaseRevoked 租约已被吊销（Schedule 变更触发）。
	DriftLeaseRevoked DriftProofKind = "lease_revoked"
	// DriftLeaseConsumed 租约已被消费（防重复 Apply）。
	DriftLeaseConsumed DriftProofKind = "lease_consumed"
	// DriftNoLease Apply 未携带租约（fail-closed 拒绝）。
	DriftNoLease DriftProofKind = "no_lease"
)

// ApplyResult Apply 消费租约的结果。

type ApplyResult struct {
	// Lease 已消费的租约（状态已更新为 applied）。
	Lease FeeQuoteLease `json:"lease"`
	// Idempotent 是否为幂等重放（已消费过同一 idempotency_key）。
	Idempotent bool `json:"idempotent"`
}

// QuoteLeaseReq QuoteLease 请求参数。
type QuoteLeaseReq struct {
	Kind string `json:"kind"`
	Asset string `json:"asset"`
	Gross      string `json:"gross"`
	Volume     string `json:"volume,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// ApplyBoundReq ApplyBound 请求参数。

type ApplyBoundReq struct {
	LeaseID        string `json:"lease_id"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// SchedulePutReq PutSchedule 请求参数（含 sch_gen 抬升）。

type SchedulePutReq struct {
	Kind string `json:"kind"`
	Asset  string `json:"asset"`
	BPS    int    `json:"bps"`
	MinFee string `json:"min_fee,omitempty"`
	MaxFee string `json:"max_fee,omitempty"`
	Flat string `json:"flat,omitempty"`
	Active bool   `json:"active"`
	PutBy  string `json:"put_by,omitempty"`
}
