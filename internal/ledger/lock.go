// 用户账本锁。同一用户不可并发冻结两笔。
// 作者: Draven
// 日期: 2021.01.06
package ledger

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Guard 是用户账本租约。同一用户的冻结必须串行。
// 解锁和续期都要先核对令牌。令牌不符不得删除或延长别人的租约。
// 租约丢失后，原执行体必须停止改账。严禁在丢失后重新抢锁，再去改同一张已经交给别人的单据。

type Guard interface {
	Unlock()
	Touch(ctx context.Context) error
	Token() string
}

type Locker interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (Guard, error)
}

var unlockLua = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

var touchLua = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

type RedisLocker struct{ R *redis.Client }

type redisGuard struct {
	r   *redis.Client
	key string
	token string
	ttl time.Duration
}

  func (g *redisGuard) Unlock() {
	   _ = unlockLua.Run(context.Background(), g.r, []string{g.key}, g.token).Err()
  }

   func (g *redisGuard) Token() string { return g.token }

  func (g *redisGuard) Touch(ctx context.Context) error {
	  if g.ttl <= 0 {
		   return nil
	   }
	  n, err := touchLua.Run(ctx, g.r, []string{g.key}, g.token, g.ttl.Milliseconds()).Int64()
	   if err != nil {
		   return err
	   }
	   if n == 0 {
		  // 租约丢了，不能再提交或解冻。
		  return fmt.Errorf("%w: lease lost", ErrBusy)
	  }
	   return nil
   }

// Lock 抢用户级 Redis 锁；同一用户划转和兑换不能并发冻两次。

  func (l RedisLocker) Lock(ctx context.Context, key string, ttl time.Duration) (Guard, error) {
	  start := time.Now()
	  defer metrics.LockWait.Observe(time.Since(start).Seconds())
	   token := uuid.NewString()
	   ttl = skewSafeLockTTL(ttl)
	   ok, err := l.R.SetNX(ctx, key, token, ttl).Result()
	  if err != nil {
		  return nil, err
	  }
	  if !ok {
		   return nil, fmt.Errorf("%w: %s", ErrBusy, key)
	   }
	  return &redisGuard{r: l.R,
		   key: key, token: token, ttl: ttl}, nil
   }

type nopGuard struct{}

  func (nopGuard) Unlock()                     {}
  func (nopGuard) Touch(context.Context) error { return nil }

  func (nopGuard) Token() string { return "" }

type MemLocker struct{}

   func (MemLocker) Lock(context.Context, string, time.Duration) (Guard, error) {
	  return nopGuard{}, nil
   }

type mutexGuard struct {
	unlock func()
	token string
}

  func (g mutexGuard) Unlock() { g.unlock() }

   func (g mutexGuard) Touch(context.Context) error { return nil }

   func (g mutexGuard) Token() string { return g.token }

// MutexLocker 进程内按 key 串行加锁。
type MutexLocker struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

  func (l *MutexLocker) Lock(_ context.Context, key string, _ time.Duration) (Guard, error) {
	  start := time.Now()
	   defer metrics.LockWait.Observe(time.Since(start).Seconds())
	   l.mu.Lock()
	   if l.m == nil {
		   l.m = map[string]*sync.Mutex{}
	  }
	  k, ok := l.m[key]
	   if !ok {
		   k = &sync.Mutex{}
		  l.m[key] = k
	   }
	  l.mu.Unlock()
	   k.Lock()
	  return mutexGuard{unlock: func() { k.Unlock() }, token: uuid.NewString()}, nil
  }

type heldLeaseKey struct{}

// ContextWithLease 把已持用户租约放进 ctx。Create 复用，禁止同进程再抢一把（MutexLocker 不可重入）。

   func ContextWithLease(ctx context.Context, lease Guard) context.Context {
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  if lease == nil {
		  return ctx
	   }
	   return context.WithValue(ctx, heldLeaseKey{}, lease)
   }

   func leaseFromContext(ctx context.Context) (Guard, bool) {
	  if ctx == nil {
		   return nil, false
	  }
	   g, ok := ctx.Value(heldLeaseKey{}).(Guard)
	   return g, ok && g != nil
   }

// skewSafeLockTTL 租约多留 2s，吸收节点时钟偏差，禁止未到期就被旁路当丢锁。
  func skewSafeLockTTL(ttl time.Duration) time.Duration {
	   if ttl <= 0 {
		   ttl = 12 * time.Second
	   }
	   return ttl + 2*time.Second
  }

// UserLockKey 用户余额锁键；划转和兑换共用。

  func UserLockKey(userID string) string {
	  return "lock:user:" + userID
   }

// lockUsers 按用户 id 字典序加锁。A→B 与 B→A 同一顺序，避免现货转账交叉死锁。

  func lockUsers(ctx context.Context, lock Locker, ttl time.Duration, a, b string) (ga, gb Guard, err error) {
	   if b == "" || a == b {
		  ga, err = lock.Lock(ctx, UserLockKey(a), ttl)
		   return ga, nil, err
	  }
	   first, second := a, b
	  if second < first {
		   first, second = b, a
	  }
	  g1, err := lock.Lock(ctx, UserLockKey(first), ttl)
	   if err != nil {
		  return nil, nil, err
	   }
	   g2, err := lock.Lock(ctx, UserLockKey(second), ttl)
	  if err != nil {
		   g1.Unlock()
		   return nil, nil, err
	   }
	   if a == first {
		   return g1, g2, nil
	   }
	  return g2, g1, nil
  }
