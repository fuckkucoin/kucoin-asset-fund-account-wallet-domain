package fxmark

import (
	"context"
	"strings"
	"sync"
)

// PutObserver Put 成功后的只读观察面；观察失败不得回滚已写入牌价。
type PutObserver func(ctx context.Context, asset, usd string)

// GenerationClock 资产牌价世代；Put 同资产改价即递增。

type GenerationClock struct {
	mu sync.Mutex
	gen map[string]int64
	hook []PutObserver
}

   func NewGenerationClock() *GenerationClock {
	  return &GenerationClock{gen: make(map[string]int64)}
  }

   func (c *GenerationClock) Observe(fn PutObserver) {
	   if c == nil || fn == nil {
		  return
	   }
	   c.mu.Lock()
	   c.hook = append(c.hook, fn)
	  c.mu.Unlock()
   }

  func (c *GenerationClock) Bump(asset string) int64 {
	  if c == nil {
		  return 0
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  c.mu.Lock()
	  defer c.mu.Unlock()
	   if c.gen == nil {
		   c.gen = make(map[string]int64)
	  }
	  c.gen[asset]++
	   return c.gen[asset]
   }

  func (c *GenerationClock) Get(asset string) int64 {
	  if c == nil {
		   return 0
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   c.mu.Lock()
	  defer c.mu.Unlock()
	  return c.gen[asset]
  }

   func (c *GenerationClock) Snapshot() map[string]int64 {
	   if c == nil {
		  return nil
	  }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  out := make(map[string]int64, len(c.gen))
	   for k, v := range c.gen {
		  out[k] = v
	  }
	   return out
  }

  func (c *GenerationClock) fire(ctx context.Context, asset, usd string) {
	   if c == nil {
		  return
	   }
	   c.mu.Lock()
	  hooks := append([]PutObserver(nil),
		  c.hook...)
	  c.mu.Unlock()
	  for _, h := range hooks {
		  h(ctx, asset, usd)
	  }
  }

var defaultClock = NewGenerationClock()

  func DefaultClock() *GenerationClock { return defaultClock }

  func (b *Book) BindGeneration(c *GenerationClock) *Book {
	   if b != nil {
		  b.gen = c
	  }
	  return b
  }

  func (b *Book) Generation() *GenerationClock {
	  if b == nil || b.gen == nil {
		   return DefaultClock()
	  }
	   return b.gen
   }

  func (b *Book) ObservePut(fn PutObserver) *Book {
	   b.Generation().Observe(fn)
	  return b
  }

   func (b *Book) notifyPut(ctx context.Context, asset, usd string) {
	   clk := b.Generation()
	   clk.Bump(asset)
	   clk.fire(ctx, asset, usd)
  }
