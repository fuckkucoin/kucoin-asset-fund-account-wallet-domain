package dustguard

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// FloodParams 粉尘洪水注入参数。
type FloodParams struct {
	Chain   string
	Address string
	FromAddr string
	Asset  string
	Amount string // 低额，如 "0.00001"
	Count int // 注入条数，应超过策略 FloodThreshold
}

// InjectDustFlood 向检测器注入 N 条粉尘信号，洪水攻击。
// 返回最后一次分值；调用方可检查 Score.Level == LevelCritical。
// 若 params.Count 超过策略 FloodThreshold，分值应达到攻击等级。
   func InjectDustFlood(ctx context.Context, svc *GuardService, p FloodParams) (AttackScore, error) {
	   if svc == nil {
		   return AttackScore{}, fmt.Errorf("guard service nil")
	   }
	   p.Chain = strings.ToUpper(strings.TrimSpace(p.Chain))
	   p.Address = strings.TrimSpace(p.Address)
	  if p.Count <= 0 {
		   p.Count = 60 // 默认超出默认阈值 50
	   }
	  if p.Amount == "" {
		   p.Amount = "0.000001"
	  }
	   if p.FromAddr == "" {
		   return AttackScore{}, fmt.Errorf("from addr required")
	  }
	  var last AttackScore
	  for i := 0; i < p.Count; i++ {
		  sig := DustSignal{
			  Chain:   p.Chain,
			  Address: p.Address,
			   FromAddr: p.FromAddr,
			  Asset:  p.Asset,
			   Amount: p.Amount,
			   TxHash: fmt.Sprintf("flood-%04d-%s", i, uuid.NewString()),
			   SignalKind: SignalFlood,
			   SeenAt:     time.Now().UTC(),
		  }
		   score, err := svc.detector.Record(ctx, sig)
		   if err != nil {
			   return AttackScore{}, fmt.Errorf("record flood signal %d: %w", i, err)
		   }
		   last = score
	  }
	  return last, nil
  }

// SurfaceParams 共享面刷尘注入参数。

type SurfaceParams struct {
	Chain      string
	SharedAddr string
	FromAddrs []string // 不同 from，多来源刷尘
	Asset     string
	Amount    string
}

// InjectSharedSurface 向共享 FamMEMO 地址注入刷尘信号，从多个 from-addr 发起。
// 返回最后一次分值；surface_hit 应为 true。
  func InjectSharedSurface(ctx context.Context, svc *GuardService, p SurfaceParams) (AttackScore, error) {
	   if svc == nil {
		  return AttackScore{}, fmt.Errorf("guard service nil")
	  }
	  p.Chain = strings.ToUpper(strings.TrimSpace(p.Chain))
	  if p.Amount == "" {
		   p.Amount = "0.000001"
	  }
	   if len(p.FromAddrs) == 0 {
		  return AttackScore{}, fmt.Errorf("from addrs required")
	  }
	  var last AttackScore
	  // 每个 from 发 5 笔，制造 surface + cluster 双命中
	  for _, from := range p.FromAddrs {
		  for j := 0; j < 5; j++ {
			  sig := DustSignal{
				   Chain:    p.Chain,
				   Address:  p.SharedAddr,
				  FromAddr: from,
				   Asset:  p.Asset,
				   Amount: p.Amount,
				  TxHash: fmt.Sprintf("surface-%s-%04d", uuid.NewString(), j),
				  SignalKind: SignalSurface,
				   SeenAt:     time.Now().UTC(),
			   }
			   score,
				  err := svc.detector.Record(ctx, sig)
			   if err != nil {
				  return AttackScore{}, fmt.Errorf("surface signal: %w", err)
			   }
			   last = score
		   }
	   }
	  return last, nil
  }

// FalseQuarantineResult 误隔离回滚夹具结果。
type FalseQuarantineResult struct {
	CaseID string
	ReleasedCase *QuarantineCase
	ReallocatableOK bool
}

