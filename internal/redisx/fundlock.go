package redisx

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/slo"
)

const (
	KeyFundLockUser = "aa:funding:flock:{%s}:user"
	KeyFundLockAsset = "aa:funding:flock:{%s}:asset:%s"
	KeyFundLockWD    = "aa:funding:flock:{%s}:wd:%s"
	KeyFundLockDep     = "aa:funding:flock:{%s}:dep:%s"
	KeyIdempotency     = "aa:funding:idem:{%s}:%s"
	KeyOutboxLeader    = "aa:funding:worker:leader:outbox"
	DefaultFundLockTTL = 3 * time.Second
)

// FundUserLockKey 用户资金总锁（hash tag=userID）。

  func FundUserLockKey(userID string) string {
	   return fmt.Sprintf(KeyFundLockUser, userID)
   }

// FundAssetLockKey 用户+资产锁。

   func FundAssetLockKey(userID, asset string) string {
	  return fmt.Sprintf(KeyFundLockAsset, userID, asset)
   }

// FundWithdrawLockKey 出金单锁。
  func FundWithdrawLockKey(userID, withdrawID string) string {
	  return fmt.Sprintf(KeyFundLockWD, userID, withdrawID)
   }

// FundDepositLockKey 入金观察锁。
  func FundDepositLockKey(userID, depositID string) string {
	   return fmt.Sprintf(KeyFundLockDep, userID, depositID)
   }

// IdempotencyKey 幂等键（短缓存，非权威）。
   func IdempotencyKey(scope, key string) string {
	   return fmt.Sprintf(KeyIdempotency,
		   scope, key)
   }

// WithFundLock 持锁执行；争抢失败返回 ErrLockHeld。

  func WithFundLock(ctx context.Context, c Client, key string, ttl time.Duration, fn func(context.Context) error) error {
	  if c == nil {
		   return ErrUnavailable
	   }
	  if fn == nil {
		   return fmt.Errorf("%w: fn required", ErrUnavailable)
	  }
	   if ttl <= 0 {
		  ttl = DefaultFundLockTTL
	  }
	  if _, ok := ctx.Deadline(); !ok {
		   var cancel context.CancelFunc
		   ctx, cancel = context.WithTimeout(ctx, slo.RedisLockTimeout+ttl)
		  defer cancel()
	  }
	  tok, err := c.TryLock(ctx, key, ttl)
	  if err != nil {
		   return err
	   }
	  defer func() {
		   _ = c.Unlock(context.Background(),
			   key, tok)
	  }()
	return fn(ctx)
}

// WithUserFundLock 用户总锁便捷封装。

   func WithUserFundLock(ctx context.Context, c Client, userID string, fn func(context.Context) error) error {
	   return WithFundLock(ctx,
		c, FundUserLockKey(userID), DefaultFundLockTTL, fn)
}

// KeyCatalog 资金域键前缀清单（运维排查）。
   func KeyCatalog() []string {
	  return []string{
		  KeyPrefix,
		  KeyWorkerLeader,
		   KeyOutboxLag,
		  KeyBalanceSnap,
		   KeyUserLock,
		  KeyOutboxLeader,
		   "aa:funding:flock:",
		   "aa:funding:idem:",
	  }
   }
