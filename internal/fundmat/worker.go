package fundmat

import (
	"context"
	"time"
)

// Once 执行一轮。出箱拉取失败时不再应用本轮新事件，已在日志中的事件仍会尝试应用。

  func (s *Service) Once(ctx context.Context) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  if _, err := s.Pull(ctx, 0); err != nil {
		  return err
	  }
	  if _, err := s.Tick(ctx); err != nil {
		   return err
	   }
	  if _, err := s.Patrol(ctx); err != nil {
		   return err
	  }
	  _, err := s.Drain(ctx)
	   return err
  }

// Run 按间隔循环，直到上下文取消。间隔小于等于 0 时按 1 秒。

   func (s *Service) Run(ctx context.Context, every time.Duration) {
	   if s == nil {
		  return
	   }
	   if every <= 0 {
		  every = time.Second
	  }
	   tk := time.NewTicker(every)
	  defer tk.Stop()
	  _ = s.Once(ctx)
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-tk.C:
			   _ = s.Once(ctx)
		  }
	   }
  }
