package batchresiduefence

import (
	"context"
	"time"
)

const (
	MutexSubmit = "submit"
	MutexRetry = "retry"
)

   func (e *Engine) AcquireMutex(ctx context.Context, batchID, kind, holder string) error {
	  if err := e.require(); err != nil {
		   return err
	  }
	   id, err := requireBatchID(batchID)
	   if err != nil {
		  return err
	  }
	  act, err := requireActor(holder)
	  if err != nil {
		   return err
	  }
	   if kind != MutexSubmit && kind != MutexRetry {
		   return ErrMutexHeld
	   }
	  now := e.now()
	  cur, ok, err := e.store.GetMutex(ctx, id)
	  if err != nil {
		   return err
	  }
	  if ok && cur.ExpireAt.After(now) && (cur.Kind != kind || cur.Holder != act) {
		   if cur.Kind != kind {
			  return ErrMutexHeld
		   }
	   }
	  hold := MutexHold{
		  BatchID: id, Kind: kind, Holder: act,
		   HeldAt: now, ExpireAt: now.Add(e.mutexTTL),
	   }
	  if err := e.store.SaveMutex(ctx, hold); err != nil {
		  return err
	   }
	  e.audit(ctx, id, "", AuditMutexAcquire, "", kind, act, kind)
	  return nil
  }

  func (e *Engine) ReleaseMutex(ctx context.Context, batchID, holder string) error {
	   if err := e.require(); err != nil {
		  return err
	  }
	   id, err := requireBatchID(batchID)
	   if err != nil {
		   return err
	   }
	  act, err := requireActor(holder)
	   if err != nil {
		   return err
	  }
	  cur,
		   ok, err := e.store.GetMutex(ctx, id)
	  if err != nil {
		   return err
	   }
	  if ok && cur.Holder != act && cur.ExpireAt.After(e.now()) {
		   return ErrMutexHeld
	  }
	  if err := e.store.DeleteMutex(ctx, id); err != nil {
		  return err
	   }
	  e.audit(ctx, id, "", AuditMutexRelease, cur.Kind, "", act, "")
	  return nil
   }

  func (e *Engine) mutexAlive(ctx context.Context, batchID, againstKind string) bool {
	  cur, ok, err := e.store.GetMutex(ctx, batchID)
	  if err != nil || !ok {
		  return false
	  }
	  if !cur.ExpireAt.After(e.now()) {
		  return false
	   }
	   return cur.Kind == againstKind
  }

  func mutexTTLOr(d time.Duration) time.Duration {
	  if d <= 0 {
		  return 2 * time.Minute
	  }
	   return d
   }
