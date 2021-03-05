package breachclosegate

import "time"

// BreachState BreachLease 状态。
type BreachState string

const (
	// BreachStateBlocking 活跃挡写状态。出金 Submit/Decide(allow) 在此状态下一律拒绝。
	BreachStateBlocking BreachState = "blocking"
	// BreachStateExpiredCase 关联案件已被强制 Expire；违约记录归档，出金已 fail-closed。
	BreachStateExpiredCase BreachState = "expired_case"
	// BreachStateCleared 四眼清障完成；挡写解除。清障本身不等于放行出金：
	// withdraw 须重新进入 reviewsla / compliance 路径，方可被 allow。
	BreachStateCleared BreachState = "cleared"
)

// Active 判断 BreachLease 是否仍在挡写中。
   func (s BreachState) Active() bool { return s == BreachStateBlocking }

// Terminal 判断是否已到终态（不可再转换）。
  func (s BreachState) Terminal() bool {
	  return s == BreachStateExpiredCase || s == BreachStateCleared
  }

// BreachLease SLA 违约挡写世代租约。
// 每笔 SLA 违约（reviewsla.Ticket.Breached == true）生成一条 BreachLease，
// 关联唯一 breach_gen 令牌。只要 State == blocking，出金路径及 Decide(allow) 必须拒绝。

type BreachLease struct {
	ID string `json:"id"`
	CaseID     string `json:"case_id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	// BreachGen 唯一世代令牌（来自 reviewsla.Ticket.BreachGen）。
	// 与 review_breaches.breach_gen 唯一索引一致，防止重复建档。
	BreachGen string `json:"breach_gen"`
	Deadline time.Time `json:"deadline"`
	BreachedAt time.Time   `json:"breached_at"`
	State      BreachState `json:"state"`
	// CaseForcedExpiredAt 案件被本包 worker 强制 Expire 的时刻（nil 表示尚未执行）。
	CaseForcedExpiredAt *time.Time `json:"case_forced_expired_at,omitempty"`
	// Clearance 四眼清障关联记录（State == cleared 时填充）。
	Proposer  string     `json:"proposer,omitempty"`
	Approver  string     `json:"approver,omitempty"`
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Active 判断此 BreachLease 是否还在活跃挡写。
  func (b *BreachLease) Active() bool {
	   return b != nil && b.State.Active()
  }

// BlockOutcome 描述字符串，便于写入 SlaBreachProof.BlockOutcome。

  func (b *BreachLease) BlockOutcome() string {
	   switch b.State {
	   case BreachStateBlocking:
		  return "blocked_allow"
	  case BreachStateExpiredCase:
		  return "expired_case"
	  case BreachStateCleared:
		   return "cleared_no_allow"
	  default:
		  return "unknown"
	   }
   }

// ClearanceState 四眼清障工单状态。
type ClearanceState string

const (
	ClearanceStatePending ClearanceState = "pending"
	ClearanceStateApproved ClearanceState = "approved"
	ClearanceStateRejected ClearanceState = "rejected"
)

// BreachClearance 四眼清障工单。
// Proposer 发起，Approver（不同人）批准；批准后 BreachLease 转 cleared。
// NoAutoAllow 字段为 true 代表清障完成后出金须重走核验/案件审查路径，不自动放行。
type BreachClearance struct {
	ID            string `json:"id"`
	BreachLeaseID string `json:"breach_lease_id"`
	CaseID        string `json:"case_id"`
	MovementID string `json:"movement_id"`
	Proposer string `json:"proposer"`
	Approver string `json:"approver,omitempty"`
	Reason   string `json:"reason"`
	State      ClearanceState `json:"state"`
	ProposedAt time.Time      `json:"proposed_at"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	// NoAutoAllow 永远为 true；清障不等于 allow_lease；出金须重入核验路径。
	NoAutoAllow bool `json:"no_auto_allow"`
}

// SlaBreachProof 违约证明仓储记录（不可变）。
// 以下场景触发写入：
// 1. TryAllowCheck 拦截 Decide(allow) → BlockOutcome = "blocked_allow"；
// 2. Worker 驱动案件 Expire → BlockOutcome = "expired_case"；
// 3. 四眼清障完成 → BlockOutcome = "cleared_no_allow"。
// 同一 (BreachLeaseID, BlockOutcome) 仅写一次（幂等）。
type SlaBreachProof struct {
	ID string `json:"id"`
	CaseID        string `json:"case_id"`
	BreachLeaseID string `json:"breach_lease_id"`
	MovementID    string `json:"movement_id"`
	Deadline   time.Time `json:"deadline"`
	BreachedAt time.Time `json:"breached_at"`
	// BlockOutcome 本次证明的语义：
	// "blocked_allow" — Decide(allow) 被拦截；
	// "expired_case" — 案件被强制 Expire；
	// "cleared_no_allow" — 四眼清障完成，但未自动放行出金。
	BlockOutcome string `json:"block_outcome"`
	// Actor 触发证明落写的操作者（Decide(allow) 的尝试者，或清障操作者，或 "system"）。
	Actor string `json:"actor,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}
