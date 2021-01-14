package wrfreeze

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// LedgerFreezer 账本冻结位观察与设置接口（只读查询 + 冻结）。
// 禁止在本包内解冻 live-hash 出金锁定（须通过提现 FSM）。
type LedgerFreezer interface {
	// SetFrozen 对账本设置用户冻结位。幂等：已冻时返回 nil。
	SetFrozen(ctx context.Context, userID string, frozen bool) error
	// IsFrozen 查询用户当前账本冻结位。
	IsFrozen(ctx context.Context,
		userID string) (bool, error)
	// HasLiveHashWithdraw 查询用户是否有 live-hash 出金在途（不得自动解冻）。
	HasLiveHashWithdraw(ctx context.Context, userID string) (bool, error)
}

// WaveService 波次管理服务。

type WaveService struct {
	store   Store
	freezer LedgerFreezer
	clock   func() time.Time
	workers int
}

// NewWaveService 构造波次服务。workers 为并发冻结 goroutine 数（0 取默认 8）。

  func NewWaveService(store Store, freezer LedgerFreezer, workers int) *WaveService {
	   if workers <= 0 {
		   workers = 8
	  }
	   return &WaveService{
		  store: store, freezer: freezer, workers: workers,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (ws *WaveService) SetClock(fn func() time.Time) {
	   if ws != nil && fn != nil {
		  ws.clock = fn
	  }
   }

   func (ws *WaveService) now() time.Time {
	  if ws == nil || ws.clock == nil {
		   return time.Now().UTC()
	  }
	   return ws.clock()
  }

// AddWave 向草稿计划添加执行波次。
// 波次 SeqNo 自动从现有波次数+1 分配；不得对非草稿计划添加波次。

  func (ws *WaveService) AddWave(ctx context.Context, planID, label string, targetUsers []string, actor string) (*FreezeWave, error) {
	   if ws == nil || ws.store == nil {
		  return nil, ErrUnavailable
	  }
	   if _, err := requireActor(actor); err != nil {
		  return nil, err
	   }
	  plan, err := ws.store.GetPlan(ctx, planID)
	  if err != nil {
		   return nil, err
	   }
	  if plan.Status != PlanDraft {
		  return nil, ErrPlanState
	  }
	   users := sanitizeUserSet(targetUsers)
	   if len(users) == 0 {
		   return nil, ErrUserSet
	   }
	  existing, err := ws.store.ListWaves(ctx, planID)
	   if err != nil {
		   return nil, err
	   }
	   maxSeq := 0
	   for _, w := range existing {
		  if w.SeqNo > maxSeq {
			   maxSeq = w.SeqNo
		  }
	  }
	  now := ws.now()
	   wave := &FreezeWave{
		   ID:     uuid.NewString(),
		  PlanID: planID,
		   SeqNo: maxSeq + 1,
		  Label:       strings.TrimSpace(label),
		  TargetUsers: users,
		  Status: WavePending,
		  CreatedAt: now,
	  }
	   if err := ws.store.SaveWave(ctx, wave); err != nil {
		  return nil, err
	  }
	   return wave, nil
   }

// RunWave 执行指定波次：并发冻结所有目标用户，记录结果。
// 部分失败允许重试（最多 MaxRetry=3 次）；全部失败则标记 WaveFailed。

   func (ws *WaveService) RunWave(ctx context.Context, waveID string) (*FreezeWave, error) {
	  if ws == nil || ws.store == nil {
		  return nil, ErrUnavailable
	   }
	   wave, err := ws.store.GetWave(ctx, waveID)
	   if err != nil {
		   return nil, err
	   }
	  if wave.Status != WavePending && wave.Status != WaveFailed {
		   return nil, ErrWaveState
	  }
	  now := ws.now()
	   wave.Status = WaveRunning
	   wave.StartedAt = &now
	  wave.RetryCount++
	  if err := ws.store.SaveWave(ctx,
		  wave); err != nil {
		  return nil, err
	  }
	   users := wave.TargetUsers
	   type result struct {
		   uid    string
		   result FreezeResult
		   errMsg string
	  }
	   results := make([]result, len(users))
	   sem := make(chan struct{}, ws.workers)
	   var wg sync.WaitGroup
	   for i, uid := range users {
		   wg.Add(1)
		   go func(idx int, userID string) {
			  defer wg.Done()
			   sem <- struct{}{}
			  defer func() { <-sem }()
			  r := result{uid: userID, result: FreezeOK}
			  frozen, ferr := ws.freezer.IsFrozen(ctx, userID)
			   if ferr == nil && frozen {
				   r.result = FreezeSkipped
			   } else {
				  if ferr2 := ws.freezer.SetFrozen(ctx, userID, true); ferr2 != nil {
					   r.result = FreezeFailed
					  r.errMsg = ferr2.Error()
				  }
			   }
			   results[idx] = r
		   }(i, uid)
	   }
	   wg.Wait()
	   frozenCount, failedCount := 0, 0
	   for _, r := range results {
		   switch r.result {
		  case FreezeOK, FreezeSkipped:
			  frozenCount++
		   case FreezeFailed:
			  failedCount++
		  }
		  entry := &WaveUserEntry{
			   ID:     uuid.NewString(),
			   WaveID: waveID,
			   PlanID: wave.PlanID,
			   UserID: r.uid,
			   Result: r.result,
			   ErrMsg: r.errMsg,
			  AttemptN: wave.RetryCount,
			   At:       ws.now(),
		   }
		   _ = ws.store.SaveWaveEntry(ctx, entry)
	   }
	  finished := ws.now()
	   wave.FrozenCount = frozenCount
	  wave.FailedCount = failedCount
	   wave.FinishedAt = &finished
	   if failedCount == 0 {
		  wave.Status = WaveDone
	  } else if frozenCount == 0 {
		   wave.Status = WaveFailed
	  } else {
		  // 部分失败：允许重试（状态回 pending 供 worker 重新触发）
		   wave.Status = WavePending
	  }
	   if err := ws.store.SaveWave(ctx, wave); err != nil {
		   return nil, err
	  }
	   return wave, nil
   }

// AdvancePlan 推进计划下一个待执行波次。
// 若所有波次均 Done 则触发覆盖率巡检。
// 由 worker 循环调用。
   func (ws *WaveService) AdvancePlan(ctx context.Context, planID string) (advanced bool, err error) {
	   if ws == nil || ws.store == nil {
		  return false, ErrUnavailable
	   }
	   plan, err := ws.store.GetPlan(ctx, planID)
	   if err != nil {
		   return false, err
	   }
	  if plan.Status != PlanExecuting {
		   return false, nil
	   }
	   waves, err := ws.store.ListWaves(ctx, planID)
	  if err != nil {
		   return false, err
	   }
	   sort.Slice(waves, func(i, j int) bool { return waves[i].SeqNo < waves[j].SeqNo })
	   for _, w := range waves {
		   if w.Status == WaveRunning {
			  return false, nil // 已有波次在跑
		   }
	   }
	  for _, w := range waves {
		  if w.Status == WavePending || w.Status == WaveFailed {
			   _, rerr := ws.RunWave(ctx, w.ID)
			  return true, rerr
		  }
	   }
	  // 所有波次完成
	  return false, nil
  }

// GetWave 查询波次。

  func (ws *WaveService) GetWave(ctx context.Context, waveID string) (*FreezeWave, error) {
	   if ws == nil || ws.store == nil {
		  return nil, ErrUnavailable
	  }
	  return ws.store.GetWave(ctx, waveID)
   }

// ListWaves 列举计划波次（按 SeqNo 排序）。

   func (ws *WaveService) ListWaves(ctx context.Context, planID string) ([]*FreezeWave, error) {
	  if ws == nil || ws.store == nil {
		  return nil, ErrUnavailable
	  }
	  waves, err := ws.store.ListWaves(ctx, planID)
	  if err != nil {
		  return nil, err
	  }
	  sort.Slice(waves, func(i, j int) bool { return waves[i].SeqNo < waves[j].SeqNo })
	   return waves, nil
   }

// AllWavesDone 检查计划所有波次是否均已完成。
  func (ws *WaveService) AllWavesDone(ctx context.Context, planID string) (bool, error) {
	   if ws == nil || ws.store == nil {
		   return false, ErrUnavailable
	  }
	   waves,
		  err := ws.store.ListWaves(ctx, planID)
	   if err != nil {
		  return false, err
	  }
	  if len(waves) == 0 {
		   return false, nil
	   }
	  for _, w := range waves {
		   if !w.Status.Terminal() || w.Status == WaveFailed {
			  return false, nil
		   }
	  }
	  return true, nil
   }
