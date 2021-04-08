package deppartial

import (
	"strings"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────
// 重开互斥租约管理
// ──────────────────────────────────────────────────
// AcquireLease 获取重开互斥租约。同一批次同时只有一个 holder 可持有租约。
// 双人控制：holder 和 approver 不得为同一人。
   func (svc *Service) AcquireLease(req ReopenReq) (ReopenLease, error) {
	   if svc == nil {
		  return ReopenLease{}, ErrUnavailable
	   }
	   if err := requireActor(req.Holder); err != nil {
		   return ReopenLease{}, err
	   }
	  if err := requireActor(req.Approver); err != nil {
		  return ReopenLease{}, ErrDualControl
	   }
	  holder := strings.TrimSpace(req.Holder)
	   approver := strings.TrimSpace(req.Approver)
	  if holder == approver {
		   return ReopenLease{}, ErrDualControl
	  }
	  batchID := strings.TrimSpace(req.BatchID)
	  if batchID == "" {
		   return ReopenLease{}, ErrBatchNotFound
	  }
	   state, ok := svc.store.GetBatchState(batchID)
	   if !ok {
		   return ReopenLease{},
			  ErrBatchNotFound
	   }
	  if state == BRClosed {
		  return ReopenLease{}, ErrAlreadyClosed
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	  now := svc.now()
	   if existing, ok := svc.store.GetActiveLease(batchID); ok {
		  if now.Before(existing.ExpiresAt) && existing.Holder != holder {
			  return ReopenLease{}, ErrLeaseHeld
		  }
		   if now.After(existing.ExpiresAt) {
			   existing.Status = LeaseExpired
			  svc.store.UpdateLease(existing)
		   }
	   }
	  gen := svc.store.NextGen()
	  lease := &ReopenLease{
		   ID:      uuid.NewString(),
		   BatchID: batchID,
		   Holder:     holder,
		  Generation: gen,
		  Status:     LeaseHeld,
		  AcquiredAt: now,
		  ExpiresAt:  now.Add(svc.leaseTTL),
		   Reason: strings.TrimSpace(req.Reason),
	  }
	  svc.store.PutLease(lease)
	  if svc.batchFSM.Can(state, brEvReopen) {
		  next, err := svc.batchFSM.Apply(state, brEvReopen)
		   if err == nil {
			  svc.store.SetBatchState(batchID, next)
		   }
	  }
	  return lease.Copy(), nil
  }

// ReleaseLease 释放重开互斥租约。
  func (svc *Service) ReleaseLease(leaseID, actor string) (ReopenLease, error) {
	  if svc == nil {
		  return ReopenLease{}, ErrUnavailable
	   }
	   if err := requireActor(actor); err != nil {
		   return ReopenLease{}, err
	   }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	  l, ok := svc.store.GetLease(leaseID)
	   if !ok {
		  return ReopenLease{}, ErrLeaseNotFound
	   }
	   if l.Status != LeaseHeld {
		  return l.Copy(), ErrLeaseExpired
	   }
	   if l.Holder != strings.TrimSpace(actor) {
		  return l.Copy(), ErrLeaseHeld
	  }
	  l.Status = LeaseReleased
	   l.ReleasedAt = svc.now()
	  svc.store.UpdateLease(l)
	  return l.Copy(), nil
   }

// GetActiveLease 查询批次活跃租约。

   func (svc *Service) GetActiveLease(batchID string) (ReopenLease, error) {
	  if svc == nil {
		  return ReopenLease{}, ErrUnavailable
	   }
	   l, ok := svc.store.GetActiveLease(batchID)
	  if !ok {
		   return ReopenLease{}, ErrLeaseNotFound
	  }
	  now := svc.now()
	  if now.After(l.ExpiresAt) {
		   svc.mu.Lock()
		   l.Status = LeaseExpired
		   svc.store.UpdateLease(l)
		  svc.mu.Unlock()
		   return l.Copy(), ErrLeaseExpired
	  }
	  return l.Copy(), nil
   }

// ExpireLeases 过期扫描：释放所有过期租约。
   func (svc *Service) ExpireLeases() []ReopenLease {
	   if svc == nil {
		   return nil
	  }
	   now := svc.now()
	   expired := svc.store.ExpiredLeases(now)
	   svc.mu.Lock()
	   defer svc.mu.Unlock()
	   var released []ReopenLease
	   for _, l := range expired {
		   cur, ok := svc.store.GetLease(l.ID)
		  if !ok || cur.Status != LeaseHeld {
			   continue
		   }
		  cur.Status = LeaseExpired
		   cur.ReleasedAt = now
		   svc.store.UpdateLease(cur)
		  released = append(released, cur.Copy())
	   }
	   return released
  }
