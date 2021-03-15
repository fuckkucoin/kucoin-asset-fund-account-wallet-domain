package breachclosegate

import (
	"errors"
	"time"
	"github.com/google/uuid"
)

// Gate 提供 Decide(allow) 与 Submit 预检。

type Gate struct {
	store Store
	clock func() time.Time
}

// NewGate 创建 Gate。

   func NewGate(store Store) *Gate {
	  return &Gate{store: store, clock: func() time.Time { return time.Now().UTC() }}
  }

// WithClock 注入时钟。

  func (g *Gate) WithClock(c func() time.Time) *Gate {
	  if g != nil && c != nil {
		  g.clock = c
	   }
	  return g
   }

  // TryAllowCheckResult TryAllowCheck 结果。

  type TryAllowCheckResult struct {
	   // Allowed true 表示无活跃 BreachLease，放行。
	  Allowed bool
	  // Lease 存在时是被拦截的 BreachLease。
	   Lease *BreachLease
	   // Proof 本次拦截写入的 SlaBreachProof（Allowed==false 时有效）。
	   Proof *SlaBreachProof
  }

   // TryAllowCheck 检查 caseID 是否有活跃 BreachLease。
   // actor 是尝试 Decide(allow) 的操作者，用于证明记录。
  // 返回 ErrBreachBlock 表示拦截，调用者须终止 Decide(allow)。
  // 返回 nil 表示无活跃违约，允许接着。

   func (g *Gate) TryAllowCheck(caseID, actor string) (TryAllowCheckResult, error) {
	   lease, err := g.store.GetLeaseByCase(caseID)
	   if err != nil {
		  if errors.Is(err, ErrNoActiveBreach) {
			  return TryAllowCheckResult{Allowed: true}, nil
		   }
		  // Store 查询失败：fail-closed，拒绝放行。
		   return TryAllowCheckResult{Allowed: false}, ErrBreachBlock
	   }
	  if !lease.Active() {
		   return TryAllowCheckResult{Allowed: true}, nil
	   }
	   // 存在活跃 BreachLease，拦截并落 SlaBreachProof。
	  proof := g.writeBlockedAllowProof(lease,
		   actor)
	   return TryAllowCheckResult{
		   Allowed: false,
		   Lease: lease,
		   Proof: proof,
	  }, ErrBreachBlock
  }

// SubmitPrecheck 检查 movementID 是否有活跃 BreachLease。
// 用于出金 Submit/广播路径预检，fail-closed：有活跃违约则拒绝。
// 返回 ErrBreachBlock 表示拦截；nil 表示通过。
  func (g *Gate) SubmitPrecheck(movementID, actor string) (TryAllowCheckResult, error) {
	  lease, err := g.store.GetLeaseByMovement(movementID)
	  if err != nil {
		   if errors.Is(err, ErrNoActiveBreach) {
			   return TryAllowCheckResult{Allowed: true}, nil
		   }
		   return TryAllowCheckResult{Allowed: false}, ErrBreachBlock
	   }
	  if !lease.Active() {
		   return TryAllowCheckResult{Allowed: true}, nil
	   }
	  proof := g.writeBlockedAllowProof(lease,
		  actor)
	   return TryAllowCheckResult{
		  Allowed: false,
		   Lease: lease,
		   Proof: proof,
	  }, ErrBreachBlock
   }

// writeBlockedAllowProof 内部：为 Decide(allow) 被拦截事件写 SlaBreachProof。
   func (g *Gate) writeBlockedAllowProof(lease *BreachLease, actor string) *SlaBreachProof {
	  proof := &SlaBreachProof{
		   ID:     uuid.NewString(),
		   CaseID: lease.CaseID,
		   BreachLeaseID: lease.ID,
		   MovementID:    lease.MovementID,
		  Deadline:      lease.Deadline,
		  BreachedAt:    lease.BreachedAt,
		   BlockOutcome: "blocked_allow",
		  Actor:        actor,
		   RecordedAt:   g.clock(),
	   }
	  // 证明写入失败不阻断拦截语义（fail-closed 已由返回 ErrBreachBlock 保证）。
	   _ = g.store.CreateProof(*proof)
	  return proof
   }
