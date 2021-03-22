package balproofgate

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// 枚举
// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus 描述 ProofLease 在状态机中的位置。
type LeaseStatus string

const (
	LeaseIssued LeaseStatus = "issued" // Issue 已落库，快照根已计算，尚未绑定闸
	LeaseBound LeaseStatus = "bound" // 已绑定世代与 HMAC，可供闸消费复核
	LeaseConsumed LeaseStatus = "consumed" // 闸消费成功终态
	LeaseExpired  LeaseStatus = "expired"  // TTL 到期，旧签名不得放行
	LeaseRevoked  LeaseStatus = "revoked"  // 显式吊销，旧签名不得放行
)

// DriftStatus 描述 DriftProof 生命周期。

type DriftStatus string

const (
	DriftOpen DriftStatus = "open" // 复核发现漂移，案已开立
	DriftProven DriftStatus = "proven" // 摘要已附，证明可核验
	DriftClosed DriftStatus = "closed" // 运维收口
)

// DriftKind 漂移类型，仅覆盖单科目活账事故面。
type DriftKind string

const (
	DriftAvailCredit DriftKind = "avail_credit" // 签发后 Available 被贷记抬升
	DriftAvailDebit DriftKind = "avail_debit" // 签发后 Available 被借记下降
	DriftFrozenUp   DriftKind = "frozen_up"   // 冻结增加
	DriftFrozenDown DriftKind = "frozen_down" // 冻结减少
	DriftObserveGen DriftKind = "observe_gen" // ledger 观察世代前进
)

// 默认租约 TTL：与 balproof 查询网关窗口一致，但不复用其 nonce 仓。
const DefaultLeaseTTL = 15 * time.Minute

// IsolationMark 明确本包证明与相邻包隔离，写入导出摘要。
const IsolationMark = "balproof-live-hmac;not-por-merkle;not-stmt-manifest;not-leddiff"

// ─────────────────────────────────────────────────────────────────────────────
// ProofGeneration
// ─────────────────────────────────────────────────────────────────────────────
// ProofGeneration 记录科目证明世代 CAS 计数器。
// CAS 协议：
// Acquire（Issue）:
// UPDATE balproof_generations
// SET generation = generation + 1, updated_at = NOW(3)
// WHERE user_id=? AND book=? AND asset=? AND generation=?
// -- 影响行数 = 0 → ErrStaleGen；= 1 → 新世代 = old+1
// Insert-first:
// INSERT ... ON CONFLICT (user_id, book, asset) DO NOTHING
// -- 冲突则对方已插入，重试 SELECT + UPDATE

type ProofGeneration struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Asset string `json:"asset"`
	Generation int64 `json:"generation"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 返回值拷贝，避免调用方持有仓内指针。

  func (g ProofGeneration) Copy() ProofGeneration { return g }

// ─────────────────────────────────────────────────────────────────────────────
// LiveSnapshotRoot
// ─────────────────────────────────────────────────────────────────────────────
// LiveSnapshotRoot 是签发瞬间的活账观察结果，闸消费必须按此复核。
// RootDigest 仅绑定单科目 HMAC 规范串，禁止写入 por 持续树。

type LiveSnapshotRoot struct {
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Total      string `json:"total"`
	ObserveGen int64  `json:"observe_gen"`
	ObservedAt time.Time `json:"observed_at"`
	RootDigest    string `json:"root_digest"`
	IsolationMark string `json:"isolation_mark"`
}

// EqualLive 比较 Available/Frozen/ObserveGen 是否仍与租约绑定一致。
   func (r LiveSnapshotRoot) EqualLive(avail, frozen string, observeGen int64) bool {
	   return stringsEqual(r.Available, avail) &&
		  stringsEqual(r.Frozen, frozen) &&
		  r.ObserveGen == observeGen
  }

   func stringsEqual(a, b string) bool {
	   return trim(a) == trim(b)
  }

  func trim(s string) string {
	   i, j := 0, len(s)
	   for i < j && (s[i] == ' ' || s[i] == '\t') {
		   i++
	  }
	   for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		  j--
	   }
	   return s[i:j]
  }

// ─────────────────────────────────────────────────────────────────────────────
// ProofLease
// ─────────────────────────────────────────────────────────────────────────────
// ProofLease 是签发必须落库的租约。闸消费只认 bound 且未过期未吊销的租约。
type ProofLease struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Asset string `json:"asset"`
	Nonce string `json:"nonce"`
	Generation int64            `json:"generation"`
	Status     LeaseStatus      `json:"status"`
	Snapshot   LiveSnapshotRoot `json:"snapshot"`
	Canonical string `json:"canonical"`
	Signature string    `json:"signature"`
	IssuedAt  time.Time `json:"issued_at"`
	BoundAt    *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
	ProofLeaseID string `json:"proof_lease_id"` // 对外只读标识，恒等于 ID
	IsolationMark string `json:"isolation_mark"`
}

// Copy 深拷贝租约，Snapshot 为值类型。
   func (l ProofLease) Copy() ProofLease {
	  out := l
	  if l.BoundAt != nil {
		  t := *l.BoundAt
		   out.BoundAt = &t
	   }
	  if l.ConsumedAt != nil {
		   t := *l.ConsumedAt
		  out.ConsumedAt = &t
	  }
	   if l.RevokedAt != nil {
		   t := *l.RevokedAt
		   out.RevokedAt = &t
	   }
	   return out
   }

// Terminal 终态：consumed / expired / revoked 均不可再消费。
   func (l ProofLease) Terminal() bool {
	  switch l.Status {
	  case LeaseConsumed,
		   LeaseExpired, LeaseRevoked:
		  return true
	  default:
		   return false
	   }
  }

// Consumable 闸消费前置：必须 bound，且未过期。

  func (l ProofLease) Consumable(now time.Time) bool {
	   if l.Status != LeaseBound {
		   return false
	   }
	  if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
		   return false
	  }
	   return true
  }

// ─────────────────────────────────────────────────────────────────────────────
// DriftProof
// ─────────────────────────────────────────────────────────────────────────────
// DriftProof 记录签发后活额漂移。扫描源必须是 balproof 事件，不得改写 por Root。

type DriftProof struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID string `json:"user_id"`
	Book   string      `json:"book"`
	Asset  string      `json:"asset"`
	Kind   DriftKind   `json:"kind"`
	Status DriftStatus `json:"status"`
	IssuedAvail string `json:"issued_avail"`
	IssuedFrozen string `json:"issued_frozen"`
	IssuedObsGen int64  `json:"issued_observe_gen"`
	LiveAvail    string `json:"live_avail"`
	LiveFrozen   string `json:"live_frozen"`
	LiveObsGen int64     `json:"live_observe_gen"`
	Digest     string    `json:"digest"`
	OpenedAt   time.Time `json:"opened_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	Closer string `json:"closer,omitempty"`
	IsolationMark string `json:"isolation_mark"`
}

