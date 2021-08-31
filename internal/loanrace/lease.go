package loanrace

import (
	"time"
	"github.com/google/uuid"
)

// AcquireLease 为指定贷款获取逾期冻结租约。同一贷款同一时间只能有一个活跃租约。
// holder 通常为 "overdue_scanner" 或 "pay_worker"。

  func (svc *Service) AcquireLease(loanID, userID, holder string) (OverdueLease, error) {
	   if svc == nil {
		   return OverdueLease{}, ErrUnavailable
	   }
	   if loanID == "" || holder == "" {
		   return OverdueLease{}, ErrActor
	  }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   if _, ok := svc.store.GetRaceState(loanID); !ok {
		   return OverdueLease{}, ErrLoanNotFound
	   }
	  if existing, ok := svc.store.ActiveLeaseForLoan(loanID); ok {
		  now := svc.now()
		   if now.Before(existing.ExpiresAt) {
			  return OverdueLease{}, ErrLeaseConflict
		  }
		   svc.expireLeaseUnsafe(existing)
	  }
	  now := svc.now()
	  gen := svc.nextGen(loanID)
	  lease := &OverdueLease{
		   ID: uuid.NewString(),
		   LoanID:     loanID,
		   UserID:     userID,
		   Generation: gen,
		  State:  LeaseAcquired,
		   Holder: holder,
		   AcquiredAt: now,
		   ExpiresAt: now.Add(svc.leaseTTL),
	   }
	  svc.store.PutLease(lease)
	   return lease.Copy(), nil
   }

// ApplyLease 将已获取的租约标记为已应用（冻结已执行）。

   func (svc *Service) ApplyLease(leaseID string) (OverdueLease, error) {
	   if svc == nil {
		   return OverdueLease{}, ErrUnavailable
	  }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	  l, ok := svc.store.GetLease(leaseID)
	  if !ok {
		   return OverdueLease{}, ErrLeaseNotFound
	   }
	   if l.State != LeaseAcquired {
		  return l.Copy(), ErrState
	   }
	   now := svc.now()
	  if now.After(l.ExpiresAt) {
		   svc.expireLeaseUnsafe(l)
		   return l.Copy(), ErrLeaseExpired
	   }
	   next, err := svc.leaseFSM.Apply(l.State, leaseEvApply)
	   if err != nil {
		   return l.Copy(), ErrState
	  }
	  l.State = next
	   l.AppliedAt = now
	  svc.store.PutLease(l)
	  return l.Copy(), nil
   }

// ReleaseLease 释放已应用的租约。通常在解冻完成后调用。
  func (svc *Service) ReleaseLease(leaseID string) (OverdueLease, error) {
	   if svc == nil {
		  return OverdueLease{}, ErrUnavailable
	  }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	   l,
		  ok := svc.store.GetLease(leaseID)
	   if !ok {
		  return OverdueLease{}, ErrLeaseNotFound
	   }
	  if l.State == LeaseReleased {
		   return l.Copy(), ErrLeaseReleased
	  }
	  if l.State == LeaseExpired {
		  return l.Copy(), ErrLeaseExpired
	  }
	  if l.State != LeaseApplied {
		  return l.Copy(), ErrState
	   }
	   next, err := svc.leaseFSM.Apply(l.State, leaseEvRelease)
	  if err != nil {
		   return l.Copy(), ErrState
	   }
	  l.State = next
	   l.ReleasedAt = svc.now()
	  svc.store.PutLease(l)
	  svc.store.ClearLeaseIndex(l.LoanID)
	  return l.Copy(), nil
   }

// GetLease 获取租约详情。

  func (svc *Service) GetLease(leaseID string) (OverdueLease, error) {
	   if svc == nil {
		  return OverdueLease{},
			   ErrUnavailable
	   }
	  l,
		  ok := svc.store.GetLease(leaseID)
	  if !ok {
		   return OverdueLease{}, ErrLeaseNotFound
	  }
	  return l.Copy(), nil
   }

   func (svc *Service) expireLeaseUnsafe(l *OverdueLease) {
	  l.State = LeaseExpired
	  l.ReleasedAt = svc.now()
	   svc.store.PutLease(l)
	   svc.store.ClearLeaseIndex(l.LoanID)
   }

   func (svc *Service) nextGen(loanID string) int64 {
	   var max int64
	  for _, l := range svc.store.AllLeases() {
		   if l.LoanID == loanID && l.Generation > max {
			  max = l.Generation
		   }
	  }
	   return max + 1
   }

// ValidateLease 检查租约是否仍然有效（未过期、未释放）。

  func (svc *Service) ValidateLease(leaseID string) error {
	  if svc == nil {
		   return ErrUnavailable
	   }
	  l, ok := svc.store.GetLease(leaseID)
	   if !ok {
		   return ErrLeaseNotFound
	   }
	   if l.State == LeaseReleased {
		  return ErrLeaseReleased
	   }
	  if l.State == LeaseExpired {
		  return ErrLeaseExpired
	   }
	  now := time.Now().UTC()
	  if now.After(l.ExpiresAt) {
		   return ErrLeaseExpired
	  }
	  return nil
   }
