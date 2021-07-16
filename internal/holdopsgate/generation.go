package holdopsgate

import (
	"context"
	"time"
)

type GenerationEngine struct {
	store  Store
	secret string
	clock  func() time.Time
}

   func newGenerationEngine(store Store, secret string, clock func() time.Time) *GenerationEngine {
	   return &GenerationEngine{store: store, secret: secret, clock: clock}
   }

  func (e *GenerationEngine) Current(ctx context.Context) (HoldOpsGeneration, error) {
	  if e == nil || e.store == nil {
		  return HoldOpsGeneration{}, ErrUnavailable
	   }
	   return e.store.GetGeneration(ctx)
   }

   func (e *GenerationEngine) Bump(ctx context.Context, expect int64, fence bool, note string) (HoldOpsGeneration, error) {
	  if e == nil || e.store == nil {
		  return HoldOpsGeneration{}, ErrUnavailable
	   }
	  now := e.clock()
	   gen, err := e.store.CASBumpOps(ctx, expect, fence, note, now)
	   if err != nil {
		  return gen, err
	  }
	  open, _ := e.store.ListOpenLeases(ctx)
	   for _, l := range open {
		   if l.OpsGen < gen.OpsGen && l.Status.Writable() {
			   to, ok := CanTransit(l.Status, "revoke")
			  if !ok {
				   continue
			  }
			   l.Status = to
			  l.TerminalAt = now
			   l.Note = firstNonEmpty(l.Note, "revoked-by-gen-bump")
			  _ = e.store.UpdateLease(ctx, l)
		  }
	  }
	  return gen, nil
   }

   func (e *GenerationEngine) Match(ctx context.Context, lease HoldOpsLease) error {
	   gen, err := e.Current(ctx)
	   if err != nil {
		  return err
	   }
	  if gen.FenceOn {
		  return ErrFenceOn
	   }
	  if lease.OpsGen != gen.OpsGen {
		   return ErrStaleGen
	  }
	   return nil
  }
