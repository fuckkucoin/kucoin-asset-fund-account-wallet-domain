package trexecgate

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/treasury"
)

// ExecWithLease 包含 treasury.Exec 与对应 PlanExecLease 的组合结果。
// 功能：RecordPlans 一次调用同时返回执行单和租约，供上层存储关联关系。
type ExecWithLease struct {
	Exec   treasury.Exec  `json:"exec"`
	Lease  *PlanExecLease `json:"lease"`
	PlanID string         `json:"plan_id"`
}

// LeaseAwareService 在 treasury.Service 基础上集成租约绑定与漂移封堵。
// 功能：
// 1. PlanAndRecord：完成 Plan + Record + BindLease 三步，返回 ExecWithLease 列表。
// 2. MarkSentWithGate：在 treasury.ExecBook.MarkSent 前强制走 Recheck + MarkSentGate。
// 3. AckWithLease / FailWithLease：终态绑定，同时更新租约与 Exec 状态。
// 风险点：
// LeaseAwareService 持有 treasury.Service 的指针；调用方须保证 treasury.Service 已初始化。
type LeaseAwareService struct {
	svc *treasury.Service
	eng *Engine
}

// NewLeaseAwareService 构造集成服务。

  func NewLeaseAwareService(svc *treasury.Service, eng *Engine) *LeaseAwareService {
	  if svc == nil {
		  panic("trexecgate: treasury.Service must not be nil")
	   }
	   if eng == nil {
		  panic("trexecgate: Engine must not be nil")
	  }
	   return &LeaseAwareService{svc: svc, eng: eng}
  }

// PlanAndRecord 执行完整的 Plan → Record → BindLease 三连操作。
// 功能：
// 1. 调用 treasury.Service.Plan 取当前水位计划。
// 2. 对每条 SweepPlan 调用 treasury.ExecBook.Record，取 Exec.ID。
// 3. 立即调用 Engine.BindLease 绑定热库存快照，返回 ExecWithLease。
// 风险点：
// BindLease 失败须将对应 Exec 置 failed；不得留下无租约的 planned Exec。
// Plan 为空（带内或冷却中）直接返回空列表，不视为错误。
   func (s *LeaseAwareService) PlanAndRecord(ctx context.Context, st ledger.Store) ([]ExecWithLease, error) {
	  plans, err := s.svc.Plan(ctx, st)
	   if err != nil {
		   return nil, fmt.Errorf("treasury plan: %w", err)
	  }
	  if len(plans) == 0 {
		  return nil, nil
	  }
	   out := make([]ExecWithLease, 0, len(plans))
	  for _, plan := range plans {
		  // 调用已有 treasury.ExecBook.Record，不修改其逻辑。
		   exec, recErr := s.svc.Exec().Record(plan)
		   if recErr != nil {
			  return out, fmt.Errorf("treasury record plan %s: %w", plan.Asset, recErr)
		   }
		   // 用 asset+ExecID 作为 PlanID（确保每次 Record 都有独立幂等键，非幂等复用场景）。
		   planID := buildPlanID(plan.Asset, exec.ID)
		   bindRes, bindErr := s.eng.BindLease(ctx, st, BindInput{
			   PlanID: planID,
			   ExecID: exec.ID,
			  Asset: plan.Asset,
			  Amount: plan.Amount,
		  })
		  if bindErr != nil {
			   // BindLease 失败：将 Exec 标记为 failed，防止残留 planned 状态。
			   _, _ = s.svc.Exec().MarkFailed(exec.ID, fmt.Sprintf("lease bind failed: %v", bindErr), exec.Fence.Epoch, exec.Fence.Token)
			   return out, fmt.Errorf("treasury bind lease for exec %s: %w", exec.ID, bindErr)
		   }
		   out = append(out, ExecWithLease{
			  Exec: exec,
			   Lease:  bindRes.Lease,
			   PlanID: planID,
		  })
	   }
	  return out, nil
   }