// Copy 值拷贝。

  func (d DriftProof) Copy() DriftProof {
	   out := d
	   if d.ProvenAt != nil {
		  t := *d.ProvenAt
		  out.ProvenAt = &t
	   }
	  if d.ClosedAt != nil {
		  t := *d.ClosedAt
		  out.ClosedAt = &t
	   }
	  return out
   }

// Blocking 存在 open/proven 漂移时挡同科目再消费。

  func (d DriftProof) Blocking() bool {
	   return d.Status == DriftOpen || d.Status == DriftProven
  }

// ─────────────────────────────────────────────────────────────────────────────
// Revocation
// ─────────────────────────────────────────────────────────────────────────────
// Revocation 吊销记录。与过期互斥：任一命中即拒旧 Signature。

type Revocation struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID string `json:"user_id"`
	Book string `json:"book"`
	Asset  string `json:"asset"`
	Nonce  string `json:"nonce"`
	Reason string `json:"reason"`
	Operator string `json:"operator"`
	RevokedAt time.Time `json:"revoked_at"`
	Propagated bool `json:"propagated"`
}

// Copy 值拷贝。

   func (r Revocation) Copy() Revocation { return r }

// ─────────────────────────────────────────────────────────────────────────────
// 请求 / 结果
// ─────────────────────────────────────────────────────────────────────────────
// IssueReq 签发请求。同 user/book/asset/nonce 幂等返回既有未过期证明。
type IssueReq struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Asset  string `json:"asset"`
	Nonce  string `json:"nonce"`
	TTLSec int64  `json:"ttl_sec,omitempty"`
}

// RecheckReq 闸消费前活额复核。

type RecheckReq struct {
	LeaseID   string `json:"lease_id"`
	Signature string `json:"signature"`
}

// ConsumeReq 闸消费：复核通过后推进 consumed。
type ConsumeReq struct {
	LeaseID string `json:"lease_id"`
	Signature string `json:"signature"`
	GateRef string `json:"gate_ref"`
}

// RevokeReq 吊销。

type RevokeReq struct {
	LeaseID string `json:"lease_id"`
	Reason string `json:"reason"`
	Operator string `json:"operator"`
}

// ExportBundle 证明导出，仅含本包字段。

type ExportBundle struct {
	ProofLeaseID string     `json:"proof_lease_id"`
	Lease        ProofLease `json:"lease"`
	Drifts []DriftProof `json:"drifts"`
	Revocation    *Revocation `json:"revocation,omitempty"`
	IsolationMark string      `json:"isolation_mark"`
	ExportedAt time.Time `json:"exported_at"`
}

// LiveView 活账读取结果。ObserveGen 来自账本观察世代只读接口。

type LiveView struct {
	Available string
	Frozen     string
	ObserveGen int64
	At time.Time
}
// temporary guard removed after the production check
