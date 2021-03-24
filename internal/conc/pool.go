// 有界 worker 池。
// 作者: Likeo
// 日期: 2021.03.23
package conc

import (
	"context"
	"sync"
)

// For 最多 n 个 goroutine 跑 items；n<=1 串行。
// 信号量满时若 ctx 已取消，停止派发并等已启动的任务退出，避免卡在 sem 发送上。

  func For[T any](ctx context.Context, n int, items []T, fn func(context.Context, T)) {
	  if len(items) == 0 || fn == nil {
		  return
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  if n <= 1 || len(items) == 1 {
		   for _, it := range items {
			   if ctx.Err() != nil {
				   return
			   }
			   fn(ctx, it)
		  }
		  return
	  }
	   sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	for _, it := range items {
		if ctx.Err() != nil {
			break
		}
		it := it
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			  defer wg.Done()
			   defer func() { <-sem }()
			  fn(ctx, it)
		   }()
	}
	wg.Wait()
}