// ScenarioFalseQuarantine 误隔离后回滚全流程：
// 1. 手动开案（无需异常流量）
// 2. Advance → holding
// 3. Release（取证窗口内失败，夹具强制推过时间后重试）
// 4. 检查 IsBlocked → false（可再分配）
// forensicsBypassed: 夹具通过构造 ForensicsUntil 已过时间来绕过窗口检查。
// 返回释放后的案件以及 reallocatable 状态。
  func ScenarioFalseQuarantine(ctx context.Context, svc *GuardService, chain, address, userID, actor string) (*FalseQuarantineResult, error) {
	  if svc == nil {
		   return nil, fmt.Errorf("guard service nil")
	   }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  // 开案（空分值）
	   emptyScore := AttackScore{Chain: chain,
		   Address: address}
	   cs, err := svc.fsm.Open(ctx, chain, address, userID, actor, emptyScore, "fixture: false quarantine")
	  if err != nil && err != ErrAlreadyQuarantined {
		  return nil, fmt.Errorf("open case: %w", err)
	  }
	   // 取案件 ID
	  caseID := ""
	  if cs != nil {
		   caseID = cs.ID
	  } else {
		   active,
			  ok := svc.fsm.ActiveCase(ctx, chain, address)
		   if !ok {
			   return nil, fmt.Errorf("no active case after open")
		   }
		  caseID = active.ID
	   }
	  // Advance → holding
	  if _, err := svc.fsm.Advance(ctx, caseID, actor); err != nil {
		  return nil, fmt.Errorf("advance: %w", err)
	   }
	  // 取证窗口内 Release 必须失败（fail-closed 验证）
	  _, errInWindow := svc.fsm.Release(ctx, caseID, actor)
	  if errInWindow == nil {
		  return nil, fmt.Errorf("fixture: release inside forensics window should fail, but succeeded")
	   }
	  // 夹具直接修改 ForensicsUntil（内存存储下可行；SQL 环境需用 UpdateCase）
	   // 把取证截止时间改到过去
	  c, err := svc.fsm.store.GetCase(ctx, caseID)
	  if err != nil {
		   return nil, fmt.Errorf("get case for forensics bypass: %w", err)
	  }
	   c.ForensicsUntil = time.Now().UTC().Add(-time.Second)
	   c.UpdatedAt = time.Now().UTC()
	  if err := svc.fsm.store.UpdateCase(ctx, c); err != nil {
		  return nil, fmt.Errorf("bypass forensics window: %w", err)
	   }
	  // 再次 Release，现在应该成功
	   released, err := svc.fsm.Release(ctx, caseID, actor)
	  if err != nil {
		   return nil, fmt.Errorf("release after window: %w", err)
	  }
	   // 确认不再被隔离
	  blocked := svc.fsm.IsBlocked(ctx,
		  chain, address)
	  return &FalseQuarantineResult{
		  CaseID: caseID,
		  ReleasedCase:    released,
		   ReallocatableOK: !blocked,
	   }, nil
   }

// ScenarioFamMEMORejectUserRotate 验证用户侧轮换 FamMEMO 地址被拒。
// 对 sharedAddr（已在 shared pool 注册）调用 RejectUserSelfRotate，应返回 ErrUserRotateRejected。
// 若 sharedAddr 不是共享地址，则不触发拒绝（走正常 1:1 路径）。
   func ScenarioFamMEMORejectUserRotate(svc *GuardService, sharedAddr string) error {
	  if svc == nil {
		   return fmt.Errorf("guard service nil")
	   }
	  if svc.rotator == nil {
		   return fmt.Errorf("rotator nil")
	  }
	  err := svc.rotator.RejectUserSelfRotate(sharedAddr)
	  if err == nil {
		  return fmt.Errorf("fixture: expected ErrUserRotateRejected for shared addr %s but got nil", sharedAddr)
	   }
	   if err != ErrUserRotateRejected {
		   return fmt.Errorf("fixture: expected ErrUserRotateRejected, got %w", err)
	   }
	  return nil
   }

// ScenarioContaminatedObserve 污染地址上的观察，验证防护单开启且贷记被拦截。
// 前提：address 必须已处于活跃隔离（调用前先 InjectDustFlood 或手动 Open case）。

   func ScenarioContaminatedObserve(ctx context.Context, svc *GuardService, p ObserveParams) (*ObserveGuardResult, error) {
	   if svc == nil {
		  return nil, fmt.Errorf("guard service nil")
	  }
	   result,
		  err := svc.observeHook.Check(ctx, p)
	  if err == nil {
		   return nil, fmt.Errorf("fixture: expected ErrObserveBlocked for contaminated address but credit was allowed")
	  }
	  if !result.Blocked {
		   return nil, fmt.Errorf("fixture: result.Blocked=false but error returned; inconsistent state")
	   }
	  return result, nil
   }
