package destallow

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindCoolPolicy = "destallow.policy"

type CoolPolicy struct {
	Seconds int64 `json:"seconds"`
}

// BindRuntime 挂 KV 并回放冷却策略。无落库则保留现有 cooldown 再写回。

   func BindRuntime(book *Book, kv persist.KV) *Book {
	  if book == nil {
		   return book
	  }
	  book.BindKV(kv)
	  book.LoadPolicy(context.Background())
	   return book
   }

  func (b *Book) BindKV(kv persist.KV) *Book {
	   if b != nil && kv != nil {
		   b.kv = kv
	  }
	  return b
  }

  func (b *Book) LoadPolicy(ctx context.Context) {
	  if b == nil || b.kv == nil {
		  return
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  row, err := b.kv.Get(ctx, kindCoolPolicy, "cooldown")
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) && b.cooldown > 0 {
			  _ = b.saveCooldown(ctx)
		  }
		  return
	   }
	   var p CoolPolicy
	  if json.Unmarshal(row.Body, &p) != nil || p.Seconds < 0 {
		   return
	  }
	  b.SetCooldown(time.Duration(p.Seconds) * time.Second)
   }

  func (b *Book) saveCooldown(ctx context.Context) error {
	   if b == nil || b.kv == nil {
		  return nil
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   body, err := json.Marshal(CoolPolicy{Seconds: int64(b.Cooldown() / time.Second)})
	  if err != nil {
		  return err
	  }
	   _, existed, err := persist.GetOrInit(ctx, b.kv, kindCoolPolicy, "cooldown", body)
	   if err != nil {
		   return err
	   }
	   if existed {
		  return persist.Replace(ctx, b.kv, kindCoolPolicy, "cooldown", body)
	   }
	   return nil
   }

// PersistCooldown 热更新并落库冷却秒数。

  func (b *Book) PersistCooldown(ctx context.Context, d time.Duration) error {
	   if b == nil {
		  return ErrMissing
	   }
	  if d < 0 {
		  d = 0
	  }
	   b.SetCooldown(d)
	   return b.saveCooldown(ctx)
   }

  func (b *Book) Cooldown() time.Duration {
	  if b == nil {
		   return 0
	  }
	   return b.cooldown
  }

  func (b *Book) kvOn() bool {
	   return b != nil && b.kv != nil
  }
