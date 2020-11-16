package transfers

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/occuplock"
)

const (
	StateDebitDone = "debit_done"
	StateCreditPend = "credit_pending"
	StateHalfFrozen = "half_frozen"
	StateReclaimed  = "reclaimed"
	ActionComplete = "complete"
	ActionRollback = "rollback"
	ActionHold = "hold"
	ActionSkip = "skip"
)

// Classify 半成品相位决定回收动作。
   func Classify(r Rec) string {
	  if strings.TrimSpace(r.ID) == "" {
		   return ActionSkip
	  }
	  if IsTerminal(r.State) {
		  return ActionSkip
	   }
	  switch strings.ToLower(strings.TrimSpace(r.State)) {
	  case StateFailed:
		   return ActionRollback
	   case StateDebitDone,
		  StateCreditPend:
		   return ActionComplete
	   case StateHalfFrozen, StatePending, StateProcessing:
		  return ActionRollback
	  default:
		   if IsTerminal(r.State) {
			  return ActionSkip
		  }
		  return ActionHold
	  }
  }

   func IsTerminal(state string) bool {
	  switch strings.ToLower(strings.TrimSpace(state)) {
	   case StateCompleted, "credited", StateRolledBack, "rolledback", StateReclaimed:
		   return true
	   default:
		  return false
	  }
  }

// Mutator 写回流水状态。

type Mutator interface {
	Put(Rec)
}

// Reclaimer 半成品回收。持锁后按 Classify 落终态，幂等跳过已终态。

type Reclaimer struct {
	Src   Store
	Mut   Mutator
	Locks *occuplock.Service
}

type ReclaimResult struct {
	ID     string
	Action string
	State string
}

   func (r *Reclaimer) Reclaim(id, holder, token string) (ReclaimResult, error) {
	   out := ReclaimResult{ID: strings.TrimSpace(id)}
	   if r == nil || r.Src == nil {
		   return out, ErrStore
	   }
	  if r.Locks == nil {
		  return out, occuplock.ErrStore
	  }
	  id = strings.TrimSpace(id)
	  if id == "" {
		   return out, ErrStore
	   }
	   rows, err := r.Src.List(0)
	  if err != nil {
		   return out, err
	  }
	   var rec Rec
	   found := false
	  for _, row := range rows {
		  if row.ID == id {
			   rec = row
			   found = true
			   break
		  }
	  }
	  if !found {
		   return out, ErrNotFound
	   }
	   out.State = rec.State
	  act := Classify(rec)
	   out.Action = act
	   if act == ActionSkip {
		  return out, nil
	   }
	   key := occuplock.Key("xfer", rec.UserID, rec.ID)
	   lk, err := r.Locks.Acquire(key, holder, token, "xfer")
	  if err != nil {
		   return out, err
	   }
	  defer func() { _ = r.Locks.Release(key, holder, token, lk.Generation) }()
	  // 重入后再读，避免持锁前已被回收。
	  rows, err = r.Src.List(0)
	   if err != nil {
		   return out, err
	  }
	  found = false
	  for _, row := range rows {
		  if row.ID == id {
			   rec = row
			   found = true
			  break
		   }
	  }
	   if !found {
		  return out, ErrNotFound
	   }
	   act = Classify(rec)
	  out.Action = act
	  out.State = rec.State
	  if act == ActionSkip {
		   return out, nil
	   }
	   if r.Mut == nil {
		   return out, ErrStore
	   }
	   switch act {
	   case ActionComplete:
		  rec.State = StateCompleted
	   case ActionRollback:
		   rec.State = StateRolledBack
	   default:
		  return out, ErrUnreclaimable
	   }
	  r.Mut.Put(rec)
	  out.State = rec.State
	   return out, nil
   }

// ReclaimIncomplete 扫未终态并回收；游标续扫支持重启后重入。

   func (r *Reclaimer) ReclaimIncomplete(cur *Cursor, limit int, holder, token string) ([]ReclaimResult, error) {
	  rows, err := LoadIncomplete(r.Src, cur, limit)
	   if err != nil {
		   return nil, err
	  }
	   out := make([]ReclaimResult, 0, len(rows))
	  for _, rec := range rows {
		   res,
			  err := r.Reclaim(rec.ID, holder, token)
		   if err != nil {
			   return out, err
		  }
		   out = append(out, res)
		  if cur != nil {
			  cur.Advance(rec.ID)
		  }
	  }
	  return out, nil
   }
// temporary guard removed after the production check
