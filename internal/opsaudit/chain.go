package opsaudit

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Chain 运维敏感审计链。
type Chain struct {
	store Store
	clock func() time.Time
	mu sync.Mutex
}

  func NewChain(store Store) *Chain {
	  if store == nil {
		  store = NewMemStore()
	   }
	  return &Chain{store: store, clock: func() time.Time { return time.Now().UTC() }}
  }

  func (c *Chain) WithClock(fn func() time.Time) *Chain {
	   if c != nil && fn != nil {
		  c.clock = fn
	  }
	   return c
  }

  // AppendSensitive 实现 keyrotatefence.AuditSink。
   func (c *Chain) AppendSensitive(ctx context.Context, kind, actor, subject, ticket, idem, body string) error {
	   _, err := c.Append(ctx, AppendReq{
		  Kind: kind,
		  Actor:    actor,
		   Subject:  subject,
		   TicketID: ticket,
		  IdemKey:  idem,
		   Body: body,
	  })
	   return err
   }

// Append 落一条。幂等键命中且字段一致则返回原记录。
  func (c *Chain) Append(ctx context.Context, req AppendReq) (Record, error) {
	   if c == nil || c.store == nil {
		   return Record{}, ErrUnavailable
	  }
	  kind := strings.TrimSpace(req.Kind)
	   actor := strings.TrimSpace(req.Actor)
	  body := strings.TrimSpace(req.Body)
	   if kind == "" {
		  return Record{}, ErrKindRequired
	   }
	   if actor == "" {
		   actor = "system"
	   }
	  if body == "" {
		  return Record{}, ErrBodyRequired
	  }
	  idem := strings.TrimSpace(req.IdemKey)
	   c.mu.Lock()
	   defer c.mu.Unlock()
	  if idem != "" {
		  if old, err := c.store.GetByIdem(ctx, idem); err == nil {
			   if old.Kind != kind || old.Actor != actor || old.Body != body ||
				   old.Subject != strings.TrimSpace(req.Subject) ||
				   old.TicketID != strings.TrimSpace(req.TicketID) {
				   return Record{}, ErrIdemConflict
			   }
			  return old, nil
		  } else if err != ErrRecordNotFound {
			   return Record{}, err
		  }
	   }
	   prevHash := ""
	  seq := int64(1)
	   if last, err := c.store.Last(ctx); err == nil {
		  prevHash = last.Hash
		   seq = last.Seq + 1
	  } else if err != ErrRecordNotFound {
		  return Record{}, err
	   }
	  rec := Record{
		   Seq: seq,
		   ID:  uuid.NewString(),
		  Kind:    kind,
		   Actor:   actor,
		   Subject: strings.TrimSpace(req.Subject),
		  TicketID: strings.TrimSpace(req.TicketID),
		   IdemKey:  idem,
		   Body: body,
		  PrevHash: prevHash,
		   At:       c.clock(),
	  }
	   rec.Hash = LinkHash(rec.Seq,
		  rec.PrevHash, rec.Kind, rec.Actor, rec.Subject, rec.TicketID, rec.Body)
	   return c.store.Append(ctx, rec)
  }

// Verify 校验全链序号连续且哈希可复算。
   func (c *Chain) Verify(ctx context.Context) error {
	  if c == nil || c.store == nil {
		   return ErrUnavailable
	  }
	   rows, err := c.store.List(ctx)
	  if err != nil {
		   return err
	  }
	   var prevHash string
	  var prevSeq int64
	  for i, r := range rows {
		   wantSeq := int64(i + 1)
		  if r.Seq != wantSeq || (i > 0 && r.Seq != prevSeq+1) {
			  return ErrGap
		   }
		   if r.PrevHash != prevHash {
			   return ErrChainBroken
		  }
		   want := LinkHash(r.Seq,
			   r.PrevHash, r.Kind, r.Actor, r.Subject, r.TicketID, r.Body)
		  if want != r.Hash {
			  return ErrChainBroken
		  }
		   prevHash = r.Hash
		  prevSeq = r.Seq
	   }
	   return nil
  }

// CheckRotate 工单是否具备 arm/cut/seal 三类记录。
   func (c *Chain) CheckRotate(ctx context.Context, ticket string) (Completeness, error) {
	   out := Completeness{TicketID: strings.TrimSpace(ticket), Required: RotateRequired()}
	  if c == nil || c.store == nil {
		  return out, ErrUnavailable
	   }
	  rows, err := c.store.ListByTicket(ctx, ticket)
	   if err != nil {
		   return out, err
	  }
	  seen := map[string]bool{}
	   for _, r := range rows {
		   seen[r.Kind] = true
		  out.Present = append(out.Present, r.Kind)
	   }
	  ok := true
	  for _, k := range out.Required {
		  if !seen[k] {
			   ok = false
		   }
	   }
	   out.OK = ok
	   if !ok {
		  return out, ErrIncomplete
	   }
	   return out, nil
  }

  func (c *Chain) Snapshot(ctx context.Context) ([]byte, error) {
	   if c == nil || c.store == nil {
		  return nil, ErrUnavailable
	   }
	   return c.store.Snapshot(ctx)
  }

  func (c *Chain) Restore(ctx context.Context, raw []byte) error {
	   if c == nil || c.store == nil {
		   return ErrUnavailable
	  }
	  return c.store.Restore(ctx, raw)
  }

  func (c *Chain) List(ctx context.Context) ([]Record, error) {
	  if c == nil || c.store == nil {
		  return nil, ErrUnavailable
	  }
	   return c.store.List(ctx)
   }