// MarkSentWithGate 在放行 MarkSent 前执行 Recheck + MarkSentGate 双重校验。
// 功能：
// 1. Engine.Recheck：重新 Observe 热账，漂移超限则落 TreasDriftProof 并返回拒绝。
// 2. Engine.MarkSentGate：校验租约有效性（未过期、已复核、LeaseID 匹配）。
// 3. 通过后调用 treasury.ExecBook.MarkSent，完成出金状态推进。
// 风险点：
// Recheck 通过但 ExecBook.MarkSent 失败（围栏过期等），须将租约回退到 rechecked，
// 等待下次复核或 Worker 巡检兜底。
   func (s *LeaseAwareService) MarkSentWithGate(
	  ctx context.Context,
	   st ledger.Store,
	  execID, leaseID, hash string,
	   epoch int64,
	  token string,
  ) (treasury.Exec, *PlanExecLease, error) {
	  if strings.TrimSpace(leaseID) == "" {
		  return treasury.Exec{}, nil, fmt.Errorf("%w: lease_id required for MarkSent gate", ErrInvalidInput)
	  }
	  // 第一关：热账漂移复核。
	   rr, recheckErr := s.eng.Recheck(ctx, st, leaseID)
	  if recheckErr != nil {
		   return treasury.Exec{}, rr.Lease, fmt.Errorf("recheck before mark-sent: %w", recheckErr)
	   }
	   // 第二关：MarkSent 闸——校验租约未过期、已复核、LeaseID 与 ExecID 绑定一致。
	  sentLease, gateErr := s.eng.MarkSentGate(ctx, MarkSentGateInput{
		  LeaseID: leaseID,
		   ExecID:  execID,
	   })
	   if gateErr != nil {
		  return treasury.Exec{}, sentLease, fmt.Errorf("mark-sent gate: %w", gateErr)
	   }
	   // 两关通过，放行 ExecBook.MarkSent。
	   exec,
		  msErr := s.svc.Exec().MarkSent(execID, hash, epoch, token)
	  if msErr != nil {
		  // ExecBook.MarkSent 失败：租约已在 sent 状态，无法直接回退；
		   // 记录错误，由 Worker 后续巡检处理。
		   return exec, sentLease, fmt.Errorf("treasury mark-sent for exec %s: %w", execID, msErr)
	   }
	  return exec, sentLease, nil
   }

// AckWithLease 链上回执确认后，同时更新 Exec 和租约终态。
// 功能：ExecBook.MarkAcked + Engine.AckLease 两步原子语义（尽力而为）。
// 风险点：两步均须成功；ExecBook 失败租约仍会被标记 acked，须告警。

   func (s *LeaseAwareService) AckWithLease(
	   ctx context.Context,
	   execID, leaseID, hash string,
	  epoch int64,
	   token string,
   ) (treasury.Exec, *PlanExecLease, error) {
	  exec, err := s.svc.Exec().MarkAcked(execID, hash, epoch, token)
	  if err != nil {
		   return exec, nil, fmt.Errorf("treasury mark-acked: %w", err)
	   }
	   lease, leaseErr := s.eng.AckLease(ctx, leaseID, execID)
	   if leaseErr != nil {
		   // Exec 已 acked，租约更新失败不阻塞；须告警排查。
		   return exec, nil, fmt.Errorf("ack lease %s (exec already acked): %w", leaseID, leaseErr)
	  }
	  return exec, lease, nil
   }

// FailWithLease 执行失败后，同时将 Exec 和租约置终态。

  func (s *LeaseAwareService) FailWithLease(
	  ctx context.Context,
	  execID, leaseID, reason string,
	   epoch int64,
	  token string,
  ) (treasury.Exec, *PlanExecLease, error) {
	  exec,
		   err := s.svc.Exec().MarkFailed(execID, reason, epoch, token)
	  if err != nil {
		   return exec, nil, fmt.Errorf("treasury mark-failed: %w", err)
	  }
	   lease, leaseErr := s.eng.FailLease(ctx, leaseID, execID, reason)
	  if leaseErr != nil {
		  return exec, nil, fmt.Errorf("fail lease %s (exec already failed): %w", leaseID, leaseErr)
	   }
	  return exec, lease, nil
  }

// Underlying 返回底层 treasury.Service，供只读路径（如查询 Open execs）使用。

   func (s *LeaseAwareService) Underlying() *treasury.Service {
	   return s.svc
  }

// Engine 返回底层 Engine，供只读路径查询租约详情使用。

   func (s *LeaseAwareService) Engine() *Engine {
	  return s.eng
   }

// buildPlanID 从资产符号与 ExecID 构造计划 ID，用作 BindLease 的 PlanID。
// 功能：保证每次 Record 产生的 PlanID 全局唯一，且不依赖外部传入。

   func buildPlanID(asset, execID string) string {
	   return strings.ToUpper(strings.TrimSpace(asset)) + ":" + strings.TrimSpace(execID)
   }
