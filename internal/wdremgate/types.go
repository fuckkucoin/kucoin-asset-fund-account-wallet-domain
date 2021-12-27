package wdremgate

import (
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// ──────────────────────────────────────────────
// Gate 请求 / 响应
// ──────────────────────────────────────────────
// GateOpenHoldReq 开立 ShardHold 请求（透传到 wdpartial.OpenHoldReq）。

type GateOpenHoldReq struct {
	PlanID string `json:"plan_id"`
	LegID string `json:"leg_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount    string `json:"amount"`
	ExpiryTTL int    `json:"expiry_ttl_sec,omitempty"`
	SkipLeg  bool   `json:"skip_leg,omitempty"`
	LiveHash string `json:"live_hash,omitempty"`
}

// GateIssueLeaseReq 颁发剩余释放租约请求。
type GateIssueLeaseReq struct {
	// PlanID 分片计划 ID。
	PlanID string `json:"plan_id"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// TTLSeconds 租约有效期（秒），默认 300。
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

// GateIssueLeaseResp 颁发剩余释放租约响应。
type GateIssueLeaseResp struct {
	// LeaseID 租约 ID（rem_release_lease_id）。
	LeaseID string `json:"lease_id"`
	// Generation 当前 rem_release_gen（供调用方携带到 GateReleaseReq）。
	Generation int64 `json:"rem_release_gen"`
	// ExpiresAt 租约过期时间。
	ExpiresAt time.Time `json:"expires_at"`
}

// GateReleaseReq 带世代剩余释放请求。

type GateReleaseReq struct {
	// PlanID 分片计划 ID。
	PlanID string `json:"plan_id"`
	// LeaseID 租约 ID。
	LeaseID string `json:"lease_id"`
	// Generation rem_release_gen（须与租约绑定世代一致）。
	Generation int64 `json:"rem_release_gen"`
	// Actor 操作人 ID（用于审计）。
	Actor string `json:"actor"`
}

// GateReleaseResp 剩余释放响应。

type GateReleaseResp struct {
	// PlanID 分片计划 ID。
	PlanID string `json:"plan_id"`
	// LeaseID 租约 ID。
	LeaseID string `json:"lease_id,omitempty"`
	// ReleasedHolds 已释放的 Hold ID 列表（store remgate）。
	ReleasedHolds []string `json:"released_holds,omitempty"`
	// ReleasedCount 已释放 Hold 数量（ShardHold leftover）。
	ReleasedCount int `json:"released_count"`
	// Remainder 已释放总金额（十进制字符串）。
	Remainder string `json:"remainder"`
}

// GateConserveReq 守恒校验请求。
type GateConserveReq struct {
	// PlanID 分片计划 ID。
	PlanID string `json:"plan_id"`
	// Actor 操作人 ID。
	Actor string `json:"actor"`
}

// GateConserveResp 守恒校验响应。
type GateConserveResp struct {
	// PlanID 分片计划 ID。
	PlanID string `json:"plan_id"`
	// Conserved 是否守恒通过。
	Conserved bool `json:"conserved"`
	// CaseID 创建的守恒案件 ID（失败时非空）。
	CaseID string `json:"conserve_case_id,omitempty"`
	// Summary 计划成交汇总。
	Summary wdpartial.PlanFillSummary `json:"summary"`
}

// GateProveConserveReq 守恒案件证明固化请求。
type GateProveConserveReq struct {
	// CaseID 案件 ID。
	CaseID string `json:"case_id"`
	// ProofPayload 证明载荷（JSON 形态）。
	ProofPayload string `json:"proof_payload"`
	// Approver 第一审核人。
	Approver string `json:"approver"`
	// Approver2 双人确认第二审核人。
	Approver2 string `json:"approver2"`
}

// GateForceReleaseReq 双人确认强制释放请求（绕过 ConserveCase 阻断）。
type GateForceReleaseReq struct {
	// PlanID 分片计划 ID。
	PlanID string `json:"plan_id"`
	// CaseID 守恒案件 ID（须先关闭）。
	CaseID string `json:"case_id"`
	// ProofPayload 强制释放原因（须填写）。
	ProofPayload string `json:"proof_payload"`
	// Approver 第一审核人。
	Approver string `json:"approver"`
	// Approver2 第二审核人。
	Approver2 string `json:"approver2"`
}

// GateInvalidateGuardReq 使守卫世代失效请求。
type GateInvalidateGuardReq struct {
	// GuardGenID 守卫世代 ID。
	GuardGenID string `json:"guard_gen_id"`
	// Reason 失效原因。
	Reason string `json:"reason"`
}

// GateConserveCaseExport 守恒证明导出（ops 接口响应）。
type GateConserveCaseExport struct {
	// Case 守恒案件完整记录。
	Case *wdpartial.ConserveCase `json:"case"`
	// Summary 案件关联的计划汇总（检测时快照）。
	Summary wdpartial.PlanFillSummary `json:"summary"`
}

// ──────────────────────────────────────────────
// Worker 配置
// ──────────────────────────────────────────────
// WorkerConfig Worker 定时调度配置。
type WorkerConfig struct {
	// HoldPatrolInterval 开放 Hold 巡检间隔。
	HoldPatrolInterval time.Duration
	// ConserveEscalateInterval 守恒失败升级巡检间隔。
	ConserveEscalateInterval time.Duration
	// LeaseRevokeInterval 过期租约吊销间隔。
	LeaseRevokeInterval time.Duration
	// ScanBatchSize 每轮扫描批量大小。
	ScanBatchSize int
}

// DefaultWorkerConfig 推荐默认配置。

var DefaultWorkerConfig = WorkerConfig{
	HoldPatrolInterval:       2 * time.Minute,
	ConserveEscalateInterval: time.Minute,
	LeaseRevokeInterval: 30 * time.Second,
	ScanBatchSize: 200,
}

// WorkerAlert 告警回调接口（由外部注入 Kafka 或 Webhook 实现）。

type WorkerAlert interface {
	// OnConserveCaseOpen 发现 open 守恒案件时触发。
	OnConserveCaseOpen(c *wdpartial.ConserveCase)
	// OnLeaseExpiredRevoke 租约过期吊销时触发。
	OnLeaseExpiredRevoke(ids []string)
	// OnGuardGenInvalidated 守卫世代失效时触发。
	OnGuardGenInvalidated(g *wdpartial.BroadcastGuardGen)
}

// noopAlert 空告警实现（默认）。
type noopAlert struct{}

  func (n *noopAlert) OnConserveCaseOpen(*wdpartial.ConserveCase)         {}
   func (n *noopAlert) OnLeaseExpiredRevoke([]string)                      {}
   func (n *noopAlert) OnGuardGenInvalidated(*wdpartial.BroadcastGuardGen) {}
