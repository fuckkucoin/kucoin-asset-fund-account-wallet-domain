package redisx

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var unlockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
else
  return 0
end
`)

// GoRedis Client 适配。

type GoRedis struct {
	rdb *redis.Client
}

// WrapGoRedis 包装已 Ping 的客户端。

  func WrapGoRedis(rdb *redis.Client) *GoRedis {
	  return &GoRedis{rdb: rdb}
  }

  func (g *GoRedis) Kind() string { return "goredis" }

  func (g *GoRedis) Close() error {
	   if g == nil || g.rdb == nil {
		  return nil
	  }
	  return g.rdb.Close()
   }

  func (g *GoRedis) Ping(ctx context.Context) error {
	  if g == nil || g.rdb == nil {
		  return ErrUnavailable
	  }
	   if err := g.rdb.Ping(ctx).Err(); err != nil {
		   return fmt.Errorf("%w: %v", ErrUnavailable, err)
	   }
	   return nil
   }

   func (g *GoRedis) Get(ctx context.Context, key string) ([]byte, error) {
	   if g == nil || g.rdb == nil {
		  return nil, ErrUnavailable
	  }
	  s, err := g.rdb.Get(ctx, key).Bytes()
	   if err == redis.Nil {
		   return nil, ErrNotFound
	   }
	   if err != nil {
		   return nil, err
	   }
	  return s, nil
  }

   func (g *GoRedis) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	  if g == nil || g.rdb == nil {
		  return ErrUnavailable
	   }
	   return g.rdb.Set(ctx, key, val, ttl).Err()
   }

   func (g *GoRedis) Del(ctx context.Context, key string) error {
	  if g == nil || g.rdb == nil {
		   return ErrUnavailable
	   }
	   return g.rdb.Del(ctx, key).Err()
   }

   func (g *GoRedis) TryLock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	  if g == nil || g.rdb == nil {
		  return "", ErrUnavailable
	  }
	   if ttl <= 0 {
		  ttl = 5 * time.Second
	   }
	  tok := uuid.NewString()
	   ok,
		   err := g.rdb.SetNX(ctx, key, tok, ttl).Result()
	   if err != nil {
		   return "", err
	   }
	  if !ok {
		   return "", ErrLockHeld
	  }
	  return tok, nil
   }

   func (g *GoRedis) Unlock(ctx context.Context, key, token string) error {
	  if g == nil || g.rdb == nil {
		   return ErrUnavailable
	  }
	  n, err := unlockScript.Run(ctx, g.rdb, []string{key}, token).Int()
	   if err != nil {
		  return err
	  }
	   if n == 0 {
		   return ErrLockHeld
	  }
	   return nil
   }

// Raw 暴露底层客户端供遗留路径迁移；新代码应走 Client 接口。
   func (g *GoRedis) Raw() *redis.Client {
	   if g == nil {
		  return nil
	   }
	   return g.rdb
  }
