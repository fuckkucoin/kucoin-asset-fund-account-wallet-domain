package ledshard

import "strings"

   func actorHuman(actor string) error {
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") || actor == "ledshard-worker" {
		   return ErrActor
	  }
	  return nil
   }

// Park 把仍冻结或在途的意图送上台，不自动解冻。双人执行后才补偿。
   func (e *Engine) Park(id, reason string) (Intent, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  it, ok := e.mem.intent(id)
	  if !ok {
		   return Intent{}, ErrNotFound
	  }
	   if it.State.Terminal() {
		  return it, ErrTerminal
	  }
	   it.Halted = true
	  if err := e.parkDesk(&it, "freeze_hold", reason); err != nil && it.State != StateDesk {
		   return it, err
	  }
	  if err := e.syncLocked(); err != nil {
		   return it, err
	  }
	   got, _ := e.mem.intent(id)
	  return got, nil
  }

// OpenDesk 由人工打开补偿单。已有 worker 开的单则直接返回，避免另起一单。
   func (e *Engine) OpenDesk(intentID, actor, reason string) (DeskCase, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   if err := actorHuman(actor); err != nil {
		  return DeskCase{}, err
	  }
	   it, ok := e.mem.intent(intentID)
	   if !ok {
		  return DeskCase{}, ErrNotFound
	  }
	   if it.DeskID != "" {
		  d, ok := e.mem.desk(it.DeskID)
		   if ok {
			   return d, nil
		   }
	   }
	  if it.State.Terminal() {
		   return DeskCase{}, ErrTerminal
	   }
	  now := e.now()
	  d := DeskCase{
		   ID: e.id("dsk"), IntentID: it.ID, State: DeskOpen,
		  OpenedBy: actor, Reason: reason, CreatedAt: now, UpdatedAt: now,
	   }
	  e.mem.putDesk(d)
	   it.DeskID = d.ID
	   it.State = StateDesk
	   it.Manual = true
	  it.Reason = reason
	  e.stamp(&it)
	   if err := e.syncLocked(); err != nil {
		   return DeskCase{}, err
	  }
	  return d, nil
   }

// ApproveDesk 记录另一人的批准。申请人不能批准自己的单。

  func (e *Engine) ApproveDesk(id, actor string) (DeskCase, error) {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  if err := actorHuman(actor); err != nil {
		   return DeskCase{}, err
	   }
	  d, ok := e.mem.desk(id)
	  if !ok {
		   return DeskCase{}, ErrNotFound
	  }
	   if d.State == DeskExecuted {
		  return d, ErrTerminal
	  }
	  if actor == d.OpenedBy {
		   return d, ErrDual
	  }
	   d.ApprovedBy = actor
	  d.State = DeskApproved
	   d.UpdatedAt = e.now()
	   e.mem.putDesk(d)
	  if err := e.syncLocked(); err != nil {
		  return DeskCase{}, err
	   }
	   return d, nil
   }

// ExecuteDesk 是补偿按钮。须已有另一人批准，且点击者不是开单人。
// worker 开的单还要求点击者与批准人不是同一人，这样台上始终有两名操作员。
  func (e *Engine) ExecuteDesk(id, actor string) (Intent, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   if err := actorHuman(actor); err != nil {
		   return Intent{}, err
	   }
	  d, ok := e.mem.desk(id)
	  if !ok {
		   return Intent{}, ErrNotFound
	   }
	  if d.State == DeskExecuted {
		  it, _ := e.mem.intent(d.IntentID)
		   return it, nil
	   }
	   if d.State != DeskApproved || d.ApprovedBy == "" || d.ApprovedBy == d.OpenedBy {
		   return Intent{}, ErrDual
	  }
	   if actor == d.OpenedBy {
		   return Intent{}, ErrDual
	  }
	  if d.OpenedBy == "ledshard-worker" && actor == d.ApprovedBy {
		  return Intent{}, ErrDual
	  }
	   it, ok := e.mem.intent(d.IntentID)
	  if !ok {
		   return Intent{}, ErrNotFound
	   }
	  out,
		  err := e.compensateLocked(&it, d.Reason, true)
	   if err != nil {
		  return out, err
	   }
	   d.State = DeskExecuted
	   d.ExecutedBy = actor
	   d.UpdatedAt = e.now()
	  e.mem.putDesk(d)
	  if err := e.syncLocked(); err != nil {
		  return out, err
	   }
	  got, _ := e.mem.intent(out.ID)
	  return got, nil
   }

  func (e *Engine) GetDesk(id string) (DeskCase, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  d, ok := e.mem.desk(id)
	  if !ok {
		  return DeskCase{}, ErrNotFound
	   }
	  return d, nil
   }

  func (e *Engine) ListDesk() []DeskCase {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   out := make([]DeskCase, 0, len(e.mem.desks))
	   for _, d := range e.mem.desks {
		   out = append(out, *d)
	  }
	  return out
   }
