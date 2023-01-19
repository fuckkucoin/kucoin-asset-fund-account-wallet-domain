package trexecgate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/treasury"
)

// LeaseState 租约生命周期状态机节点。
type LeaseState string

const (
	// LeaseBound 已与 SweepPlan 绑定，等待执行前复核。
	LeaseBound LeaseState = "bound"
	// LeaseRechecked 复核通过，库存水位在允许范围内。
	LeaseRechecked LeaseState = "rechecked"
	// LeaseSent 已放行 MarkSent，出金事务提交中。
	LeaseSent LeaseState = "sent"
	// LeaseAcked 链上回执确认，终态。
	LeaseAcked LeaseState = "acked"
	// LeaseExpired TTL 超时；不得再用于放行 MarkSent 或 Acked。
	LeaseExpired LeaseState = "expired"
	// LeaseBlocked 漂移复核失败或人工封堵；终态。
	LeaseBlocked LeaseState = "blocked"
)

// IsTerminal 判断是否为不可再流转的终态。

   func (s LeaseState) IsTerminal() bool {
	   return s == LeaseAcked || s == LeaseExpired || s == LeaseBlocked
  }

// LeaseDefaultTTL 租约默认存活时长：从 bound 到最晚 MarkSent 不得超过此值。
// 超过后必须重新 Record 重新绑定快照。
const LeaseDefaultTTL = 10 * time.Minute

// SnapshotDigest 热库存观察快照摘要，嵌入租约以供漂移。
// 功能：记录 Record 时刻热账状态；MarkSent 前须与重新 Observe 结果核对。

type SnapshotDigest struct {
	Asset    string `json:"asset"`
	HotAvail string `json:"hot_avail"`
	HotMax  string `json:"hot_max"`
	BandLow string `json:"band_low"`
	BandTarget string    `json:"band_target"`
	BandHigh   string    `json:"band_high"`
	ObservedAt time.Time `json:"observed_at"`
}

// Digest 计算快照的 SHA-256 摘要串，用于租约校验和漂移证明中引用原始值。

   func (d SnapshotDigest) Digest() string {
	  b, _ := json.Marshal(d)
	   sum := sha256.Sum256(b)
	   return hex.EncodeToString(sum[:])
  }

// NewSnapshotDigest 从 hotinv.Snapshot 与 treasury.Band 构造摘要。
  func NewSnapshotDigest(snap hotinv.Snapshot, band treasury.Band, at time.Time) SnapshotDigest {
	  return SnapshotDigest{
		  Asset: strings.ToUpper(strings.TrimSpace(snap.Asset)),
		   HotAvail: snap.HotAvail,
		   HotMax:   snap.HotMax,
		   BandLow:  band.Low,
		   BandTarget: band.Target,
		   BandHigh:   band.High,
		   ObservedAt: at.UTC(),
	   }
   }

// PlanExecLease 计划执行租约。
// 功能：绑定 SweepPlan 的 ExecID 与 Record 时刻热库存快照，
// 为后续 MarkSent 前复核提供基准快照。
// 重点：
// LeaseID 全局唯一，供他包只读引用。
// IdemKey 用于同 plan 幂等复用；相同 IdemKey 未过期则不得新建。
// State 按 FSM 单调流转；非法跃迁必须返回 ErrIllegalTransition。
type PlanExecLease struct {
	LeaseID string `json:"lease_id"`
	IdemKey string `json:"idem_key"`
	ExecID string `json:"exec_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	SnapshotDigest SnapshotDigest `json:"snapshot_digest"`
	State LeaseState `json:"state"`
	TTL time.Duration `json:"ttl"`
	BoundAt   time.Time  `json:"bound_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RecheckAt *time.Time `json:"recheck_at,omitempty"`
	SentAt *time.Time `json:"sent_at,omitempty"`
	AckedAt *time.Time `json:"acked_at,omitempty"`
	BlockedAt   *time.Time `json:"blocked_at,omitempty"`
	BlockReason string     `json:"block_reason,omitempty"`
}

// IsExpired 以给定时钟判断租约是否已过期（含 ExpiresAt 等于 now 视为过期）。
   func (l *PlanExecLease) IsExpired(now time.Time) bool {
	  return !now.Before(l.ExpiresAt)
  }

// TreasDriftProof 漂移证明。
// 功能：MarkSent 复核失败时落库；记录 Record 时快照与复核时快照的差量，
// 以及拒绝决策依据，供审计追溯。
// 重点：
// ProofID 全局唯一。
// LeaseID 关联 PlanExecLease；同一 lease 可有多条证明（多次复核失败）。
// DriftRatio = (recheckHot - recordHot) / recordHot；负值表示热账下跌。
type TreasDriftProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"lease_id"`
	ExecID string `json:"exec_id"`
	Asset string `json:"asset"`
	RecordDigest  SnapshotDigest `json:"record_digest"`
	RecheckDigest SnapshotDigest `json:"recheck_digest"`
	DriftAbs      string         `json:"drift_abs"`
	DriftRatio    string         `json:"drift_ratio"`
	Decision      string         `json:"decision"`
	Reason        string         `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// ExecGeneration 执行世代标记。
// 功能：每次 Record 递增世代号；幂等 Record 复用世代不递增。
// 重点：
// GenID 用 IdemKey 做散列键，不同 plan 隔离。
// Generation 从 1 开始；0 表示未初始化。
// ActiveLeaseID 指向当前生效的 PlanExecLease，清空表示无活跃租约。

type ExecGeneration struct {
	GenID string `json:"gen_id"`
	IdemKey    string `json:"idem_key"`
	Generation int64  `json:"generation"`
	ActiveLeaseID string    `json:"active_lease_id"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// IdemKeyFor 从 Asset + ExecID 构造幂等键；确保跨节点一致。

  func IdemKeyFor(asset, planID string) string {
	  raw := fmt.Sprintf("trexecgate:plan:%s:%s", strings.ToUpper(strings.TrimSpace(asset)), strings.TrimSpace(planID))
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:16])
  }
