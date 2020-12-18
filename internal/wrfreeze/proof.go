package wrfreeze

import (
	"context"
	"time"
	"github.com/google/uuid"
)

// ProofService 覆盖率证明服务。

type ProofService struct {
	store Store
	freezer LedgerFreezer
	clock   func() time.Time
}

// NewProofService 构造证明服务。

  func NewProofService(store Store, freezer LedgerFreezer) *ProofService {
	   return &ProofService{
		  store: store, freezer: freezer,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

  func (ps *ProofService) SetClock(fn func() time.Time) {
	   if ps != nil && fn != nil {
		  ps.clock = fn
	  }
  }

  func (ps *ProofService) now() time.Time {
	   if ps == nil || ps.clock == nil {
		   return time.Now().UTC()
	  }
	  return ps.clock()
   }

// ComputeProof 计算计划覆盖率证明。
// 每次调用生成新 proof 快照；历史 proof 保留供审计。
// 返回 VerdictPassed 时 delta 为空；VerdictGap 时 delta 含缺失用户。

  func (ps *ProofService) ComputeProof(ctx context.Context, planID string) (*CoverageProof, error) {
	  if ps == nil || ps.store == nil {
		   return nil, ErrUnavailable
	   }
	   plan, err := ps.store.GetPlan(ctx, planID)
	  if err != nil {
		   return nil, err
	  }
	   // 构建应冻集合
	  expected, err := ps.buildExpectedSet(ctx, plan)
	  if err != nil {
		  return nil, err
	  }
	  // 查询实冻集合（账本冻结位）
	   delta := make([]string, 0)
	   actualCount := 0
	  for _, uid := range expected {
		  frozen, ferr := ps.freezer.IsFrozen(ctx, uid)
		   if ferr != nil {
			   delta = append(delta, uid)
			  continue
		   }
		  if frozen {
			  actualCount++
		   } else {
			  delta = append(delta, uid)
		  }
	  }
	  verdict := VerdictPassed
	  if len(delta) > 0 {
		   verdict = VerdictGap
	  }
	  now := ps.now()
	  proof := &CoverageProof{
		  ID:     uuid.NewString(),
		  PlanID: planID,
		  ExpectedCount: len(expected),
		  ActualCount: actualCount,
		   DeltaCount:  len(delta),
		  DeltaUsers:  delta,
		   Verdict:     verdict,
		   CheckedAt:   now,
		  CreatedAt: now,
	   }
	  if err := ps.store.SaveProof(ctx, proof); err != nil {
		  return nil, err
	  }
	  return proof, nil
  }

// LatestProof 获取计划最新证明（用于封存判断）。
  func (ps *ProofService) LatestProof(ctx context.Context, planID string) (*CoverageProof, error) {
	  if ps == nil || ps.store == nil {
		   return nil, ErrUnavailable
	   }
	   return ps.store.LatestProof(ctx, planID)
   }

// ListProofs 列举历史证明（供审计）。

  func (ps *ProofService) ListProofs(ctx context.Context, planID string) ([]*CoverageProof, error) {
	   if ps == nil || ps.store == nil {
		  return nil, ErrUnavailable
	  }
	   return ps.store.ListProofs(ctx,
		   planID)
   }

// buildExpectedSet 从 plan 构建应冻用户集合。
// ScopeUserSet：直接使用 UserSet。
// 其余 scope：从 FreezeWave.TargetUsers 合并（wave 是计划的执行单元，最精确）。
  func (ps *ProofService) buildExpectedSet(ctx context.Context, plan *CascadePlan) ([]string, error) {
	  if plan.Scope == ScopeUserSet && len(plan.UserSet) > 0 {
		   return plan.UserSet, nil
	  }
	  waves, err := ps.store.ListWaves(ctx, plan.ID)
	  if err != nil {
		   return nil, err
	   }
	   seen := map[string]struct{}{}
	  out := make([]string, 0)
	  for _, w := range waves {
		  for _, u := range w.TargetUsers {
			   if _, ok := seen[u]; !ok {
				  seen[u] = struct{}{}
				  out = append(out, u)
			   }
		  }
	  }
	  return out, nil
  }

// PatrolAndSeal 巡检：若所有波次完成且覆盖率 100%，自动尝试封存计划。
// 由 worker 定期调用；封存失败时记录但不终止巡检循环。
  func (ps *ProofService) PatrolAndSeal(ctx context.Context, planSvc *PlanService, waveSvc *WaveService) error {
	   if ps == nil || ps.store == nil {
		   return ErrUnavailable
	   }
	  plans, err := ps.store.ListPlans(ctx, string(PlanExecuting), 100)
	   if err != nil {
		  return err
	  }
	   for _, plan := range plans {
		  done, err := waveSvc.AllWavesDone(ctx, plan.ID)
		  if err != nil || !done {
			  continue
		  }
		  proof, err := ps.ComputeProof(ctx, plan.ID)
		   if err != nil {
			  continue
		   }
		  if proof.Verdict == VerdictPassed {
			   _, _ = planSvc.SealPlan(ctx, plan.ID, "system")
		   }
	  }
	   return nil
  }

// ProofSummary 证明摘要（用于 HTTP 响应）。
type ProofSummary struct {
	PlanID string `json:"plan_id"`
	ExpectedCount int `json:"expected_count"`
	ActualCount   int `json:"actual_count"`
	DeltaCount int          `json:"delta_count"`
	DeltaUsers []string     `json:"delta_users,omitempty"`
	Verdict    ProofVerdict `json:"verdict"`
	CheckedAt  time.Time    `json:"checked_at"`
}

// ToSummary 将 proof 转换为摘要。

  func (p *CoverageProof) ToSummary() ProofSummary {
	   if p == nil {
		  return ProofSummary{}
	   }
	   return ProofSummary{
		   PlanID: p.PlanID,
		   ExpectedCount: p.ExpectedCount,
		  ActualCount:   p.ActualCount,
		  DeltaCount:    p.DeltaCount,
		  DeltaUsers: p.DeltaUsers,
		   Verdict:   p.Verdict,
		  CheckedAt: p.CheckedAt,
	  }
  }
