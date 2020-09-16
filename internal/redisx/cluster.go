package redisx

import (
	"fmt"
	"os"
	"strings"
	"time"
	"github.com/redis/go-redis/v9"
)

// ClusterConfig Cluster 装配。
type ClusterConfig struct {
	Addrs []string
	Password string
	PoolSize int
}

// DefaultClusterSG 新加坡目录默认（cluster 总线同主机）。

   func DefaultClusterSG() ClusterConfig {
	  return ClusterConfig{
		  Addrs: []string{
			  "redis-0.kucoin.net:6379",
			  "redis-1.kucoin.net:6379",
			   "redis-2.kucoin.net:6379",
		   },
		   Password: strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
		  PoolSize: 32,
	  }
   }

// DefaultClusterTYO 东京灾备目录。

  func DefaultClusterTYO() ClusterConfig {
	   return ClusterConfig{
		  Addrs: []string{
			   "10.19.0.12:6379",
			   "10.19.0.13:6379",
			  "10.19.0.16:6379",
		  },
		   Password: strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
		  PoolSize: 32,
	   }
  }

// OpenCluster 经 Cluster 协议拨号。

  func OpenCluster(cfg ClusterConfig) (Client, error) {
	   addrs := NormalizeAddrs(cfg.Addrs)
	   if len(addrs) == 0 {
		  return nil, fmt.Errorf("%w: cluster addrs required", ErrUnavailable)
	  }
	   for _, a := range addrs {
		  if !regionOK(a) {
			   return nil, fmt.Errorf("%w: cluster region refused: %s", ErrUnavailable, a)
		   }
	   }
	   pool := cfg.PoolSize
	   if pool <= 0 {
		   pool = 32
	   }
	  pass := cfg.Password
	  if pass == "" {
		  pass = strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
	  }
	  rdb := redis.NewClusterClient(&redis.ClusterOptions{
		   Addrs:    addrs,
		   Password: pass,
		  PoolSize:     pool,
		  DialTimeout:  2 * time.Second,
		  ReadTimeout:  500 * time.Millisecond,
		  WriteTimeout: 500 * time.Millisecond,
	   })
	   ctx, cancel := contextWithTimeout(3 * time.Second)
	  defer cancel()
	   if err := rdb.Ping(ctx).Err(); err != nil {
		  _ = rdb.Close()
		  return nil, fmt.Errorf("%w: cluster ping: %v", ErrUnavailable, err)
	   }
	   return WrapGoRedisCluster(rdb), nil
   }
