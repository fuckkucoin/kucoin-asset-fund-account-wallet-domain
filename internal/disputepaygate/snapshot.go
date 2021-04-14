package disputepaygate

import (
	"context"
	"time"
)

// EngineSnapshot 运行快照。

type EngineSnapshot struct {
	PayIssued   int `json:"pay_issued"`
	ClawIssued  int `json:"claw_issued"`
	PayConsumed int `json:"pay_consumed"`
	ClawConsumed int `json:"claw_consumed"`
	Expired int `json:"expired"`
	Revoked int       `json:"revoked"`
	TakenAt time.Time `json:"taken_at"`
}

  func (e *Engine) Snapshot(ctx context.Context) EngineSnapshot {
	   out := EngineSnapshot{TakenAt: e.now()}
	   if e == nil {
		  return out
	  }
	  if rows, err := e.pays.ListByState(ctx, PayIssued, 1000); err == nil {
		   out.PayIssued = len(rows)
	   }
	   if rows, err := e.pays.ListByState(ctx, PayConsumed, 1000); err == nil {
		  out.PayConsumed = len(rows)
	  }
	  if rows, err := e.pays.ListByState(ctx, LeaseExpired, 1000); err == nil {
		  out.Expired += len(rows)
	  }
	  if rows, err := e.pays.ListByState(ctx, LeaseRevoked, 1000); err == nil {
		   out.Revoked += len(rows)
	  }
	  if rows, err := e.claws.ListByState(ctx, ClawIssued, 1000); err == nil {
		  out.ClawIssued = len(rows)
	   }
	  if rows, err := e.claws.ListByState(ctx, ClawConsumed, 1000); err == nil {
		   out.ClawConsumed = len(rows)
	  }
	  if rows, err := e.claws.ListByState(ctx, LeaseExpired, 1000); err == nil {
		   out.Expired += len(rows)
	  }
	  if rows, err := e.claws.ListByState(ctx, LeaseRevoked, 1000); err == nil {
		  out.Revoked += len(rows)
	  }
	   return out
   }

// ReadLeaseID 只读导出租约标识。
  func ReadLeaseID(l *DisputePayLease) string {
	  if l == nil {
		  return ""
	   }
	   return l.LeaseID
  }
