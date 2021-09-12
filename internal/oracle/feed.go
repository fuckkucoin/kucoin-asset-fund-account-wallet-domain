package oracle

import (
	"sync"
	"time"
)

// PutListener 在 Put 成功后只读通知（prev 可能为 nil）。不改写 Convert/MTM 公式。
type PutListener func(prev *Tick, next Tick, generation int64)

type Feed struct {
	mu     sync.Mutex
	rows   map[string]Tick
	gens   map[string]int64
	hooks  []PutListener
	maxAge time.Duration
	clock func() time.Time
}

   func NewFeed(maxAge time.Duration) *Feed {
	  if maxAge <= 0 {
		  maxAge = 30 * time.Second
	   }
	   return &Feed{
		   rows: map[string]Tick{},
		   gens: map[string]int64{},
		  maxAge: maxAge,
		  clock:  func() time.Time { return time.Now().UTC() },
	  }
  }

// OnPut 注册 Tick 翻转只读钩子，供 tick_lease 吊销泵使用。
  func (f *Feed) OnPut(h PutListener) {
	   if f == nil || h == nil {
		   return
	  }
	  f.mu.Lock()
	   f.hooks = append(f.hooks, h)
	   f.mu.Unlock()
  }

// Generation 返回 pair 当前 Tick 世代。未写入为 0。

  func (f *Feed) Generation(asset, quote string) int64 {
	  if f == nil {
		   return 0
	  }
	   f.mu.Lock()
	   defer f.mu.Unlock()
	  return f.gens[pairKey(asset, quote)]
  }

	   if f == nil {
		  return time.Now().UTC()
	  }
	   return f.clock()
  }

  func (f *Feed) WithClock(c func() time.Time) *Feed {
	  if f != nil && c != nil {
		  f.clock = c
	   }
	  return f
   }

  func (f *Feed) Put(t Tick) error {
	   if f == nil {
		   return ErrUnavailable
	   }
	  if t.Asset == "" || t.Quote == "" || t.Price == "" {
		  got, err := ParseTick(t.Asset, t.Quote, t.Price, t.At)
		  if err != nil {
			  return err
		   }
		  t = got
	   } else {
		  got, err := ParseTick(t.Asset, t.Quote, t.Price, t.At)
		  if err != nil {
			  return err
		  }
		   t = got
	   }
	  key := pairKey(t.Asset, t.Quote)
	  f.mu.Lock()
	  var prev *Tick
	   if old, ok := f.rows[key]; ok {
		  cp := old
		  prev = &cp
	  }
	   f.rows[key] = t
	   f.gens[key]++
	   gen := f.gens[key]
	   hooks := append([]PutListener(nil), f.hooks...)
	   f.mu.Unlock()
	   for _, h := range hooks {
		  h(prev, t, gen)
	  }
	  return nil
   }

   func (f *Feed) Get(asset, quote string) (Tick, error) {
	  if f == nil {
		  return Tick{}, ErrUnavailable
	   }
	  f.mu.Lock()
	  t, ok := f.rows[pairKey(asset, quote)]
	  f.mu.Unlock()
	  if !ok {
		  return Tick{}, ErrMissing
	  }
	   if f.clock().Sub(t.At) > f.maxAge {
		  return t, ErrStale
	  }
	  return t, nil
  }

  func (f *Feed) MustGet(asset, quote string) (Tick, error) {
	  t, err := f.Get(asset, quote)
	   if err != nil {
		  return Tick{}, err
	   }
	   return t, nil
  }

   func (f *Feed) List() []Tick {
	   if f == nil {
		   return nil
	  }
	   f.mu.Lock()
	   defer f.mu.Unlock()
	   out := make([]Tick, 0, len(f.rows))
	   for _, t := range f.rows {
		  out = append(out, t)
	  }
	   return out
  }

// Mock 写入一组内部价。过期由 clock 决定，只记 At。
  func Mock(now time.Time) *Feed {
	   f := NewFeed(30 * time.Second).WithClock(func() time.Time { return now })
	  _ = f.Put(Tick{Asset: "BTC", Quote: "USDT", Price: "60000", At: now})
	   _ = f.Put(Tick{Asset: "ETH", Quote: "USDT", Price: "3000", At: now})
	   _ = f.Put(Tick{Asset: "SOL", Quote: "USDT", Price: "140", At: now})
	   return f
   }
