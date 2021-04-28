package deppartial

import (
	"strings"
)

// ──────────────────────────────────────────────────
// 残余修复：贷记修复 / 放弃 / 升级
// ──────────────────────────────────────────────────
// RepairCredit 修复残余行：补充贷记。必须持有当前代际租约且 (chain,tx_hash) 未被贷记过。

   func (svc *Service) RepairCredit(req RepairCreditReq) (ObserveResidue, error) {
	  if svc == nil {
		   return ObserveResidue{},
			  ErrUnavailable
	  }
	   if err := requireActor(req.Actor); err != nil {
		   return ObserveResidue{}, err
	   }
	   batchID := strings.TrimSpace(req.BatchID)
	  residueID := strings.TrimSpace(req.ResidueID)
	   if batchID == "" {
		  return ObserveResidue{}, ErrBatchNotFound
	  }
	  if residueID == "" {
		   return ObserveResidue{}, ErrResidueNotOpen
	  }
	  state, ok := svc.store.GetBatchState(batchID)
	   if !ok {
		   return ObserveResidue{}, ErrBatchNotFound
	   }
	  if state == BRClosed {
		  return ObserveResidue{},
			  ErrAlreadyClosed
	   }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	   lease, hasLease := svc.store.GetActiveLease(batchID)
	   if !hasLease {
		  return ObserveResidue{},
			  ErrLeaseNotFound
	   }
	  now := svc.now()
	  if now.After(lease.ExpiresAt) {
		   lease.Status = LeaseExpired
		  svc.store.UpdateLease(lease)
		   return ObserveResidue{}, ErrLeaseExpired
	   }
	   if req.Generation != 0 && req.Generation != lease.Generation {
		   return ObserveResidue{}, ErrStaleGen
	   }
	   r, rok := svc.store.GetResidue(residueID)
	  if !rok {
		   return ObserveResidue{},
			  ErrResidueNotOpen
	  }
	  if r.BatchID != batchID {
		  return ObserveResidue{}, ErrBatchNotFound
	   }
	   if r.State != ResOpen && r.State != ResEscalated {
		   return ObserveResidue{}, ErrResidueNotOpen
	   }
	   if !svc.store.MarkCredited(r.Chain, r.TxHash) {
		  return ObserveResidue{},
			  ErrDualCredit
	   }
	  next, err := svc.residueFSM.Apply(r.State, resEvCreditRepair)
	  if err != nil {
		   return ObserveResidue{}, ErrState
	   }
	  r.State = next
	   r.LedgerID = strings.TrimSpace(req.LedgerID)
	   r.Amount = strings.TrimSpace(req.Amount)
	  r.ResolvedAt = now
	  r.ResolvedBy = strings.TrimSpace(req.Actor)
	   svc.store.UpdateResidue(r)
	  svc.store.CloseFence(r.Chain, r.TxHash)
	   return r.Copy(), nil
   }

// Abandon 放弃残余行。
   func (svc *Service) Abandon(req AbandonReq) (ObserveResidue, error) {
	   if svc == nil {
		   return ObserveResidue{},
			  ErrUnavailable
	   }
	  if err := requireActor(req.Actor); err != nil {
		   return ObserveResidue{}, err
	  }
	   batchID := strings.TrimSpace(req.BatchID)
	  residueID := strings.TrimSpace(req.ResidueID)
	   if batchID == "" {
		  return ObserveResidue{}, ErrBatchNotFound
	  }
	  state, ok := svc.store.GetBatchState(batchID)
	  if !ok {
		   return ObserveResidue{}, ErrBatchNotFound
	   }
	  if state == BRClosed {
		  return ObserveResidue{}, ErrAlreadyClosed
	  }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   r, rok := svc.store.GetResidue(residueID)
	   if !rok {
		  return ObserveResidue{}, ErrResidueNotOpen
	   }
	   if r.BatchID != batchID {
		   return ObserveResidue{}, ErrBatchNotFound
	   }
	  if r.State != ResOpen && r.State != ResEscalated {
		  return ObserveResidue{},
			   ErrResidueNotOpen
	   }
	  next, err := svc.residueFSM.Apply(r.State, resEvAbandon)
	   if err != nil {
		   return ObserveResidue{}, ErrState
	  }
	  r.State = next
	  r.Reason = strings.TrimSpace(req.Reason)
	   r.ResolvedAt = svc.now()
	   r.ResolvedBy = strings.TrimSpace(req.Actor)
	   svc.store.UpdateResidue(r)
	  svc.store.CloseFence(r.Chain,
		   r.TxHash)
	   return r.Copy(), nil
   }

// Escalate 升级残余行（标记需人工介入）。

   func (svc *Service) Escalate(req EscalateReq) (ObserveResidue, error) {
	   if svc == nil {
		  return ObserveResidue{}, ErrUnavailable
	  }
	  if err := requireActor(req.Actor); err != nil {
		   return ObserveResidue{}, err
	   }
	   batchID := strings.TrimSpace(req.BatchID)
	  residueID := strings.TrimSpace(req.ResidueID)
	   if batchID == "" {
		   return ObserveResidue{}, ErrBatchNotFound
	   }
	   state, ok := svc.store.GetBatchState(batchID)
	  if !ok {
		   return ObserveResidue{},
			  ErrBatchNotFound
	  }
	  if state == BRClosed {
		  return ObserveResidue{}, ErrAlreadyClosed
	  }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	  r, rok := svc.store.GetResidue(residueID)
	  if !rok {
		   return ObserveResidue{},
			   ErrResidueNotOpen
	   }
	   if r.BatchID != batchID {
		   return ObserveResidue{},
			  ErrBatchNotFound
	   }
	   if r.State != ResOpen {
		   return ObserveResidue{}, ErrResidueNotOpen
	   }
	   next, err := svc.residueFSM.Apply(r.State, resEvEscalate)
	   if err != nil {
		   return ObserveResidue{}, ErrState
	  }
	   r.State = next
	   r.Reason = strings.TrimSpace(req.Reason)
	   svc.store.UpdateResidue(r)
	  return r.Copy(), nil
  }
