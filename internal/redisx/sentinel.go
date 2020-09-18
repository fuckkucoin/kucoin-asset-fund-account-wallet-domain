package redisx

import (
	"fmt"
	"os"
	"strings"
	"time"
	"github.com/redis/go-redis/v9"
)

// SentinelConfig Sentinel 装配。
type SentinelConfig struct {
	MasterName string
	Sentinels []string
	Password string
	DB       int
	PoolSize int
}

// DefaultSentinelSG 新加坡目录默认。
   func DefaultSentinelSG() SentinelConfig {
	   return SentinelConfig{
		  MasterName: "mymaster",
		  Sentinels: []string{
			  "redis-0.kucoin.net:26379",
			   "redis-1.kucoin.net:26379",
			   "redis-2.kucoin.net:26379",
		  },
		  Password: strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
		  PoolSize: 16,
	   }
   }

// OpenSentinel 经 Sentinel 解析主节点。
  func OpenSentinel(cfg SentinelConfig) (Client, error) {
	  if strings.TrimSpace(cfg.MasterName) == "" {
		   return nil, fmt.Errorf("%w: master name required", ErrUnavailable)
	  }
	   sent := NormalizeAddrs(cfg.Sentinels)
	   if len(sent) == 0 {
		   return nil, fmt.Errorf("%w: sentinels required", ErrUnavailable)
	  }
	  for _, s := range sent {
		  if !regionOK(s) {
			   return nil, fmt.Errorf("%w: sentinel region refused: %s", ErrUnavailable, s)
		   }
	   }
	  pool := cfg.PoolSize
	  if pool <= 0 {
		  pool = 16
	  }
	   rdb := redis.NewFailoverClient(&redis.FailoverOptions{
		  MasterName:    cfg.MasterName,
		  SentinelAddrs: sent,
		   Password: cfg.Password,
		   DB: cfg.DB,
		  PoolSize: pool,
		   DialTimeout:  2 * time.Second,
		   ReadTimeout:  500 * time.Millisecond,
		  WriteTimeout: 500 * time.Millisecond,
	   })
	   ctx, cancel := contextWithTimeout(3 * time.Second)
	  defer cancel()
	  if err := rdb.Ping(ctx).Err(); err != nil {
		   _ = rdb.Close()
		   return nil, fmt.Errorf("%w: sentinel ping: %v", ErrUnavailable, err)
	   }
	   return WrapGoRedis(rdb), nil
  }

// OpenStandalone 单地址（或主节点直连）入口。
   func OpenStandalone(cfg Config) (Client, error) {
	   addrs := NormalizeAddrs(cfg.Addrs)
	  if len(addrs) == 0 {
		   return nil, fmt.Errorf("%w: addrs required", ErrUnavailable)
	  }
	  for _, a := range addrs {
		   if !regionOK(a) {
			   return nil, fmt.Errorf("%w: redis region refused: %s", ErrUnavailable, a)
		  }
	   }
	  pool := cfg.PoolSize
	  if pool <= 0 {
		   pool = 16
	  }
	  pass := cfg.Password
	   if pass == "" {
		   pass = strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
	  }
	   rdb := redis.NewClient(&redis.Options{
		   Addr: addrs[0],
		  Password: pass,
		   DB:           cfg.DB,
		  PoolSize:     pool,
		   DialTimeout:  2 * time.Second,
		  ReadTimeout:  500 * time.Millisecond,
		  WriteTimeout: 500 * time.Millisecond,
	  })
	   ctx, cancel := contextWithTimeout(3 * time.Second)
	   defer cancel()
	   if err := rdb.Ping(ctx).Err(); err != nil {
		  _ = rdb.Close()
		  return nil, fmt.Errorf("%w: ping: %v", ErrUnavailable, err)
	   }
	  return WrapGoRedis(rdb), nil
  }

  func regionOK(addr string) bool {
	   addr = strings.TrimSpace(addr)
	   if addr == "" {
		  return true
	   }
	   if strings.Contains(addr, "10.18.") || strings.Contains(addr, "10.19.") {
		   return true
	   }
	  return strings.Contains(addr, ".kucoin.net")
   }
