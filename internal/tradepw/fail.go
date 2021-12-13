package tradepw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindFail   = "aa_trade_pw_fail"
	defaultMax = 5
	defaultTTL = 30 * time.Minute
)

var (
	ErrLocked = errors.New("trading password locked after failures")
)

type failRec struct {
	Count int `json:"count"`
	LockedUntil int64 `json:"locked_until"`
	UpdatedAt   int64 `json:"updated_at"`
}

// FailPolicy 错密锁定策略；零值用默认 5 次 / 30 分钟。
type FailPolicy struct {
	MaxFails int
	LockFor time.Duration
}

   func (b *Book) BindFailPolicy(p FailPolicy) *Book {
	   if b == nil {
		  return b
	   }
	  if p.MaxFails > 0 {
		   b.maxFails = p.MaxFails
	   }
	  if p.LockFor > 0 {
		   b.lockTTL = p.LockFor
	   }
	   return b
   }

   func (b *Book) policy() (int, time.Duration) {
	  max, ttl := defaultMax, defaultTTL
	  if b != nil {
		   if b.maxFails > 0 {
			  max = b.maxFails
		  }
		  if b.lockTTL > 0 {
			  ttl = b.lockTTL
		   }
	  }
	  return max, ttl
   }

// Locked 查询错密锁是否生效；返回剩余秒数与原因。

   func (b *Book) Locked(ctx context.Context, userID string) (bool, int64, error) {
	  rec, err := b.loadFail(ctx, userID)
	   if err != nil {
		  return false, 0, err
	  }
	   now := time.Now().UTC().Unix()
	   if rec.LockedUntil > now {
		  return true,
			  rec.LockedUntil - now, nil
	   }
	  return false, 0, nil
   }

   func (b *Book) assertUnlocked(ctx context.Context, userID string) error {
	  ok,
		   remain, err := b.Locked(ctx, userID)
	  if err != nil {
		  return err
	   }
	   if ok {
		  return fmt.Errorf("%w: remain_sec=%d", ErrLocked, remain)
	   }
	   return nil
   }

   func (b *Book) noteFail(ctx context.Context, userID string) error {
	   if b == nil || b.kv == nil {
		  return nil
	   }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return ErrUser
	  }
	   max, ttl := b.policy()
	   rec,
		   err := b.loadFail(ctx, userID)
	  if err != nil {
		   return err
	   }
	  now := time.Now().UTC().Unix()
	   if rec.LockedUntil > now {
		  return fmt.Errorf("%w: remain_sec=%d", ErrLocked, rec.LockedUntil-now)
	   }
	  if rec.LockedUntil > 0 && rec.LockedUntil <= now {
		  rec = failRec{}
	  }
	   rec.Count++
	  rec.UpdatedAt = now
	   if rec.Count >= max {
		   rec.LockedUntil = now + int64(ttl.Seconds())
		   rec.Count = 0
	   }
	  return b.writeFail(ctx, userID, rec)
   }

   func (b *Book) clearFail(ctx context.Context, userID string) error {
	  if b == nil || b.kv == nil {
		  return nil
	   }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return nil
	   }
	   cur, err := b.kv.Get(ctx, kindFail, userID)
	   if errors.Is(err, persist.ErrNotFound) {
		   return nil
	  }
	  if err != nil {
		   return err
	   }
	  body,
		  _ := json.Marshal(failRec{UpdatedAt: time.Now().UTC().Unix()})
	   return b.kv.Put(ctx, persist.Row{Kind: kindFail, Key: userID, Body: body}, cur.Fence)
  }

  func (b *Book) loadFail(ctx context.Context, userID string) (failRec, error) {
	  if b == nil || b.kv == nil {
		  return failRec{}, nil
	   }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return failRec{}, ErrUser
	   }
	  row, err := b.kv.Get(ctx, kindFail, userID)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return failRec{}, nil
		  }
		   return failRec{}, err
	  }
	   var rec failRec
	   _ = json.Unmarshal(row.Body,
		  &rec)
	  return rec, nil
  }

  func (b *Book) writeFail(ctx context.Context, userID string, rec failRec) error {
	  body, err := json.Marshal(rec)
	   if err != nil {
		  return err
	  }
	   cur, err := b.kv.Get(ctx, kindFail, userID)
	   exp := persist.Fence{}
	  if err == nil {
		  exp = cur.Fence
	  } else if !errors.Is(err, persist.ErrNotFound) {
		  return err
	  }
	  return b.kv.Put(ctx,
		  persist.Row{Kind: kindFail, Key: userID, Body: body}, exp)
   }
