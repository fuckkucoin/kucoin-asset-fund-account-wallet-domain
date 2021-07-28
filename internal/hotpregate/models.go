package hotpregate

import "time"

// LeaseState 预检租约状态。

type LeaseState string

const (
	LeaseIssued LeaseState = "issued"
	LeaseBound LeaseState = "bound"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
	LeaseDrifted LeaseState = "drifted"
)

// DriftSeverity 漂移严重程度。
type DriftSeverity string

const (
	DriftMinor DriftSeverity = "minor" // 在阈值内，仍超过误差带
	DriftMajor    DriftSeverity = "major"    // 超过 CapBand 带宽 10%
	DriftCritical DriftSeverity = "critical" // 穿越 Cap
)

// HotPrecheckLease 热库存预检租约。
// 签发条件：PreSubmit/ReadGate 在只读 Observe 后通过。
// 消费条件：提交决策前调用方携带 hot_precheck_id 并通过复核。
// 隔离保证：本结构不引用 hotbound/hotreserve 世代；与归集完全隔离。

type HotPrecheckLease struct {
	// LeaseID 全局唯一，供外部携带；形态 "hpl_<uuid>"。
	LeaseID string `json:"lease_id"`
	// Asset 币种（大写），不含 network 后缀。
	Asset string `json:"asset"`
	// Window 预检时间窗口标识（yyyyMMddHHmm，按分钟一致）。
	Window string `json:"window"`
	// Digest 签发时刻快照摘要；复核时重新计算并。
	Digest InvSnapshotDigest `json:"digest"`
	// CapSnapshot 签发时刻的 Cap 值（HotMax）。
	CapSnapshot string `json:"cap_snapshot"`
	// BandSnapshot 签发时刻的 GateBand。
	BandSnapshot BandSnapshot `json:"band_snapshot"`
	// State 当前状态。
	State LeaseState `json:"state"`
	// IssuedAt 签发时刻（UTC）。
	IssuedAt time.Time `json:"issued_at"`
	// ExpiresAt 租约过期时刻（IssuedAt + TTL）。
	ExpiresAt time.Time `json:"expires_at"`
	// BoundAt 绑定时刻（State=bound 时设置）。
	BoundAt *time.Time `json:"bound_at,omitempty"`
	// ConsumedAt 消费时刻（State=consumed/drifted 时设置）。
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	// DriftProofID 若漂移拒绝，关联的 InvDriftProof.ProofID。
	DriftProofID string `json:"drift_proof_id,omitempty"`
	// IdemKey 幂等键（asset + ":" + window）；同 asset/window 复用未过期租约。
	IdemKey string `json:"idem_key"`
	// Actor 签发方（调用服务标识）。
	Actor string `json:"actor"`
	// Note 状态变更备注。
	Note string `json:"note,omitempty"`
}

// IsExpired 判断租约是否已超期（以参数 now 为基准）。
   func (l *HotPrecheckLease) IsExpired(now time.Time) bool {
	  return now.After(l.ExpiresAt)
  }

// IsUsable 判断租约是否可用于提交复核（issued 且未过期）。

  func (l *HotPrecheckLease) IsUsable(now time.Time) bool {
	  return (l.State == LeaseIssued || l.State == LeaseBound) && !l.IsExpired(now)
  }

// InvSnapshotDigest 库存快照摘要。
// Digest 字段为 SHA-256(asset || "|" || hotAvail || "|" || warmAvail || "|" ||
// coldAvail || "|" || hotMax || "|" || band.HotLow || "|" || band.HotHigh || "|" ||
// band.WarmLow || "|" || band.WarmHigh || "|" || capturedAt.Unix)

type InvSnapshotDigest struct {
	// Asset 币种。
	Asset string `json:"asset"`
	// HotAvail 热账可用余额（签发时刻）。
	HotAvail string `json:"hot_avail"`
	// WarmAvail 温账可用余额（签发时刻）。
	WarmAvail string `json:"warm_avail"`
	// ColdAvail 冷账可用余额（签发时刻）。
	ColdAvail string `json:"cold_avail"`
	// HotMax 热库上限（Cap）。
	HotMax string `json:"hot_max"`
	// BandHotLow 低水位线。
	BandHotLow string `json:"band_hot_low"`
	// BandHotHigh 高水位线。
	BandHotHigh string `json:"band_hot_high"`
	// BandWarmLow 温账低水位。
	BandWarmLow string `json:"band_warm_low"`
	// BandWarmHigh 温账高水位。
	BandWarmHigh string `json:"band_warm_high"`
	// CapturedAt 快照采集时刻（UTC）。
	CapturedAt time.Time `json:"captured_at"`
	// Hex SHA-256 摘要十六进制字符串（64 位小写）。
	Hex string `json:"hex"`
}

