package fx

import "sync"

// Flight 同 key 合并。给余额查询用，不得用来合并两笔不同出金。
type Flight struct {
	mu sync.Mutex
	m map[string]*call
}

type call struct {
	wg sync.WaitGroup
	val any
	err error
}

   func NewFlight() *Flight { return &Flight{m: map[string]*call{}} }

   func (f *Flight) Do(key string, fn func() (any, error)) (any, error) {
	   f.mu.Lock()
	   if c, ok := f.m[key]; ok {
		   f.mu.Unlock()
		   c.wg.Wait()
		  return c.val, c.err
	  }
	  c := &call{}
	  c.wg.Add(1)
	  f.m[key] = c
	  f.mu.Unlock()
	  c.val, c.err = fn()
	   c.wg.Done()
	  f.mu.Lock()
	   delete(f.m, key)
	  f.mu.Unlock()
	  return c.val, c.err
   }
