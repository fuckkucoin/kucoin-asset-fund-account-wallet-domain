package feepathgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// Kind 表示费贷路径类型；与 feecredit.Kind 值对应，此处重新声明避免 import cycle。
type Kind string

const (
	KindTake Kind = "take"
	KindRebate Kind = "rebate"
)

  func (k Kind) Valid() bool { return k == KindTake || k == KindRebate }

// Opposite 返回另一条路径的 Kind；用于路径互斥判断。
  func (k Kind) Opposite() Kind {
	  if k == KindTake {
		  return KindRebate
	  }
	   return KindTake
   }

// LeaseStatus 是 PathLease 状态机的所有合法状态。
type LeaseStatus string

const (
	// LeaseOpen 初始态：已分配 lease_id，world_key 已注册，等待与 Credit 绑定。
	LeaseOpen LeaseStatus = "open"
	// LeaseBound 已绑定态：lease 与具体 Credit ID 和 LedgerID 关联，等待对冲证明。
	LeaseBound LeaseStatus = "bound"
	// LeaseSettled 终态（成功）：对冲证明通过，双路径贷记已核验完毕。
	LeaseSettled LeaseStatus = "settled"
	// LeaseConflicted 终态（冲突）：同一 business_key 经两条 Kind 出现双贷，
	// 需人工双人确认后才能关闭。
	LeaseConflicted LeaseStatus = "conflicted"
)

// DualHitStatus 是 DualHit 的生命周期状态。

type DualHitStatus string

const (
	DualHitOpen   DualHitStatus = "open"
	DualHitProven DualHitStatus = "proven"
	DualHitClosed DualHitStatus = "closed"
)

// HedgeStatus 是对冲证明的验证状态。
type HedgeStatus string

const (
	HedgePending HedgeStatus = "pending"
	HedgeVerified HedgeStatus = "verified"
	HedgeFailed   HedgeStatus = "failed"
)

