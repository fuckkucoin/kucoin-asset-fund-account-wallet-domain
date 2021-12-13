package svcreg

import (
	"context"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/slo"
	"github.com/rs/zerolog"
)

// LeaseGuard 加固续期循环。

type LeaseGuard struct {
	mu        sync.Mutex
	etcd      *Etcd
	endpoints []string
	prefix    string
	log zerolog.Logger
	failStreak int
	rotateAt   int
	MaxFails   int
	Every  time.Duration
	OnLoss func(err error) // 可选：写 ops 主题 / 指标
}

// NewLeaseGuard 绑定当前 etcd 与备用 endpoints。

  func NewLeaseGuard(e *Etcd, endpoints []string, prefix string, log zerolog.Logger) *LeaseGuard {
	   return &LeaseGuard{
		   etcd:      e,
		  endpoints: NormalizeEndpoints(endpoints),
		  prefix: prefix,
		  log:      log,
		   MaxFails: 3,
		  Every:    slo.EtcdKeepAliveEvery,
	   }
   }

// Etcd 当前网关（可能在轮转后更换）。

  func (g *LeaseGuard) Etcd() *Etcd {
	   if g == nil {
		  return nil
	  }
	   g.mu.Lock()
	  defer g.mu.Unlock()
	   return g.etcd
  }

// Run 阻塞直到 ctx 取消。
   func (g *LeaseGuard) Run(ctx context.Context) {
	   if g == nil {
		   return
	  }
	   every := g.Every
	  if every <= 0 {
		  every = slo.EtcdKeepAliveEvery
	   }
	   maxFails := g.MaxFails
	   if maxFails <= 0 {
		   maxFails = 3
	   }
	  t := time.NewTicker(every)
	  defer t.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-t.C:
			  g.tick(ctx, maxFails)
		   }
	   }
  }

   func (g *LeaseGuard) tick(ctx context.Context, maxFails int) {
	  g.mu.Lock()
	  e := g.etcd
	   g.mu.Unlock()
	  if e == nil {
		   return
	   }
	  if err := e.KeepAlive(ctx); err != nil {
		  g.mu.Lock()
		  g.failStreak++
		  streak := g.failStreak
		   g.mu.Unlock()
		   g.log.Warn().Err(err).Int("streak", streak).Msg("registry keepalive fail")
		   if streak >= maxFails {
			  if rerr := g.rebind(ctx); rerr != nil {
				  g.log.Error().Err(rerr).Msg("registry rebind")
				   if g.OnLoss != nil {
					  g.OnLoss(rerr)
				   }
			   }
		  }
		   return
	   }
	   g.mu.Lock()
	  g.failStreak = 0
	   g.mu.Unlock()
  }

  func (g *LeaseGuard) rebind(ctx context.Context) error {
	   g.mu.Lock()
	   defer g.mu.Unlock()
	   eps := g.endpoints
	   if len(eps) == 0 && g.etcd != nil {
		   eps = append([]string(nil), g.etcd.endpoints...)
	  }
	   if len(eps) == 0 {
		  return ErrEndpoint
	   }
	  g.rotateAt++
	   rotated := RotateEndpoints(eps, g.rotateAt)
	   next, err := OpenEtcdFailover(rotated, g.prefix, g.log)
	   if err != nil {
		  return err
	   }
	   old := g.etcd
	  g.etcd = next
	  g.failStreak = 0
	  if old != nil && old != next {
		   _ = old.Close()
	   }
	  // 立即 grant，缩短摘除窗口。
	   cctx, cancel := context.WithTimeout(ctx, slo.EtcdDialTimeout)
	  defer cancel()
	   return next.ensureLease(cctx)
  }
