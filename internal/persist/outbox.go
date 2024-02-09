// 通用持久化辅助之出箱投递。

// Author: Axel
// Date: 2020.09.06

package persist

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"github.com/google/uuid"
)

const kindOutbox = "outbox"

// OutboxEvent 事务内出箱。未 Mark 不得当已投递。

type OutboxEvent struct {
	ID  string `json:"id"`
	Key string `json:"key"`
	Kind      string    `json:"kind"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
	SentAt *time.Time `json:"sent_at,omitempty"`
}

type Outbox struct {
	kv    KV
	clock func() time.Time
}

  func NewOutbox(kv KV) *Outbox {
	   if kv == nil {
		   kv = NewMemKV()
	  }
	  return &Outbox{kv: kv, clock: func() time.Time { return time.Now().UTC() }}
  }

  func (o *Outbox) WithClock(c func() time.Time) *Outbox {
	  if o != nil && c != nil {
		   o.clock = c
	  }
	   return o
  }

  func (o *Outbox) KV() KV { return o.kv }

// Enqueue 与业务写同事务。key/kind/payload 空拒。

   func (o *Outbox) Enqueue(ctx context.Context, key, kind, payload string) (OutboxEvent, error) {
	  if o == nil || o.kv == nil {
		   return OutboxEvent{}, ErrTxClosed
	   }
	  key = strings.TrimSpace(key)
	   kind = strings.TrimSpace(kind)
	  if key == "" || kind == "" || strings.TrimSpace(payload) == "" {
		  return OutboxEvent{}, ErrConflict
	  }
	   ev := OutboxEvent{
		   ID: uuid.NewString(), Key: key, Kind: kind, Payload: payload, CreatedAt: o.clock(),
	  }
	   body, err := json.Marshal(ev)
	  if err != nil {
		  return OutboxEvent{}, err
	   }
	  if err := o.kv.Put(ctx, Row{Kind: kindOutbox, Key: ev.ID, Body: body}, Fence{}); err != nil {
		  return OutboxEvent{}, err
	   }
	  return ev, nil
   }

   func (o *Outbox) Pending(ctx context.Context, limit int) ([]OutboxEvent, error) {
	   if o == nil || o.kv == nil {
		   return nil, ErrTxClosed
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	   rows, err := o.kv.List(ctx, kindOutbox, limit*4)
	  if err != nil {
		  return nil, err
	  }
	  out := make([]OutboxEvent,
		   0, limit)
	   for _, r := range rows {
		  var ev OutboxEvent
		  if err := json.Unmarshal(r.Body, &ev); err != nil {
			   continue
		   }
		  if ev.SentAt != nil {
			   continue
		  }
		  out = append(out, ev)
		  if len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
  }

// Mark 按当前围栏把 SentAt 写上。旧 token 失败，调用方须重读再投。

   func (o *Outbox) Mark(ctx context.Context, id string) error {
	   if o == nil || o.kv == nil {
		   return ErrTxClosed
	   }
	  id = strings.TrimSpace(id)
	   if id == "" {
		   return ErrNotFound
	   }
	  return o.kv.InTx(ctx, func(ctx context.Context) error {
		   row, err := o.kv.Get(ctx, kindOutbox, id)
		   if err != nil {
			   return err
		  }
		  var ev OutboxEvent
		   if err := json.Unmarshal(row.Body, &ev); err != nil {
			  return ErrConflict
		  }
		   if ev.SentAt != nil {
			   return nil
		   }
		  now := o.clock()
		   ev.SentAt = &now
		  body, err := json.Marshal(ev)
		   if err != nil {
			   return err
		   }
		  row.Body = body
		  return o.kv.Put(ctx, row, row.Fence)
	  })
   }

   func (o *Outbox) Get(ctx context.Context, id string) (OutboxEvent, error) {
	   row, err := o.kv.Get(ctx, kindOutbox, strings.TrimSpace(id))
	   if err != nil {
		  return OutboxEvent{}, err
	  }
	   var ev OutboxEvent
	   if err := json.Unmarshal(row.Body, &ev); err != nil {
		  return OutboxEvent{},
			   ErrConflict
	   }
	  return ev, nil
   }