// InvDriftProof 活库存漂移证明。不可变，落库后禁止更新。
// 在复核 Observe 时，若任何字段超出漂移阈值，签发此证明并拒绝提交。
// 与 hotbound/hotreserve 漂移证明命名空间完全隔离（字段无重叠）。

type InvDriftProof struct {
	// ProofID 全局唯一；形态 "idp_<uuid>"。
	ProofID string `json:"proof_id"`
	// LeaseID 关联的 HotPrecheckLease.LeaseID。
	LeaseID string `json:"lease_id"`
	// Asset 币种。
	Asset string `json:"asset"`
	// Window 时间窗口。
	Window string `json:"window"`
	// SnapshotDigestAtIssue 签发时刻摘要（已存入 Lease）。
	SnapshotDigestAtIssue InvSnapshotDigest `json:"snapshot_digest_at_issue"`
	// SnapshotDigestAtRecheck 复核时刻摘要。
	SnapshotDigestAtRecheck InvSnapshotDigest `json:"snapshot_digest_at_recheck"`
	// HotAvailDelta 热账可用余额变化量（recheck - issue，可为负）。
	HotAvailDelta string `json:"hot_avail_delta"`
	// WarmAvailDelta 温账可用余额变化量。
	WarmAvailDelta string `json:"warm_avail_delta"`
	// DigestMismatch 摘要不匹配（任一字段变化）。
	DigestMismatch bool `json:"digest_mismatch"`
	// Severity 漂移严重程度。
	Severity DriftSeverity `json:"severity"`
	// Reason 人类可读的拒绝原因。
	Reason string `json:"reason"`
	// CreatedAt 创建时刻（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// BandSnapshot GateBand 快照（签发时刻），用于复核核对。
type BandSnapshot struct {
	HotLow string `json:"hot_low"`
	HotHigh string `json:"hot_high"`
	WarmLow  string `json:"warm_low"`
	WarmHigh string `json:"warm_high"`
}

// LeaseIssuedEvent 租约签发事件（用于 outbox 发布）。

type LeaseIssuedEvent struct {
	LeaseID  string    `json:"lease_id"`
	Asset    string    `json:"asset"`
	Window   string    `json:"window"`
	IdemKey  string    `json:"idem_key"`
	IssuedAt time.Time `json:"issued_at"`
}

// DriftDetectedEvent 漂移检测事件（用于 outbox 发布）。
type DriftDetectedEvent struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"lease_id"`
	Asset    string        `json:"asset"`
	Severity DriftSeverity `json:"severity"`
	Reason   string        `json:"reason"`
}

// IssueRequest 预检租约签发请求。

type IssueRequest struct {
	// Asset 目标币种（大写）。
	Asset string `json:"asset"`
	// Window 时间窗口（yyyyMMddHHmm）；空时由 Service 自动填充当前分钟。
	Window string `json:"window,omitempty"`
	// Band 当前闸门水位配置。
	Band BandSnapshot `json:"band"`
	// TTL 租约有效期；0 时使用默认值（DefaultLeaseTTL）。
	TTL int64 `json:"ttl_seconds,omitempty"`
	// Actor 签发方标识。
	Actor string `json:"actor"`
}

// BindRequest 绑定租约请求（提交前调用，将 issued→bound）。
type BindRequest struct {
	LeaseID string `json:"lease_id"`
	Actor string `json:"actor"`
}

// RecheckRequest 提交复核请求。
type RecheckRequest struct {
	LeaseID string `json:"lease_id"`
	Actor string `json:"actor"`
}

// RecheckResult 提交复核结果。

type RecheckResult struct {
	// Accepted true=无漂移，可接着提交；false=漂移拒绝。
	Accepted bool `json:"accepted"`
	// Lease 复核后的租约快照（已更新状态）。
	Lease *HotPrecheckLease `json:"lease"`
	// DriftProof 漂移证明（仅 Accepted=false 时非 nil）。
	DriftProof *InvDriftProof `json:"drift_proof,omitempty"`
}

// LeaseListFilter 租约列表过滤器。

type LeaseListFilter struct {
	Asset string `json:"asset,omitempty"`
	State LeaseState `json:"state,omitempty"`
	Limit int        `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// DriftProofListFilter 漂移证明列表过滤器。

type DriftProofListFilter struct {
	Asset    string        `json:"asset,omitempty"`
	Severity DriftSeverity `json:"severity,omitempty"`
	Limit    int           `json:"limit,omitempty"`
	Offset   int           `json:"offset,omitempty"`
}
