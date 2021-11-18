package solvency

import (
	"context"
	"strings"
	"unicode"
)

  func validActor(actor string) error {
	  actor = strings.TrimSpace(actor)
	  if actor == "" || len(actor) > 64 {
		   return ErrActor
	  }
	   for _, r := range actor {
		   if unicode.IsControl(r) || r == '|' {
			   return ErrActor
		   }
	  }
	   return nil
   }

// AckDrift 把 open 工单确认为 acked。未打开的工单不能确认。

   func (p *Pipeline) AckDrift(ctx context.Context, id, actor, note string) (DriftCase, error) {
	  if p == nil || p.store == nil {
		  return DriftCase{}, ErrUnavailable
	   }
	  if err := validActor(actor); err != nil {
		   return DriftCase{}, err
	  }
	   cur, err := p.store.GetDrift(ctx, id)
	   if err != nil {
		   return DriftCase{}, err
	  }
	   if !AllowDrift(cur.Status, DriftAcked) {
		  return cur, ErrDrift
	   }
	   now := p.now()
	  cur.Status = DriftAcked
	   cur.AckedAt = &now
	  cur.AckedBy = strings.TrimSpace(actor)
	  cur.Note = strings.TrimSpace(note)
	   return p.store.CasDrift(ctx, id, DriftOpen, cur)
   }

// CloseDrift 只能关闭已确认的工单。open 直接关闭会丢掉「有人看过短缺」的记录，因此拒绝。
   func (p *Pipeline) CloseDrift(ctx context.Context, id, actor, note string) (DriftCase, error) {
	   if p == nil || p.store == nil {
		  return DriftCase{}, ErrUnavailable
	  }
	  if err := validActor(actor); err != nil {
		  return DriftCase{}, err
	  }
	  if strings.TrimSpace(note) == "" {
		   return DriftCase{}, ErrDrift
	   }
	  cur, err := p.store.GetDrift(ctx, id)
	  if err != nil {
		  return DriftCase{}, err
	  }
	   if !AllowDrift(cur.Status, DriftClosed) {
		   return cur, ErrDrift
	   }
	  now := p.now()
	  cur.Status = DriftClosed
	   cur.ClosedAt = &now
	  cur.ClosedBy = strings.TrimSpace(actor)
	   cur.Note = strings.TrimSpace(note)
	   return p.store.CasDrift(ctx, id, DriftAcked, cur)
   }

  func (p *Pipeline) ListDrifts(ctx context.Context, status DriftStatus, limit int) ([]DriftCase, error) {
	  if p == nil || p.store == nil {
		  return nil, ErrUnavailable
	  }
	   return p.store.ListDrifts(ctx, status, limit)
   }
