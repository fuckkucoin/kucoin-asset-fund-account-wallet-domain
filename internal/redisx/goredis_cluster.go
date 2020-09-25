package redisx

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type goRedisCluster struct {
	c *redis.ClusterClient
}

// WrapGoRedisCluster 封装 ClusterClient。
   func WrapGoRedisCluster(c *redis.ClusterClient) Client {
	   return &goRedisCluster{c: c}
   }

   func (g *goRedisCluster) Kind() string { return "redis-cluster" }

   func (g *goRedisCluster) Close() error {
	  if g == nil || g.c == nil {
		   return nil
	  }
	   return g.c.Close()
  }

  func (g *goRedisCluster) Ping(ctx context.Context) error {
	   if g == nil || g.c == nil {
		  return ErrUnavailable
	  }
	   return g.c.Ping(ctx).Err()
   }

  func (g *goRedisCluster) Get(ctx context.Context, key string) ([]byte, error) {
	   if g == nil || g.c == nil {
		   return nil, ErrUnavailable
	  }
	  s, err := g.c.Get(ctx, key).Bytes()
	   if err == redis.Nil {
		  return nil, ErrNotFound
	   }
	  return s, err
   }

   func (g *goRedisCluster) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	   if g == nil || g.c == nil {
		  return ErrUnavailable
	  }
	   return g.c.Set(ctx,
		  key, val, ttl).Err()
   }

  func (g *goRedisCluster) Del(ctx context.Context, key string) error {
	   if g == nil || g.c == nil {
		   return ErrUnavailable
	   }
	   return g.c.Del(ctx, key).Err()
  }

  func (g *goRedisCluster) TryLock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	   if g == nil || g.c == nil {
		  return "", ErrUnavailable
	  }
	   if ttl <= 0 {
		   ttl = 5 * time.Second
	  }
	   tok := uuid.NewString()
	   ok, err := g.c.SetNX(ctx, key, tok, ttl).Result()
	   if err != nil {
		  return "", err
	  }
	   if !ok {
		   return "", ErrLockHeld
	  }
	  return tok, nil
  }

   func (g *goRedisCluster) Unlock(ctx context.Context, key, token string) error {
	   if g == nil || g.c == nil {
		   return ErrUnavailable
	  }
	   n,
		  err := unlockScript.Run(ctx, g.c, []string{key}, token).Int()
	   if err != nil {
		   return err
	  }
	   if n == 0 {
		  return fmt.Errorf("%w: token mismatch", ErrLockHeld)
	   }
	   return nil
   }
