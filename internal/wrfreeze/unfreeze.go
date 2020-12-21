package wrfreeze

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// UnfreezeService 分阶段解冻编排服务。
type UnfreezeService struct {
	store Store
	freezer LedgerFreezer
	clock   func() time.Time
}

// NewUnfreezeService 构造解冻服务。
  func NewUnfreezeService(store Store, freezer LedgerFreezer) *UnfreezeService {
	   return &UnfreezeService{
		  store: store, freezer: freezer,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (us *UnfreezeService) SetClock(fn func() time.Time) {
	   if us != nil && fn != nil {
		  us.clock = fn
	  }
   }

  func (us *UnfreezeService) now() time.Time {
	   if us == nil || us.clock == nil {
		  return time.Now().UTC()
	   }
	   return us.clock()
   }

// InitPhase 发起解冻阶段。
// 约束：
// - probe 可在 plan 任意非终态下发起（验证覆盖完整性）。
// - partial/full 须 probe 已 passed。
// - 同 plan 不得同时有两个相同 kind 的 running 阶段。

   func (us *UnfreezeService) InitPhase(ctx context.Context, req InitPhaseReq) (*UnfreezePhase, error) {
	   if us == nil || us.store == nil {
		  return nil, ErrUnavailable
	  }
	   actor, err := requireActor(req.Actor)
	   if err != nil {
		  return nil, err
	  }
	  kind, err := validatePhaseKind(req.Kind)
	  if err != nil {
		   return nil, ErrPhaseState
	   }
	  plan, err := us.store.GetPlan(ctx, req.PlanID)
	  if err != nil {
		  return nil, err
	  }
	  if plan.Status.Terminal() {
		  return nil, ErrPlanState
	   }
	  // partial/full 须 probe 已通过
	   if kind == PhasePartial || kind == PhaseFull {
		   passed, perr := us.probeHasPassed(ctx, req.PlanID)
		  if perr != nil {
			   return nil, perr
		  }
		   if !passed {
			   return nil, ErrProbeNotPassed
		  }
	   }
	   // 不允许同 plan 同 kind 有运行中阶段
	  existing, err := us.store.ListPhases(ctx, req.PlanID)
	   if err != nil {
		   return nil, err
	   }
	   for _, p := range existing {
		   if p.Kind == kind && p.Status == PhaseRunning {
			  return nil, ErrPhaseState
		  }
	  }
	   now := us.now()
	   phase := &UnfreezePhase{
		  ID:     uuid.NewString(),
		   PlanID: req.PlanID,
		   Kind:   kind,
		   Status: PhasePending,
		   InitiatedBy: actor,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := us.store.SavePhase(ctx, phase); err != nil {
		   return nil, err
	  }
	   return phase, nil
   }

// ApprovePhase 批准解冻阶段（四眼）并推进到 running。
// 批准人不得是发起人。

  func (us *UnfreezeService) ApprovePhase(ctx context.Context, req ApprovePhaseReq) (*UnfreezePhase, error) {
	  if us == nil || us.store == nil {
		  return nil, ErrUnavailable
	   }
	  actor, err := requireActor(req.Actor)
	  if err != nil {
		  return nil, err
	   }
	  phase, err := us.store.GetPhase(ctx, req.PhaseID)
	   if err != nil {
		  return nil, err
	  }
	   if phase.Status != PhasePending {
		  return nil, ErrPhaseState
	   }
	   if strings.EqualFold(actor, phase.InitiatedBy) {
		  return nil, ErrFourEye
	  }
	   now := us.now()
	   phase.ApprovedBy = actor
	  phase.Status = PhaseRunning
	   phase.StartedAt = &now
	   phase.UpdatedAt = now
	   if err := us.store.SavePhase(ctx, phase); err != nil {
		   return nil, err
	   }
	  // 立即执行阶段逻辑
	  return us.executePhase(ctx, phase)
   }

// executePhase 执行具体阶段。

  func (us *UnfreezeService) executePhase(ctx context.Context, phase *UnfreezePhase) (*UnfreezePhase, error) {
	   plan, err := us.store.GetPlan(ctx, phase.PlanID)
	   if err != nil {
		  return phase, err
	   }
	  users,
		  err := us.collectPlanUsers(ctx, plan)
	  if err != nil {
		   return phase, err
	   }
	  switch phase.Kind {
	  case PhaseProbe:
		  return us.runProbe(ctx, phase, plan, users)
	  case PhasePartial:
		  return us.runPartial(ctx, phase, users)
	  case PhaseFull:
		   return us.runFull(ctx, phase, users)
	  default:
		  phase.Status = PhaseFailed
		   phase.IntegrityNote = "unknown phase kind"
		   _ = us.store.SavePhase(ctx,
			  phase)
		  return phase, ErrPhaseState
	  }
   }

// runProbe 探针阶段：校验所有目标用户均已冻结（不执行解冻）。

  func (us *UnfreezeService) runProbe(ctx context.Context, phase *UnfreezePhase, plan *CascadePlan, users []string) (*UnfreezePhase, error) {
	  notFrozen := 0
	   notFrozenUsers := make([]string, 0)
	   for _, uid := range users {
		  frozen, err := us.freezer.IsFrozen(ctx, uid)
		  if err != nil || !frozen {
			  notFrozen++
			   notFrozenUsers = append(notFrozenUsers, uid)
		   }
	  }
	   now := us.now()
	  phase.FinishedAt = &now
	   phase.UpdatedAt = now
	  if notFrozen == 0 {
		  phase.Status = PhasePassed
		   phase.IntegrityNote = "all target users confirmed frozen"
		  phase.UnfrozenCount = 0
	   } else {
		   phase.Status = PhaseFailed
		  phase.IntegrityNote = "probe detected " + itoa(notFrozen) + " unfrozen user(s): " + strings.Join(notFrozenUsers, ",")
	   }
	   if err := us.store.SavePhase(ctx, phase); err != nil {
		  return phase, err
	   }
	  return phase, nil
  }

// runPartial 部分解冻：仅解冻前 50% 用户（低风险子集）。
// 不解冻有 live-hash 出金的用户。
   func (us *UnfreezeService) runPartial(ctx context.Context, phase *UnfreezePhase, users []string) (*UnfreezePhase, error) {
	  cutoff := len(users) / 2
	   if cutoff == 0 && len(users) > 0 {
		   cutoff = 1
	   }
	   subset := users[:cutoff]
	   unfrozen, failed := 0, 0
	  for _, uid := range subset {
		  if skip, _ := us.freezer.HasLiveHashWithdraw(ctx, uid); skip {
			  // live-hash 出金存在：跳过，不计入失败
			   continue
		   }
		   if err := us.freezer.SetFrozen(ctx, uid, false); err != nil {
			   failed++
		   } else {
			   unfrozen++
		  }
	   }
	  return us.finishPhase(ctx, phase, unfrozen, failed, "partial unfreeze: "+itoa(unfrozen)+" unfrozen, "+itoa(failed)+" failed")
   }

// runFull 全量解冻：解冻所有目标用户。
// live-hash 出金用户跳过（不自动解冻）。
   func (us *UnfreezeService) runFull(ctx context.Context, phase *UnfreezePhase, users []string) (*UnfreezePhase, error) {
	   unfrozen, failed, skippedLiveHash := 0, 0, 0
	   for _, uid := range users {
		   if skip, _ := us.freezer.HasLiveHashWithdraw(ctx, uid); skip {
			   skippedLiveHash++
			   continue
		  }
		  if err := us.freezer.SetFrozen(ctx, uid, false); err != nil {
			  failed++
		   } else {
			   unfrozen++
		  }
	  }
	  note := "full unfreeze: " + itoa(unfrozen) + " unfrozen, " + itoa(failed) + " failed, " + itoa(skippedLiveHash) + " skipped(live-hash)"
	   return us.finishPhase(ctx, phase, unfrozen, failed, note)
   }

  func (us *UnfreezeService) finishPhase(ctx context.Context, phase *UnfreezePhase, unfrozen, failed int, note string) (*UnfreezePhase, error) {
	   now := us.now()
	   phase.UnfrozenCount = unfrozen
	   phase.FailedCount = failed
	  phase.IntegrityNote = note
	   phase.FinishedAt = &now
	   phase.UpdatedAt = now
	   if failed == 0 {
		  phase.Status = PhasePassed
	  } else {
		  phase.Status = PhaseFailed
	   }
	  if err := us.store.SavePhase(ctx, phase); err != nil {
		   return phase, err
	  }
	  return phase, nil
  }

// probeHasPassed 检查 plan 下是否有 passed 的 probe 阶段。
   func (us *UnfreezeService) probeHasPassed(ctx context.Context, planID string) (bool, error) {
	  phases, err := us.store.ListPhases(ctx, planID)
	  if err != nil {
		  return false, err
	   }
	   for _, p := range phases {
		  if p.Kind == PhaseProbe && p.Status == PhasePassed {
			  return true, nil
		  }
	   }
	  return false, nil
  }

// collectPlanUsers 收集计划目标用户集合（按 scope 展开）。

  func (us *UnfreezeService) collectPlanUsers(ctx context.Context, plan *CascadePlan) ([]string, error) {
	   switch plan.Scope {
	  case ScopeUserSet:
		  return plan.UserSet, nil
	   case ScopeGlobal, ScopeAsset, ScopeCohort:
		   // 从 wave TargetUsers 合并（通用实现：实际部署时可从 store 展开 cohort）
		  waves, err := us.store.ListWaves(ctx, plan.ID)
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
	   default:
		   return nil, ErrScope
	  }
   }

// GetPhase 查询解冻阶段。

   func (us *UnfreezeService) GetPhase(ctx context.Context, phaseID string) (*UnfreezePhase, error) {
	   if us == nil || us.store == nil {
		   return nil, ErrUnavailable
	  }
	  return us.store.GetPhase(ctx, phaseID)
  }

// ListPhases 列举计划下的解冻阶段。
   func (us *UnfreezeService) ListPhases(ctx context.Context, planID string) ([]*UnfreezePhase, error) {
	  if us == nil || us.store == nil {
		  return nil, ErrUnavailable
	   }
	   return us.store.ListPhases(ctx, planID)
  }

   func validatePhaseKind(k PhaseKind) (PhaseKind, error) {
	   switch k {
	  case PhaseProbe, PhasePartial, PhaseFull:
		   return k, nil
	  default:
		   return "", ErrPhaseState
	   }
  }

  func itoa(n int) string {
	  if n == 0 {
		   return "0"
	   }
	  neg := false
	  if n < 0 {
		  neg = true
		   n = -n
	   }
	   var b [20]byte
	  i := len(b)
	   for n > 0 {
		  i--
		   b[i] = byte('0' + n%10)
		   n /= 10
	  }
	  if neg {
		  i--
		   b[i] = '-'
	  }
	  return string(b[i:])
   }
