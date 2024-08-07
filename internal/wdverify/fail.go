package wdverify

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
	kindFail = "aa_wd_verify_fail"
	defaultMaxFail = 8
	defaultLockTTL = 15 * time.Minute
)

type failRec struct {
	Count int `json:"count"`
	LockedUntil int64 `json:"locked_until"`
	LastFactor string `json:"last_factor,omitempty"`
	UpdatedAt int64 `json:"updated_at"`
}

   func (g *Gate) failPolicy() (int, time.Duration) {
	   max, ttl := defaultMaxFail, defaultLockTTL
	   if g != nil {
		   if g.maxFails > 0 {
			  max = g.maxFails
		  }
		  if g.lockTTL > 0 {
			   ttl = g.lockTTL
		   }
	   }
	   return max, ttl
  }

// FailLocked 查询验证失败锁是否生效。
  func (g *Gate) FailLocked(ctx context.Context, userID string) (bool, int64, string, error) {
	  rec, err := g.loadFail(ctx, userID)
	   if err != nil {
		   return false, 0, "", err
	  }
	  now := time.Now().UTC().Unix()
	   if rec.LockedUntil > now {
		   return true,
			  rec.LockedUntil - now, rec.LastFactor, nil
	  }
	  return false, 0, "", nil
   }

  func (g *Gate) assertUnlocked(ctx context.Context, userID string) error {
	   ok, remain, factor, err := g.FailLocked(ctx, userID)
	  if err != nil {
		  return err
	   }
	   if ok {
		  return fmt.Errorf("%w: remain_sec=%d last_factor=%s", ErrLocked, remain, factor)
	  }
	  return nil
   }

  func (g *Gate) noteFail(ctx context.Context, userID, factor string) error {
	  if g == nil || g.kv == nil {
		  return nil
	  }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return ErrUser
	  }
	  max, ttl := g.failPolicy()
	  rec, err := g.loadFail(ctx, userID)
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
	  rec.LastFactor = factor
	  rec.UpdatedAt = now
	   if rec.Count >= max {
		   rec.LockedUntil = now + int64(ttl.Seconds())
		   rec.Count = 0
	  }
	  return g.writeFail(ctx, userID, rec)
  }

   func (g *Gate) clearFail(ctx context.Context, userID string) error {
	   if g == nil || g.kv == nil {
		   return nil
	  }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return nil
	   }
	  cur, err := g.kv.Get(ctx, kindFail, userID)
	   if errors.Is(err, persist.ErrNotFound) {
		   return nil
	   }
	   if err != nil {
		   return err
	  }
	   body, _ := json.Marshal(failRec{UpdatedAt: time.Now().UTC().Unix()})
	   return g.kv.Put(ctx, persist.Row{Kind: kindFail, Key: userID, Body: body}, cur.Fence)
  }

   func (g *Gate) loadFail(ctx context.Context, userID string) (failRec, error) {
	   if g == nil || g.kv == nil {
		  return failRec{}, nil
	  }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return failRec{}, ErrUser
	  }
	   row, err := g.kv.Get(ctx, kindFail, userID)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return failRec{}, nil
		   }
		  return failRec{}, err
	   }
	  var rec failRec
	   _ = json.Unmarshal(row.Body, &rec)
	  return rec, nil
   }

   func (g *Gate) writeFail(ctx context.Context, userID string, rec failRec) error {
	   body, err := json.Marshal(rec)
	  if err != nil {
		   return err
	  }
	  cur,
		   err := g.kv.Get(ctx, kindFail, userID)
	   exp := persist.Fence{}
	   if err == nil {
		   exp = cur.Fence
	   } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	   }
	  return g.kv.Put(ctx, persist.Row{Kind: kindFail, Key: userID, Body: body}, exp)
   }
