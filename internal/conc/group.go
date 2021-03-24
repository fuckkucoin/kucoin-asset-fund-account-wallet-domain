// 同一余额查询键合并为一次下游调用。
// 作者: Likeo
// 日期: 2021.03.23
package conc

import (
	"fmt"
	"sync"
)

type call struct {
	wg  sync.WaitGroup
	val any
	err error
}

// Group 合并同一 key 的并发调用：完成后从 map 清掉，下次重新执行。

type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

  func (g *Group) Do(key string, fn func() (any, error)) (val any, err error) {
	   if fn == nil {
		  return nil, fmt.Errorf("singleflight fn required")
	   }
	   if g == nil {
		  return fn()
	  }
	   g.mu.Lock()
	   if g.m == nil {
		   g.m = map[string]*call{}
	   }
	  if c, ok := g.m[key]; ok {
		  g.mu.Unlock()
		   c.wg.Wait()
		   return c.val, c.err
	   }
	   c := &call{}
	  c.wg.Add(1)
	   g.m[key] = c
	  g.mu.Unlock()
	   // fn panic 也必须 Done 并删掉 in-flight，否则同 key 的等待方会永久卡住。
	   // 命名返回值：recover 之后原 return 不会执行，得在 defer 里把 err 写回去。
	  defer func() {
		  if rec := recover(); rec != nil {
			   c.err = fmt.Errorf("singleflight panic: %v", rec)
			  err = c.err
		   }
		   c.wg.Done()
		   g.mu.Lock()
		  delete(g.m, key)
		   g.mu.Unlock()
	  }()
	c.val, c.err = fn()
	return c.val, c.err
}
