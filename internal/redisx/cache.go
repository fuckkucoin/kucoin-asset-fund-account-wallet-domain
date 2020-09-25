package redisx

import (
	"context"
	"time"
)

const defaultBalanceTTL = 3 * time.Second

// GetBalanceSnap 读余额快照。
   func GetBalanceSnap(ctx context.Context, c Client, userID, asset string) ([]byte, error) {
	   if c == nil {
		   return nil, ErrUnavailable
	  }
	   return c.Get(ctx, BalanceKey(userID, asset))
   }

// SetBalanceSnap 写余额快照。

  func SetBalanceSnap(ctx context.Context, c Client, userID, asset string, val []byte, ttl time.Duration) error {
	  if c == nil {
		  return ErrUnavailable
	   }
	  if ttl <= 0 {
		  ttl = defaultBalanceTTL
	  }
	  return c.Set(ctx, BalanceKey(userID, asset), val, ttl)
   }

// InvalidateBalance 删快照，写路径后调用。
  func InvalidateBalance(ctx context.Context, c Client, userID, asset string) error {
	  if c == nil {
		   return ErrUnavailable
	   }
	  return c.Del(ctx, BalanceKey(userID, asset))
  }

// WithUserLock 在用户锁内执行 fn；锁失败即返回，不跳过。
  func WithUserLock(ctx context.Context, c Client, userID string, ttl time.Duration, fn func(context.Context) error) error {
	  if c == nil {
		   return ErrUnavailable
	   }
	   if fn == nil {
		  return nil
	  }
	  if ttl <= 0 {
		   ttl = 5 * time.Second
	   }
	  tok,
		   err := c.TryLock(ctx, UserLockKey(userID), ttl)
	  if err != nil {
		   return err
	  }
	  defer func() { _ = c.Unlock(context.Background(), UserLockKey(userID), tok) }()
	return fn(ctx)
}
