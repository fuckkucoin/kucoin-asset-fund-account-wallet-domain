package wdrelease

import (
	"context"
	"encoding/json"
	"account-assets.kucoin.net/funding/internal/approvestore"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/shopspring/decimal"
)

const (
	kvTicket = "wd_release_ticket"
	kvGrant = "wd_release_grant"
	kvAudit = "wd_release_audit"
)

  func (d *Desk) persist(ctx context.Context, t *Ticket) error {
	   if d == nil || t == nil {
		  return ErrUnavailable
	  }
	  recompute(t)
	  if d.kv != nil {
		   body, err := json.Marshal(t)
		  if err != nil {
			   return err
		  }
		  exp := persist.Fence{Epoch: t.FenceEpoch, Token: t.FenceToken}
		   if err := d.kv.Put(ctx, persist.Row{Kind: kvTicket, Key: t.ID, Body: body}, exp); err != nil {
			   return err
		   }
		   got, err := d.kv.Get(ctx, kvTicket, t.ID)
		  if err != nil {
			   return err
		   }
		   t.FenceEpoch = got.Fence.Epoch
		   t.FenceToken = got.Fence.Token
		   idemBody, err := json.Marshal(map[string]string{"id": t.ID, "fp": t.FP})
		   if err != nil {
			  return err
		   }
		  if err := d.putFenced(ctx, "wd_release_idem", t.Idem, idemBody, d.idemFence); err != nil {
			   return err
		   }
		   ab,
			   err := json.Marshal(d.audits[t.ID])
		  if err != nil {
			  return err
		   }
		   if err := d.putFenced(ctx, kvAudit, t.ID, ab, d.auditFence); err != nil {
			   return err
		  }
	   }
	  if d.sqlOn() {
		  if err := d.putSQL(ctx, t); err != nil {
			  return err
		   }
	   }
	   if d.refs != nil {
		  if err := d.refs.NoteRelease(ctx, approvestore.ReleaseRef{
			   ID:       t.ID,
			   TicketID: t.ID, Applicant: t.Applicant, PolicyVer: t.PolicyVer,
			  State: t.State, Amount: t.Amount, Asset: t.Asset, Kind: approvestore.KindWDRelease,
		   }); err != nil {
			   return err
		  }
	  }
	   return nil
  }

  func (d *Desk) persistGrant(ctx context.Context, g *Grant) error {
	   if d == nil || g == nil || d.kv == nil {
		  return nil
	  }
	  body, err := json.Marshal(g)
	  if err != nil {
		  return err
	  }
	   return d.putFenced(ctx,
		   kvGrant, g.Idem, body, d.grantFence)
   }

  func (d *Desk) putFenced(ctx context.Context, kind, key string, body []byte, fences map[string]persist.Fence) error {
	  exp := fences[key]
	  if err := d.kv.Put(ctx, persist.Row{Kind: kind, Key: key, Body: body}, exp); err != nil {
		   return err
	   }
	   got, err := d.kv.Get(ctx, kind, key)
	   if err != nil {
		   return err
	  }
	   fences[key] = got.Fence
	   return nil
   }

// Hydrate 从 KV 装回票据、占用和审计。失败则台面停摆，冻账入口失败关闭。
  func (d *Desk) Hydrate(ctx context.Context) error {
	   if d == nil {
		  return ErrUnavailable
	   }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  if d.hydrated || d.kv == nil {
		   return nil
	   }
	   if err := d.loadKind(ctx, kvTicket, func(row persist.Row) error {
		   var t Ticket
		  if err := json.Unmarshal(row.Body, &t); err != nil {
			  return err
		  }
		   t.FenceEpoch = row.Fence.Epoch
		  t.FenceToken = row.Fence.Token
		   d.tickets[t.ID] = &t
		   if t.Idem != "" {
			  d.byIdem[t.Idem] = t.ID
		   }
		  d.seenAsset[t.Asset] = struct{}{}
		  d.rebuildHold(&t)
		   return nil
	  }); err != nil {
		   d.halted = err
		   return err
	  }
	  if err := d.loadKind(ctx, kvGrant, func(row persist.Row) error {
		  var g Grant
		   if err := json.Unmarshal(row.Body, &g); err != nil {
			  return err
		  }
		   d.grants[g.Idem] = &g
		  d.grantFence[g.Idem] = row.Fence
		   return nil
	   }); err != nil {
		  d.halted = err
		   return err
	   }
	  if err := d.loadKind(ctx, kvAudit, func(row persist.Row) error {
		  var rows []Audit
		  if err := json.Unmarshal(row.Body, &rows); err != nil {
			   return err
		  }
		  d.audits[row.Key] = rows
		  d.auditFence[row.Key] = row.Fence
		  return nil
	  }); err != nil {
		   d.halted = err
		  return err
	  }
	   d.halted = nil
	  d.hydrated = true
	   return nil
   }

   func (d *Desk) loadKind(ctx context.Context, kind string, fn func(persist.Row) error) error {
	  after := ""
	   for {
		   rows,
			  err := d.kv.ListAfter(ctx, kind, after, 200)
		  if err != nil {
			   return err
		   }
		   if len(rows) == 0 {
			   return nil
		   }
		   for _, row := range rows {
			  if err := fn(row); err != nil {
				   return err
			  }
			   if row.Key > after {
				  after = row.Key
			   }
		   }
		  if len(rows) < 200 {
			  return nil
		  }
	   }
  }

   func (d *Desk) rebuildHold(t *Ticket) {
	   total, err := decimal.NewFromString(t.Amount)
	   if err != nil {
		  return
	  }
	  frz := frozenDecimal(t)
	  left := total.Sub(frz)
	  if left.IsNegative() {
		  left = decimal.Zero
	  }
	   uk, dk := dayKey(t.UserID, t.Asset, t.Dest, t.CreatedAt)
	   d.keyUser[t.ID] = uk
	   d.keyDest[t.ID] = dk
	  add := func(key string, reserved, committed decimal.Decimal) {
		  if reserved.IsPositive() {
			   d.holds.reserved[key] = d.holds.reserved[key].Add(reserved)
		  }
		  if committed.IsPositive() {
			  d.holds.committed[key] = d.holds.committed[key].Add(committed)
		   }
	  }
	  switch t.State {
	  case StateRejected, StateExpired:
		  add(uk, decimal.Zero, frz)
		   add(dk, decimal.Zero, frz)
	  default:
		   add(uk, left, frz)
		   add(dk, left, frz)
	   }
  }
