package addrpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// stampSLA 补回收 SLA 字段。SLAUntil 与 CoolUntil 一致。

  func stampSLA(rec *RecycleRec, reuse int64) {
	   if rec == nil {
		  return
	   }
	   rec.SLAUntil = rec.CoolUntil
	   rec.ReclaimAfter = rec.CoolUntil
	   rec.SLASec = reuse
   }

// Reclaim 冷却满后回收槽位。删 KV 冷却键，标 Reclaimed。
   func (b *RecycleBook) Reclaim(ctx context.Context, addr string) (RecycleRec, error) {
	  if b == nil {
		   return RecycleRec{}, fmt.Errorf("recycle book unavailable")
	   }
	  rec, err := b.Ready(ctx, addr)
	   if err != nil {
		   return rec, err
	   }
	  key := strings.ToLower(strings.TrimSpace(addr))
	  err = b.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, ferr := b.kv.Get(ctx, kindSlot, key)
		  if ferr != nil {
			  return ferr
		  }
		   return b.kv.Delete(ctx, kindSlot, key, cur.Fence)
	   })
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  rec.Reclaimed = true
			  stampSLA(&rec, int64(b.reuse.Seconds()))
			   return rec, nil
		   }
		  return rec, err
	  }
	   rec.Reclaimed = true
	  stampSLA(&rec, int64(b.reuse.Seconds()))
	  return rec, nil
   }

// ReclaimUser 属主核对后回收一对一地址。

   func ReclaimUser(ctx context.Context, pool *Pool, rec *RecycleBook, userID, chain, addr string) (RecycleRec, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return RecycleRec{}, err
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return RecycleRec{}, err
	   }
	   if wallet.RequiresFamMemo(ch) {
		  return RecycleRec{}, ErrSharedRelease
	  }
	  a := strings.TrimSpace(addr)
	   if a == "" && pool != nil {
		   if b, ok := pool.Lookup(uid, ch); ok {
			  a = b.Address
		   }
	  }
	   a, err = validate.Address(ch, a)
	   if err != nil {
		   return RecycleRec{}, err
	   }
	  if pool != nil {
		  if owner, ok := pool.ByAddress(a); ok && owner.UserID != uid {
			   return RecycleRec{}, ErrAssignOwner
		  }
	  }
	   if rec == nil {
		  rec = NewRecycleBook(nil, 0)
	  }
	   got, err := rec.Reclaim(ctx, a)
	   if err != nil {
		  return got, err
	  }
	   got.UserID = uid
	  got.Chain = ch
	  got.Address = a
	  return got, nil
  }

  func decodeRecycle(body []byte) (RecycleRec, error) {
	  var rec RecycleRec
	  if err := json.Unmarshal(body, &rec); err != nil {
		   return RecycleRec{}, err
	  }
	  return rec, nil
   }
