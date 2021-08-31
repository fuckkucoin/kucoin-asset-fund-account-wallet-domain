package loanrace

import (
	"context"
	"time"
	"github.com/rs/zerolog"
)

// WorkerConfig 工作线程配置。

type WorkerConfig struct {
	ScanInterval time.Duration
	DriftInterval time.Duration
	WriteoffAgeDays int
	Logger          zerolog.Logger
}

  func DefaultWorkerConfig() WorkerConfig {
	   return WorkerConfig{
		   ScanInterval:    10 * time.Second,
		   DriftInterval:   30 * time.Second,
		   WriteoffAgeDays: 90,
		  Logger: zerolog.Nop(),
	   }
  }

// OverdueChecker 检查贷款是否逾期的回调接口。

type OverdueChecker func(loanID string) bool

// ──────────────────────────────────────────────────
// 逾期扫描 Worker
// ──────────────────────────────────────────────────
// ScanOverdueWorker 扫描所有活跃贷款，对逾期的执行冻结。返回冻结数量。
  func (svc *Service) ScanOverdueWorker(ctx context.Context, checker OverdueChecker, actor string) (int, error) {
	   if svc == nil {
		  return 0, ErrUnavailable
	  }
	  if checker == nil || actor == "" {
		  return 0, ErrActor
	  }
	  loans := svc.LoansInState(LRActive)
	  frozen := 0
	   for _, loanID := range loans {
		  select {
		  case <-ctx.Done():
			  return frozen, ctx.Err()
		   default:
		  }
		   if !checker(loanID) {
			  continue
		  }
		  uid, _ := svc.store.GetUserID(loanID)
		   _, err := svc.FreezeForOverdue(loanID, uid, actor)
		  if err != nil {
			  if err == ErrLeaseConflict {
				  continue
			   }
			   continue
		  }
		   frozen++
	   }
	  return frozen, nil
  }

// ──────────────────────────────────────────────────
// 冻结漂移巡逻 Worker
// ──────────────────────────────────────────────────
// FrozenDriftPatrol 过期租约自动释放 + 确保冻结状态一致性。
   func (svc *Service) FrozenDriftPatrol(ctx context.Context, log zerolog.Logger) (int, error) {
	  if svc == nil {
		  return 0, ErrUnavailable
	  }
	   now := svc.now()
	  expired := svc.store.ExpiredLeases(now)
	   released := 0
	  for _, l := range expired {
		  select {
		  case <-ctx.Done():
			  return released, ctx.Err()
		   default:
		   }
		  svc.mu.Lock()
		   if l.State == LeaseAcquired || l.State == LeaseApplied {
			  svc.expireLeaseUnsafe(l)
			   released++
			  log.Warn().
				   Str("lease_id", l.ID).
				   Str("loan_id", l.LoanID).
				  Str("holder", l.Holder).
				   Msg("lease expired by drift patrol")
		   }
		   svc.mu.Unlock()
	  }
	   frozenLoans := svc.LoansInState(LRFrozen)
	  for _, loanID := range frozenLoans {
		  if !svc.store.IsFrozen(loanID) {
			   log.Error().
				  Str("loan_id", loanID).
				  Msg("drift: race state frozen but frozen bit not set, repairing")
			   svc.store.SetFrozen(loanID, true)
		   }
	   }
	   return released, nil
  }

// ──────────────────────────────────────────────────
// 互斥门检查 Worker
// ──────────────────────────────────────────────────
// MutexGateWorker 检查所有冻结贷款的门状态。无门的冻结贷款确保 Pay 被阻断。

   func (svc *Service) MutexGateWorker(ctx context.Context, log zerolog.Logger) (int, error) {
	   if svc == nil {
		  return 0, ErrUnavailable
	   }
	   frozenLoans := svc.LoansInState(LRFrozen)
	   checked := 0
	  for _, loanID := range frozenLoans {
		   select {
		  case <-ctx.Done():
			   return checked, ctx.Err()
		   default:
		  }
		   if _, ok := svc.store.ActiveGateForLoan(loanID); !ok {
			  log.Debug().
				   Str("loan_id", loanID).
				   Msg("frozen loan without active gate, pay blocked")
		   }
		  checked++
	   }
	  return checked, nil
   }

// ──────────────────────────────────────────────────
// 核销账龄 Worker
// ──────────────────────────────────────────────────
// WriteoffAgingWorker 检查长期冻结的贷款是否应进入核销流程。
// threshold 天数之后的冻结贷款被标记为核销候选。返回候选数量。

   func (svc *Service) WriteoffAgingWorker(ctx context.Context, thresholdDays int, log zerolog.Logger) ([]string, error) {
	   if svc == nil {
		   return nil, ErrUnavailable
	  }
	  if thresholdDays <= 0 {
		   thresholdDays = 90
	   }
	   frozenLoans := svc.LoansInState(LRFrozen)
	  candidates := make([]string, 0)
	   threshold := svc.now().AddDate(0, 0, -thresholdDays)
	  for _, loanID := range frozenLoans {
		   select {
		   case <-ctx.Done():
			  return candidates, ctx.Err()
		  default:
		  }
		   if _, ok := svc.store.WriteoffForLoan(loanID); ok {
			   continue
		   }
		  allLeases := svc.store.AllLeases()
		   var oldestFreeze time.Time
		  for _, l := range allLeases {
			   if l.LoanID == loanID && (l.State == LeaseApplied || l.State == LeaseReleased) {
				  if oldestFreeze.IsZero() || l.AcquiredAt.Before(oldestFreeze) {
					  oldestFreeze = l.AcquiredAt
				  }
			   }
		  }
		  if !oldestFreeze.IsZero() && oldestFreeze.Before(threshold) {
			  candidates = append(candidates, loanID)
			   log.Info().
				  Str("loan_id", loanID).
				   Time("oldest_freeze", oldestFreeze).
				   Int("threshold_days", thresholdDays).
				   Msg("writeoff aging candidate")
		  }
	   }
	  return candidates, nil
  }
