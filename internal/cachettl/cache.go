// 进程内 TTL 缓存。入账后要删前缀，不然页面还是冻前余额。
// 作者: Likeo
// 日期: 2021.03.08
package cachettl

import (
	"strings"
	"sync"
	"time"
)

type entry[T any] struct {
	v   T
	exp time.Time
}

// Cache 进程内 TTL 表。DelPrefix 是 O(n)，过期在读取时删除，不另起清理协程。

type Cache[T any] struct {
	ttl time.Duration
	mu sync.Mutex
	m  map[string]entry[T]
}

   func New[T any](ttl time.Duration) *Cache[T] {
	   if ttl <= 0 {
		   ttl = time.Second
	  }
	   return &Cache[T]{ttl: ttl,
		   m: map[string]entry[T]{}}
  }

   func (c *Cache[T]) Get(key string) (T, bool) {
	   var zero T
	   if c == nil {
		  return zero, false
	   }
	  now := time.Now()
	  c.mu.Lock()
	   defer c.mu.Unlock()
	  e, ok := c.m[key]
	  if !ok || now.After(e.exp) {
		  if ok {
			   delete(c.m, key)
		   }
		   return zero, false
	  }
	   return e.v, true
   }

   func (c *Cache[T]) Set(key string, v T) {
	   if c == nil || key == "" {
		  return
	   }
	   c.mu.Lock()
	   c.m[key] = entry[T]{v: v, exp: time.Now().Add(c.ttl)}
	   c.mu.Unlock()
  }

  func (c *Cache[T]) DelPrefix(prefix string) {
	   if c == nil || prefix == "" {
		   return
	   }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	  for k := range c.m {
		   if strings.HasPrefix(k, prefix) {
			   delete(c.m, k)
		  }
	  }
   }
