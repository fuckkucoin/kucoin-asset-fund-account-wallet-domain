package ledshard

import (
	"context"
	"time"
)

// Tick 扫描在途意图：漂移进台，超时或被杀则补偿，已贷记则向前提交。
// auto 打开时，健康意图也会向前一步。
   func (e *Engine) Tick() (TickReport, error) {
	  if e == nil {
		   return TickReport{}, ErrRouteMissing
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  var rep TickReport
	   ids := make([]string,
		  0, len(e.mem.intents))
	   for id := range e.mem.intents {
		   ids = append(ids, id)
	  }
	   for _, id := range ids {
		  it, ok := e.mem.intent(id)
		  if !ok || it.State.Terminal() {
			  continue
		   }
		  rep.Scanned++
		   if err := e.tickOne(&it, &rep); err != nil {
			   rep.Errors++
		   }
	  }
	  if err := e.syncLocked(); err != nil {
		  return rep, err
	  }
	  return rep, nil
   }

   func (e *Engine) tickOne(it *Intent, rep *TickReport) error {
	   if it.State == StateDesk {
		   rep.Desk++
		   return nil
	  }
	   if err := e.guardRoute(it); err != nil {
		  rep.Drift++
		  rep.Desk++
		   return nil
	  }
	  fresh, _ := e.mem.intent(it.ID)
	  *it = fresh
	  if it.State == StateDesk || it.Manual {
		   if it.State != StateDesk && it.Manual {
			  _ = e.parkDesk(it, "freeze_hold", it.Reason)
			   rep.Desk++
		  }
		   return nil
	   }
	   due := !it.Deadline.IsZero() && e.now().After(it.Deadline)
	   if it.Halted || due {
		  if it.State == StateCredited && it.CreditPosted {
			   if err := e.stepCommit(it); err != nil {
				   return err
			  }
			   rep.Committed++
			   return nil
		  }
		   out, err := e.compensateLocked(it, "timeout", false)
		   if err != nil {
			   if out.State == StateDesk {
				   rep.Desk++
				  return nil
			   }
			   return err
		   }
		   if out.State == StateAborted {
			   rep.Aborted++
		  } else {
			   rep.Compensated++
		   }
		  return nil
	  }
	  if it.State == StateCredited && it.CreditPosted {
		  if err := e.stepCommit(it); err != nil {
			  return err
		  }
		   rep.Committed++
		   return nil
	   }
	  if !e.auto {
		   return nil
	   }
	  before := it.State
	   _, err := e.advanceLocked(it.ID)
	   if err != nil {
		  if err == ErrHalted || err == ErrDrift || err == ErrDesk || err == ErrTerminal {
			  return nil
		   }
		  return err
	   }
	  after, _ := e.mem.intent(it.ID)
	   if after.State != before {
		  rep.Advanced++
		  if after.State == StateCommitted {
			   rep.Committed++
		   }
	   }
	   return nil
  }

// Loop 按间隔跑 Tick，直到 ctx 结束。

  func (e *Engine) Loop(ctx context.Context, every time.Duration) {
	  if e == nil {
		  return
	  }
	  if every <= 0 {
		  every = 2 * time.Second
	  }
	  t := time.NewTicker(every)
	   defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-t.C:
			   _, _ = e.Tick()
		   }
	  }
   }
