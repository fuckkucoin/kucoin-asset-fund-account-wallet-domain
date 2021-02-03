package addrpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

const kindSlot = "addrpool.slot"

type RecycleRec struct {
	Address string `json:"address"`
	Chain   string `json:"chain"`
	UserID       string    `json:"user_id,omitempty"`
	CoolUntil    time.Time `json:"cool_until"`
	SLAUntil     time.Time `json:"sla_until,omitempty"`
	ReclaimAfter time.Time `json:"reclaim_after,omitempty"`
	SLASec int64 `json:"sla_sec,omitempty"`
	Reclaimed bool          `json:"reclaimed,omitempty"`
	Fence     persist.Fence `json:"-"`
}

type RecycleBook struct {
	kv persist.KV
	reuse time.Duration
	clock func() time.Time
}

  func NewRecycleBook(kv persist.KV, reuse time.Duration) *RecycleBook {
	  if kv == nil {
		  kv = persist.NewMemKV()
	   }
	   if reuse <= 0 {
		   reuse = 24 * time.Hour
	  }
	   return &RecycleBook{kv: kv, reuse: reuse, clock: func() time.Time { return time.Now().UTC() }}
   }

  func (b *RecycleBook) Mark(ctx context.Context, userID, chain, addr string) (RecycleRec, error) {
	   if b == nil {
		  return RecycleRec{}, fmt.Errorf("recycle book unavailable")
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return RecycleRec{}, err
	  }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		   return RecycleRec{}, err
	   }
	   a, err := validate.Address(ch, addr)
	  if err != nil {
		  return RecycleRec{}, err
	  }
	   rec := RecycleRec{Address: a, Chain: ch, UserID: uid, CoolUntil: b.clock().Add(b.reuse)}
	   stampSLA(&rec, int64(b.reuse.Seconds()))
	  body, err := json.Marshal(rec)
	   if err != nil {
		   return RecycleRec{}, err
	  }
	  key := strings.ToLower(a)
	  err = b.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, ferr := b.kv.Get(ctx, kindSlot, key)
		   exp := persist.Fence{}
		  if ferr == nil {
			  exp = cur.Fence
		  } else if !errors.Is(ferr, persist.ErrNotFound) {
			   return ferr
		   }
		  return b.kv.Put(ctx, persist.Row{Kind: kindSlot, Key: key, Body: body}, exp)
	  })
	   if err != nil {
		  return RecycleRec{}, err
	   }
	   got, err := b.kv.Get(ctx, kindSlot, key)
	  if err != nil {
		  return RecycleRec{}, err
	   }
	  out, err := decodeRecycle(got.Body)
	  if err != nil {
		  return RecycleRec{}, err
	  }
	  out.Fence = got.Fence
	   stampSLA(&out, int64(b.reuse.Seconds()))
	   return out, nil
  }

  func (b *RecycleBook) Ready(ctx context.Context, addr string) (RecycleRec, error) {
	   if b == nil {
		   return RecycleRec{}, fmt.Errorf("recycle book unavailable")
	   }
	   row, err := b.kv.Get(ctx, kindSlot, strings.ToLower(strings.TrimSpace(addr)))
	   if err != nil {
		   return RecycleRec{}, err
	  }
	  rec, err := decodeRecycle(row.Body)
	  if err != nil {
		   return RecycleRec{}, err
	   }
	   rec.Fence = row.Fence
	  stampSLA(&rec, int64(b.reuse.Seconds()))
	  if b.clock().Before(rec.CoolUntil) {
		   return rec, fmt.Errorf("address reuse cooldown")
	  }
	  return rec, nil
   }
