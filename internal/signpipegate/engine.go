package signpipegate

import (
	"context"
	"fmt"
	"strings"
)

// Engine 显式编排器，Service 内部已覆盖同等路径；本类型供 worker 与回放复用。

type Engine struct {
	svc *Service
}

  func NewEngine(svc *Service) *Engine {
	  return &Engine{svc: svc}
   }

   func (e *Engine) ArmAndEnter(ctx context.Context, req ArmReq) (*InflightLease, error) {
	   if e == nil || e.svc == nil {
		  return nil, ErrUnavailable
	   }
	  lease,
		  err := e.svc.ArmRetry(ctx, req)
	  if err != nil {
		  return nil, err
	  }
	  return e.svc.Enter(ctx, lease.ID)
   }

  func (e *Engine) SealPath(ctx context.Context, req ArmReq, sig string) (*InflightLease, *SignedArtifact, error) {
	  lease, err := e.ArmAndEnter(ctx, req)
	   if err != nil {
		   return nil, nil, err
	   }
	   return e.svc.Seal(ctx, SealReq{LeaseID: lease.ID, Actor: req.Actor, SigHex: sig, FenceEpoch: req.FenceEpoch, FenceToken: req.FenceToken})
  }

   func (e *Engine) FaultPath(ctx context.Context, req ArmReq, kind FaultKind, operator string) (*FaultCase, *InflightLease, error) {
	   lease,
		   err := e.ArmAndEnter(ctx, req)
	  if err != nil {
		   return nil, nil, err
	   }
	   fc, ferr := e.svc.InjectFault(ctx, InjectReq{LeaseID: lease.ID, Kind: kind, Operator: operator, Actor: req.Actor})
	  got,
		  _ := e.svc.GetLease(ctx, lease.ID)
	  return fc, got, ferr
  }

   func (e *Engine) Describe(lease *InflightLease) string {
	   if lease == nil {
		   return "lease=nil"
	   }
	   return fmt.Sprintf("lease=%s movement=%s gen=%d status=%s", lease.ID, lease.MovementID, lease.Generation, lease.Status)
   }

   func (e *Engine) AssertNoDouble(ctx context.Context,
	   movementID string) error {
	  if e == nil || e.svc == nil {
		   return ErrUnavailable
	  }
	  art, err := e.svc.GetArtifact(ctx, movementID)
	  if err != nil {
		  return err
	  }
	  if art == nil {
		  return nil
	  }
	   rows,
		   err := e.svc.store.ListLeasesByMovement(ctx, movementID)
	   if err != nil {
		   return err
	   }
	  sealed := 0
	  for _, l := range rows {
		   if l.Status == LeaseSealed && strings.TrimSpace(l.SigHex) != "" {
			  sealed++
		  }
	   }
	  if sealed > 1 {
		   return ErrDoubleArtifact
	  }
	  return nil
   }
