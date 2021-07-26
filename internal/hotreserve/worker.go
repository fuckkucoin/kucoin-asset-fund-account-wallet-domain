package hotreserve

import (
	"context"
	"time"
)

// Tick 一轮：刷新快照、释放到期未冻预留、巡检一致。
// 任一步失败即返回，不把失败当成已一致。
  func (s *Service) Tick(ctx context.Context) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   if _, err := s.RefreshAll(ctx); err != nil {
		  return err
	   }
	  if _, err := s.ExpireDue(ctx); err != nil {
		  return err
	  }
	   _, err := s.PatrolAll(ctx)
	  return err
   }

// ExpireDue 释放到期仍处于 open 的预留。已圈存和已兑付原样留下。

  func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	  if s == nil {
		  return 0, ErrUnavailable
	   }
	   s.mu.Lock()
	  now := s.now()
	   ids := make([]string, 0)
	   for _, r := range s.reserves {
		  if r.State == StateOpen && !now.Before(r.ExpiresAt) {
			   ids = append(ids, r.ID)
		  }
	  }
	  s.mu.Unlock()
	   n := 0
	  for _, id := range ids {
		  got, err := s.Expire(ctx, id)
		   if err != nil {
			   return n, err
		   }
		  if got.State == StateExpired {
			  n++
		   }
	  }
	  return n, nil
  }

// Start 按策略间隔跑 Tick，直到 ctx 取消。重复调用只起一个循环。

  func (s *Service) Start(ctx context.Context) {
	   if s == nil {
		  return
	   }
	   s.once.Do(func() {
		  go s.loop(ctx)
	   })
   }

  func (s *Service) loop(ctx context.Context) {
	  every := s.pol.TickEvery
	  if every <= 0 {
		   every = 30 * time.Second
	   }
	  t := time.NewTicker(every)
	   defer t.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-t.C:
			  _ = s.Tick(ctx)
		   }
	   }
   }
