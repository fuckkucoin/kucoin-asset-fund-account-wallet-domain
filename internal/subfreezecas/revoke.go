package subfreezecas

import (
	"context"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/subacct"
)

  func (e *Engine) revokeVelocityLocked(ctx context.Context, cas *FreezeCAS) error {
	  if cas == nil {
		   return ErrCASNotFound
	  }
	  owners := append([]string{cas.ParentID},
		   cas.ChildIDs...)
	  for _, owner := range owners {
		   if err := e.revokeOwnerLocked(ctx, cas, owner); err != nil {
			   return err
		  }
	   }
	  return nil
   }

  func (e *Engine) revokeOwnerLocked(ctx context.Context, cas *FreezeCAS, owner string) error {
	   if owner == "" || cas == nil {
		   return nil
	  }
	   now := e.now()
	  row := &VelocityRevocation{
		  RevokeID: uuid.NewString(), FreezeCASID: cas.FreezeCASID, ParentID: cas.ParentID,
		  OwnerID: owner, Generation: cas.Generation, FrozenAt: now,
		   WindowFrom: now.Add(-24 * time.Hour), WindowTo: now.Add(24 * time.Hour),
		  Active: true, Note: "freeze parent velocity revoke",
	   }
	  if vel := e.books.Velocity(); vel != nil {
		  n := vel.RevokeWindow(owner, cas.FreezeCASID, cas.Generation, now)
		   row.HitCount = n
	  }
	   if err := e.revokes.Save(ctx, row); err != nil {
		  return err
	  }
	  e.audit(ctx,
		  cas.FreezeCASID, cas.ParentID, owner, cas.Actor, ActionVelRevoke, OutcomeOK, owner, cas.Generation)
	  recordRevoke()
	   return nil
   }

   func (e *Engine) deactivateRevokesLocked(ctx context.Context, casID string) {
	   rows, err := e.revokes.ListByCAS(ctx, casID)
	  if err != nil {
		  return
	  }
	  vel := e.books.Velocity()
	  for i := range rows {
		  rows[i].Active = false
		   _ = e.revokes.Save(ctx, &rows[i])
		  if vel != nil {
			   vel.ClearRevoke(rows[i].OwnerID)
		   }
	   }
  }

  func (e *Engine) assertVelocityLocked(ctx context.Context, owner string) error {
	  rows, err := e.revokes.ListActiveByOwner(ctx, owner)
	   if err != nil {
		  return err
	   }
	   now := e.now()
	   for _, r := range rows {
		  if r.Covers(owner, now) {
			  return ErrVelocityRevoked
		  }
	  }
	   if vel := e.books.Velocity(); vel != nil {
		   if err := vel.AssertNotRevoked(owner,
			   now); err != nil {
			   if err == subacct.ErrVelocity {
				   return ErrVelocityRevoked
			   }
			  return err
		   }
	  }
	  return nil
  }

// PumpRevocations 速度窗吊销泵：复扫仍冻但吊销缺失的 owner。
  func (e *Engine) PumpRevocations(ctx context.Context, batch int) (int, error) {
	  if e == nil {
		  return 0, ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  if batch <= 0 {
		   batch = 80
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  frozen,
		  err := e.cas.ListFrozen(ctx, batch)
	  if err != nil {
		  return 0, err
	   }
	   n := 0
	  for _, cas := range frozen {
		  exist, _ := e.revokes.ListByCAS(ctx, cas.FreezeCASID)
		  have := map[string]bool{}
		   for _, r := range exist {
			  have[r.OwnerID] = true
		   }
		  owners := append([]string{cas.ParentID}, cas.ChildIDs...)
		  for _, owner := range owners {
			  if have[owner] {
				   continue
			  }
			  if err := e.revokeOwnerLocked(ctx, &cas, owner); err != nil {
				   return n, err
			   }
			  n++
		  }
	   }
	  return n, nil
   }

// ListRevocations 按 CAS 列吊销。

  func (e *Engine) ListRevocations(ctx context.Context, casID string) ([]VelocityRevocation, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	  return e.revokes.ListByCAS(ctx, casID)
  }
