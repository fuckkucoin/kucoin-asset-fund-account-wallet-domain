package redisx

import (
	"fmt"
	"os"
	"strings"
)

// Open 入口。默认走 go-redis 单地址；AA_REDIS_MEM=1 仅进程内夹具。
   func Open(cfg Config) (Client, error) {
	  addrs := NormalizeAddrs(cfg.Addrs)
	  if len(addrs) == 0 {
		   return nil, fmt.Errorf("%w: addrs required", ErrUnavailable)
	   }
	  if strings.TrimSpace(os.Getenv("AA_REDIS_MEM")) == "1" {
		  return NewMem(), nil
	   }
	  mode := strings.ToLower(strings.TrimSpace(os.Getenv("AA_REDIS_MODE")))
	  switch mode {
	  case "sentinel":
		   sc := DefaultSentinelSG()
		   if cfg.Password != "" {
			  sc.Password = cfg.Password
		  }
		  if cfg.PoolSize > 0 {
			  sc.PoolSize = cfg.PoolSize
		   }
		   if len(addrs) > 0 {
			   sc.Sentinels = addrs
		   }
		   return OpenSentinel(sc)
	  case "cluster":
		   cc := DefaultClusterSG()
		   if cfg.Password != "" {
			  cc.Password = cfg.Password
		  }
		   if cfg.PoolSize > 0 {
			  cc.PoolSize = cfg.PoolSize
		   }
		  if len(addrs) > 0 {
			  cc.Addrs = addrs
		  }
		   return OpenCluster(cc)
	   default:
		   return OpenStandalone(cfg)
	   }
  }

// OpenFromEnv 读 REDIS_ADDR / REDIS_ADDRS。
   func OpenFromEnv() (Client, error) {
	  raw := strings.TrimSpace(os.Getenv("REDIS_ADDRS"))
	   if raw == "" {
		   raw = strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	   }
	  if raw == "" {
		   raw = "redis-0.kucoin.net:6379"
	  }
	  parts := strings.Split(raw, ",")
	   return Open(Config{Addrs: parts, Password: strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))})
   }
