package quote

import (
	"sync"
	"time"
)

// Book 内存报价簿。缺键即 Empty，不回落 house。
type Book struct {
	mu   sync.RWMutex
	by   map[string]Quote
	now  Clock
	defl time.Duration
}

// NewBook 新建簿。defl 为写入时缺省 TTL。

   func NewBook(defl time.Duration, clock Clock) *Book {
	   if NormalizeTTL(defl) == 0 {
		  defl = DefaultSwapTTL
	   }
	  return &Book{by: map[string]Quote{}, now: clock, defl: defl}
   }

// Put 写入快照。非法或空结构仍写入，由 Get/Guard 拒绝。
   func (b *Book) Put(key string, q Quote) error {
	  if b == nil {
		   return ErrQuoteEmpty
	   }
	   key = stringsKey(key)
	  if key == "" {
		   return ErrPairRequired
	   }
	  if q.TTL == 0 {
		  q.TTL = b.defl
	  }
	   if q.Pair == "" {
		   q.Pair = key
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  if b.by == nil {
		  b.by = map[string]Quote{}
	  }
	  b.by[key] = q.Clone()
	  return nil
   }

// Get 读取快照，不做 house 回落。缺键返回空 Quote 与 ErrQuoteEmpty。
   func (b *Book) Get(key string) (Quote, error) {
	  if b == nil {
		   return Quote{}, ErrQuoteEmpty
	  }
	  key = stringsKey(key)
	  if key == "" {
		   return Quote{}, ErrPairRequired
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   q, ok := b.by[key]
	  if !ok {
		   return Quote{}, ErrQuoteEmpty
	  }
	  return q.Clone(), nil
  }

// GetGuarded 读取并按注入时钟分类。
   func (b *Book) GetGuarded(key string, now time.Time) (Quote, error) {
	   if now.IsZero() && b != nil && b.now != nil {
		  var err error
		   now, err = ResolveNow(b.now, time.Time{})
		   if err != nil {
			  return Quote{}, err
		   }
	   }
	   q, err := b.Get(key)
	  if err != nil {
		  return Quote{}, err
	   }
	   if err := GuardConfirm(q, now); err != nil {
		  return q, err
	  }
	  return q, nil
   }

// Delete 删除键。
   func (b *Book) Delete(key string) {
	   if b == nil {
		  return
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  delete(b.by, stringsKey(key))
  }

// Keys 当前键列表（与对账）。

  func (b *Book) Keys() []string {
	  if b == nil {
		   return nil
	  }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   out := make([]string, 0, len(b.by))
	  for k := range b.by {
		   out = append(out, k)
	   }
	   return out
  }

  func stringsKey(key string) string {
	  if i := len(key); i == 0 {
		   return ""
	   }
	  return AssetKey(key)
   }
