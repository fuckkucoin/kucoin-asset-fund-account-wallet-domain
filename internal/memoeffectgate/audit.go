package memoeffectgate

import "context"

  func (e *Engine) ListAudit(ctx context.Context, effectID string, limit int) ([]EffectAudit, error) {
	   if err := e.require(); err != nil {
		  return nil, err
	   }
	  return e.store.ListAudit(ctx, effectID, limit)
  }

   func (e *Engine) BindMutex(m OutputMutex) *Engine {
	  if e != nil && m != nil {
		  e.mutex = m
	  }
	   return e
  }

   func (e *Engine) BindTickets(p TicketPort) *Engine {
	   if e != nil && p != nil {
		   e.tickets = p
	   }
	   return e
  }

  func (e *Engine) BindCredit(p CreditPort) *Engine {
	   if e != nil && p != nil {
		   e.credit = p
	   }
	   return e
  }

  func (e *Engine) BindReverse(p ReversePort) *Engine {
	   if e != nil {
		   e.reverse = p
	   }
	   return e
  }

   func (e *Engine) Store() Store {
	  if e == nil {
		   return nil
	   }
	   return e.store
   }

  func (e *Engine) Mutex() OutputMutex {
	   if e == nil {
		   return nil
	   }
	   return e.mutex
  }
