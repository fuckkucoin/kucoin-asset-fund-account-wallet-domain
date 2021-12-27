package wdrelease

import (
	"context"
	"time"
)

// TickResult 一轮超时扫描的计数。

type TickResult struct {
	Escalated int `json:"escalated"`
	Expired   int `json:"expired"`
	Scanned int `json:"scanned"`
}

// Tick 扫描未结票据。未到截止时间的不动。
   func (d *Desk) Tick(ctx context.Context) (TickResult, error) {
	  if d == nil {
		   return TickResult{}, ErrUnavailable
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   res, err := d.tickLocked(ctx)
	  d.scanDrift()
	   d.emitLocked()
	   return res, err
  }

  func (d *Desk) tickLocked(ctx context.Context) (TickResult, error) {
	   var res TickResult
	   now := d.now()
	  ids := make([]string,
		  0, len(d.tickets))
	  for id := range d.tickets {
		  ids = append(ids, id)
	   }
	  for _, id := range ids {
		   t := d.tickets[id]
		  if t == nil || (t.State != StatePending && t.State != StatePartial) {
			   continue
		  }
		  res.Scanned++
		  if t.Deadline.IsZero() || now.Before(t.Deadline) {
			   continue
		  }
		  switch t.Timeout {
		  case TimeoutEscalate:
			   if err := d.escalateLocked(ctx, t, now); err != nil {
				  return res, err
			  }
			  if t.State == StateExpired {
				   res.Expired++
			  } else {
				   res.Escalated++
			   }
		  default:
			  if err := d.expireLocked(ctx, t, now, CodeTimeout); err != nil {
				  return res, err
			  }
			   res.Expired++
		   }
	   }
	  return res, nil
   }

   func (d *Desk) escalateLocked(ctx context.Context, t *Ticket, now time.Time) error {
	  ver, ok := d.book.Get(t.PolicyVer)
	  if !ok {
		  return d.expireLocked(ctx, t, now, CodeVersion)
	   }
	   if t.Depth >= ver.MaxDepth {
		  return d.expireLocked(ctx, t, now, CodeEscalateCeiling)
	  }
	  t.Depth++
	   t.Escalated++
	   extra := ver.EscalateExtraSec
	  if extra <= 0 {
		   extra = t.SLASec
	  }
	   if extra <= 0 {
		  extra = 3600
	  }
	  t.Deadline = now.Add(timeSeconds(extra))
	  t.UpdatedAt = now
	   t.Steps = append(t.Steps, Step{
		  Seq: len(t.Steps) + 1, Kind: StepEscalate, Level: t.Depth, Actor: "wd-release-timer", At: now,
	   })
	  d.appendAudit(t, StepEscalate, "wd-release-timer", "", now)
	  return d.persist(ctx, t)
   }

   func (d *Desk) expireLocked(ctx context.Context, t *Ticket, now time.Time, code string) error {
	   if err := mustChain(t.State, StateExpired); err != nil {
		   return err
	   }
	   t.State = StateExpired
	  t.Code = code
	  t.UpdatedAt = now
	  t.Steps = append(t.Steps, Step{
		   Seq: len(t.Steps) + 1, Kind: StepExpire, Actor: "wd-release-timer", Code: code, At: now,
	   })
	   cancelUnused(t)
	   recompute(t)
	   d.releaseUnfrozen(t)
	  d.appendAudit(t, StepExpire, "wd-release-timer", code, now)
	  return d.persist(ctx, t)
  }