// PathLease 表示一个 business_key 与路径 Kind 的独占租约。
// 状态机：
// open → bound → settled
// open → conflicted
// bound → conflicted
// CAS 协议：
// Acquire: INSERT gen=1 WHERE NOT EXISTS(same biz_key, any kind) or UPDATE gen+1 WHERE gen=expected
// Release: UPDATE status=released WHERE lease_id=? AND worker_id=? AND gen=?
type PathLease struct {
	ID string `json:"lease_id"`
	BusinessKey string `json:"business_key"`
	Kind        Kind   `json:"kind"`
	Asset       string `json:"asset"`
	Gross string `json:"gross"`
	Fee   string `json:"fee"`
	Net   string `json:"net"`
	Generation int64  `json:"generation"`
	WorkerID   string `json:"worker_id"`
	CreditID string `json:"credit_id,omitempty"`
	LedgerID string `json:"ledger_id,omitempty"`
	ProofID  string `json:"proof_id,omitempty"`
	Status     LeaseStatus `json:"status"`
	ConflictAt *time.Time  `json:"conflict_at,omitempty"`
	SettledAt  *time.Time  `json:"settled_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PathGeneration 记录每个 business_key 的当前世代号，供 CAS 防双 worker 竞态。

type PathGeneration struct {
	ID          string `json:"id"`
	BusinessKey string `json:"business_key"`
	Generation  int64  `json:"generation"`
	LockedBy   string `json:"locked_by"` // lease_id
	LockedKind Kind   `json:"locked_kind"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DualHit 记录同一 business_key 经 Take 与 Rebate 双路径均产生贷记的事实。
// 发现时立刻创建；对冲证明通过后 status→proven；人工核实后 status→closed。
type DualHit struct {
	ID          string `json:"dual_hit_id"`
	BusinessKey string `json:"business_key"`
	Asset        string `json:"asset"`
	TakeLeaseID  string `json:"take_lease_id"`
	TakeCreditID string `json:"take_credit_id"`
	TakeLedgerID string `json:"take_ledger_id"`
	TakeGross    string `json:"take_gross"`
	TakeFee string `json:"take_fee"`
	TakeNet string `json:"take_net"`
	RebateLeaseID string `json:"rebate_lease_id"`
	RebateCreditID string `json:"rebate_credit_id"`
	RebateLedgerID string `json:"rebate_ledger_id"`
	RebateGross    string `json:"rebate_gross"`
	RebateFee      string `json:"rebate_fee"`
	RebateNet string `json:"rebate_net"`
	// Severity 严重程度：high / medium / low
	// high — 双贷金额均非零且无任何证明
	// medium — 有一方已有证明或对冲不完全
	// low — 金额极小（<最小精度阈值）
	Severity string `json:"severity"`
	Status     DualHitStatus `json:"status"`
	ProofID    string        `json:"proof_id,omitempty"`
	DetectedAt time.Time     `json:"detected_at"`
	ResolvedAt *time.Time    `json:"resolved_at,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty"`
}

// HedgeProof 是费收取与返佣贷记的对冲证明。
// 证明字段：
// - Digest = SHA256(business_key ‖ asset ‖ gross_take ‖ fee_take ‖ net_take
// ‖ gross_rebate ‖ fee_rebate ‖ net_rebate ‖ sorted(ledger_ids…))
// 验证：对冲平衡条件
// - fee_take == fee_rebate（同一资产同一金额，否则失衡）
// - gross_take、net_take、fee_take 守恒（由 feecredit 业务层保证，此处核对）
// - LedgerIDs 集合非空，哈希与 Digest 一致

type HedgeProof struct {
	ID string `json:"proof_id"`
	BusinessKey string `json:"business_key"`
	Asset       string `json:"asset"`
	GrossTake string `json:"gross_take"`
	FeeTake   string `json:"fee_take"`
	NetTake   string `json:"net_take"`
	GrossRebate string `json:"gross_rebate"`
	FeeRebate   string `json:"fee_rebate"`
	NetRebate   string `json:"net_rebate"`
	LedgerIDs []string    `json:"ledger_ids"`
	Digest    string      `json:"digest"`
	Status    HedgeStatus `json:"status"`
	TakeLeaseID string `json:"take_lease_id"`
	RebateLeaseID string     `json:"rebate_lease_id"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ComputeDigest 计算并返回对冲证明摘要。
// 摘要采用 SHA-256，输入字段按固定顺序拼接，ledger_ids 先排序再拼接。

   func (p *HedgeProof) ComputeDigest() string {
	  ids := append([]string(nil), p.LedgerIDs...)
	  sort.Strings(ids)
	  raw := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s",
		  p.BusinessKey, p.Asset,
		  p.GrossTake, p.FeeTake, p.NetTake,
		   p.GrossRebate, p.FeeRebate, p.NetRebate,
		  joinSorted(ids),
	  )
	   h := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(h[:])
   }

  func joinSorted(ss []string) string {
	  out := ""
	   for i, s := range ss {
		   if i > 0 {
			  out += ","
		   }
		  out += s
	   }
	  return out
  }

// ReplayBlock 幂等复用冲突封堵记录。
// 当同一 idem_key 被以不同 Kind/Asset/Gross 重用时，
// 将冲突请求记入此表并返回 ErrReplayConflict。
type ReplayBlock struct {
	ID      string `json:"id"`
	IdemKey string `json:"idem_key"`
	BusinessKey string `json:"business_key"`
	OrigKind    Kind   `json:"orig_kind"`
	OrigAsset   string `json:"orig_asset"`
	OrigGross string `json:"orig_gross"`
	ReqKind  Kind   `json:"req_kind"`
	ReqAsset string `json:"req_asset"`
	ReqGross string `json:"req_gross"`
	BlockedAt time.Time `json:"blocked_at"`
}

// AcquireReq 申请路径租约的请求参数。
type AcquireReq struct {
	BusinessKey string `json:"business_key"`
	IdemKey     string `json:"idem_key"`
	Kind Kind `json:"kind"`
	Asset string `json:"asset"`
	Gross string `json:"gross"`
	// WorkerID 调用方标识（hostname:pid:goroutine），
	// 用于跨调用验证同一 worker 续期与释放。
	WorkerID string `json:"worker_id"`
}

// BindReq 将已 open 的 PathLease 与具体 Credit/LedgerID 绑定的请求。
type BindReq struct {
	LeaseID string `json:"lease_id"`
	CreditID string `json:"credit_id"`
	LedgerID string `json:"ledger_id"`
	Fee      string `json:"fee"`
	Net      string `json:"net"`
	WorkerID string `json:"worker_id"`
}

// SettleReq 对冲证明固化后请求将 lease 标记为 settled 的参数。

type SettleReq struct {
	LeaseID string `json:"lease_id"`
	ProofID string `json:"proof_id"`
}

// DualHitResolveReq 人工双人确认关闭 DualHit 的请求参数。

type DualHitResolveReq struct {
	DualHitID  string `json:"dual_hit_id"`
	ResolvedBy string `json:"resolved_by"`
	Note string `json:"note"`
}

// ProofBuildReq 请求固化一对 Take/Rebate 租约的对冲证明。
type ProofBuildReq struct {
	TakeLeaseID   string `json:"take_lease_id"`
	RebateLeaseID string `json:"rebate_lease_id"`
}

// ListLeasesReq 查询租约列表参数。
type ListLeasesReq struct {
	BusinessKey string `json:"business_key,omitempty"`
	Status LeaseStatus `json:"status,omitempty"`
	Kind   Kind        `json:"kind,omitempty"`
	Limit  int         `json:"limit,omitempty"`
}
