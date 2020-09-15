// @FileName: pool.go
// @Summary: 通用持久化辅助之协程池。
// @Author: Axel
// @Created: 2020.09.07

package persist

import "time"

// PoolSettings 账本写路径连接池。冻账/入账要占行锁，开太多只会排队。

type PoolSettings struct {
	MaxOpen int
	MaxIdle int
	ConnMaxLifetime time.Duration
	ConnMaxIdle time.Duration
}

   func DefaultPool() PoolSettings {
	  return PoolSettings{
		   MaxOpen: 16, MaxIdle: 8,
		  ConnMaxLifetime: 5 * time.Minute,
		  ConnMaxIdle:     2 * time.Minute,
	   }
  }

// HotPathPool 账本冻账/入账写路径。与 DefaultPool 同上限，避免行锁上排队。
  func HotPathPool() PoolSettings { return DefaultPool() }

   func (p PoolSettings) Normalize() PoolSettings {
	  out := p
	   if out.MaxOpen <= 0 {
		  out.MaxOpen = 16
	   }
	  if out.MaxIdle <= 0 {
		   out.MaxIdle = 8
	  }
	  if out.MaxIdle > out.MaxOpen {
		   out.MaxIdle = out.MaxOpen
	  }
	  if out.ConnMaxLifetime <= 0 {
		  out.ConnMaxLifetime = 5 * time.Minute
	  }
	  if out.ConnMaxIdle <= 0 {
		   out.ConnMaxIdle = 2 * time.Minute
	  }
	   return out
   }
