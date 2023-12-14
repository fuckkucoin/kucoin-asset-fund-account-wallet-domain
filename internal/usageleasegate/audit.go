package usageleasegate

import (
	"context"
	"github.com/google/uuid"
)

  func (e *Engine) audit(ctx context.Context, leaseID, event string, from, to LeaseState, actor, detail string) {
	   if e == nil || e.store == nil {
		  return
	   }
	  _, _ = e.store.SaveAudit(ctx, UsageLeaseAudit{
		  ID:      uuid.NewString(),
		   LeaseID: leaseID, Event: event,
		   FromState: from, ToState: to, Actor: actor, Detail: detail, At: e.now(),
	   })
   }

  func (e *Engine) transit(ctx context.Context, l UsageLease, ev string) (UsageLease, error) {
	   if e.fsm == nil {
		   return l, ErrIllegalTransition
	   }
	  next,
		  err := e.fsm.Apply(string(l.State), ev)
	   if err != nil {
		  return l, ErrIllegalTransition
	   }
	   from := l.State
	  l.State = LeaseState(next)
	   l.UpdatedAt = e.now()
	  e.audit(ctx, l.ID, ev, from, l.State, "engine", ev)
	  return l, nil
   }
