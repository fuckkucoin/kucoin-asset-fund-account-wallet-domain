package destlimit

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

   func transitHold(cur, to string) error {
	   if cur != HoldReserved {
		  return ErrHoldState
	  }
	  switch to {
	  case HoldConsumed, HoldReleased, HoldExpired:
		  return nil
	   default:
		   return ErrHoldState
	   }
  }

   func (e *Engine) putHold(h Hold) (Hold, error) {
	  if h.ID == "" {
		  h.ID = uuid.NewString()
	  }
	  cp := h
	   e.holds[h.ID] = &cp
	   if h.RequestID != "" {
		  e.byReq[h.RequestID] = h.ID
	   }
	   if err := e.persistHold(cp); err != nil {
		   delete(e.holds, h.ID)
		  if h.RequestID != "" {
			  delete(e.byReq, h.RequestID)
		   }
		   return Hold{}, err
	   }
	   return cp, nil
   }

   func (e *Engine) moveHold(id, to, reason, actor string) (Hold, error) {
	   id = strings.TrimSpace(id)
	  h := e.holds[id]
	   if h == nil {
		  return Hold{}, ErrNotFound
	   }
	  if err := transitHold(h.State, to); err != nil {
		  return Hold{}, err
	  }
	   prev := h.State
	  h.State = to
	   h.Reason = strings.TrimSpace(reason)
	   h.ClosedAt = e.now()
	  if err := e.persistHold(*h); err != nil {
		  h.State = prev
		  h.ClosedAt = time.Time{}
		   return Hold{}, err
	  }
	  kind := AuditRelease
	   switch to {
	  case HoldConsumed:
		  kind = AuditConsume
	   case HoldExpired:
		   kind = AuditExpire
	  }
	  e.pushAudit(Audit{
		  Kind: kind, Subject: id, Before: prev, After: to,
		  Actor: actor,
		   Detail: h.Reason, At: h.ClosedAt,
	   })
	  e.syncOccupancy(h.ClusterID,
		  h.Asset)
	  return *h, nil
  }

// Consume 把 reserved 记为已消耗。重复调用返回状态错误。
  func (e *Engine) Consume(id, reason string) (Hold, error) {
	  if e == nil {
		   return Hold{}, ErrUnavailable
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   return e.moveHold(id, HoldConsumed, reason, "")
   }

// Release 释放尚未终态的占用，额度立即让出。
  func (e *Engine) Release(id, reason string) (Hold, error) {
	  if e == nil {
		  return Hold{}, ErrUnavailable
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   return e.moveHold(id, HoldReleased, reason, "")
   }

// ReleaseAdmit 供出金预检在冻账失败时归还占用。

  func (e *Engine) ReleaseAdmit(ctx context.Context, holdID, reason string) error {
	  _ = ctx
	  _, err := e.Release(holdID, reason)
	   return err
   }

// Hold 按标识读取。

  func (e *Engine) Hold(id string) (Hold, error) {
	   if e == nil {
		   return Hold{}, ErrUnavailable
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  h := e.holds[strings.TrimSpace(id)]
	   if h == nil {
		  return Hold{}, ErrNotFound
	  }
	  return *h, nil
   }

// Holds 按用户与资产过滤。asset 空则该用户全部资产。

   func (e *Engine) Holds(userID, asset string, limit int) []Hold {
	  if e == nil {
		  return nil
	   }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  userID = strings.TrimSpace(userID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   out := make([]Hold, 0, limit)
	  for _, h := range e.holds {
		   if userID != "" && h.UserID != userID {
			  continue
		   }
		  if asset != "" && h.Asset != asset {
			   continue
		   }
		  out = append(out, *h)
		  if len(out) >= limit {
			  break
		   }
	  }
	   return out
  }

// ExpireDue 将已过截止时间的 reserved 改为 expired。

  func (e *Engine) ExpireDue(now time.Time) (int, error) {
	   if e == nil {
		   return 0, ErrUnavailable
	   }
	   if now.IsZero() {
		   now = e.now()
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  n := 0
	  ids := make([]string, 0)
	   for id, h := range e.holds {
		  if h.State != HoldReserved {
			   continue
		  }
		  if h.Deadline.IsZero() || h.Deadline.After(now) {
			   continue
		  }
		  ids = append(ids, id)
	  }
	  for _, id := range ids {
		  if _, err := e.moveHold(id, HoldExpired, "deadline", "worker"); err != nil {
			  return n, err
		  }
		  n++
	  }
	   return n, nil
  }

   func (e *Engine) syncOccupancy(cluster, asset string) {
	   if e.graph == nil || cluster == "" {
		   return
	  }
	  rs := decimal.Zero
	  cs := decimal.Zero
	  for _, h := range e.holds {
		  if h.ClusterID != cluster || h.Asset != asset {
			  continue
		   }
		  amt, err := decimal.NewFromString(h.Amount)
		   if err != nil {
			  continue
		   }
		  switch h.State {
		   case HoldReserved:
			   rs = rs.Add(amt)
		   case HoldConsumed:
			  cs = cs.Add(amt)
		   }
	   }
	   e.graph.NoteOccupancy(cluster, asset, rs.String(), cs.String())
   }
