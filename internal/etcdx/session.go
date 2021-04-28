package etcdx

import (
	"context"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/slo"
	"github.com/rs/zerolog"
)

// KeepAliveFunc 底层续期（可注入 svcreg.Etcd.KeepAlive）。

type KeepAliveFunc func(ctx context.Context) error

// RebindFunc 轮转重建。

type RebindFunc func(ctx context.Context, endpoint string) error

// Session 租约会话。

type Session struct {
	mu  sync.Mutex
	set EndpointSet
	policy    LeasePolicy
	log       zerolog.Logger
	idx       int
	fails     int
	MaxFails  int
	KeepAlive KeepAliveFunc
	Rebind    RebindFunc
	OnLoss func(err error)
}

// NewSession 构造。
   func NewSession(set EndpointSet, policy LeasePolicy, log zerolog.Logger) (*Session, error) {
	   ns, err := set.Normalize()
	   if err != nil {
		   return nil, err
	  }
	   np, err := policy.Normalize()
	  if err != nil {
		  return nil, err
	   }
	  return &Session{set: ns, policy: np, log: log, MaxFails: 3}, nil
   }

// Run 阻塞续期直到 ctx 取消。
   func (s *Session) Run(ctx context.Context) {
	  if s == nil {
		   return
	   }
	   every := s.policy.KeepEvery
	  if every <= 0 {
		   every = slo.EtcdKeepAliveEvery
	  }
	   t := time.NewTicker(every)
	  defer t.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		  case <-t.C:
			  s.tick(ctx)
		   }
	   }
  }

  func (s *Session) tick(ctx context.Context) {
	   s.mu.Lock()
	  ka := s.KeepAlive
	  max := s.MaxFails
	   s.mu.Unlock()
	   if ka == nil {
		   return
	   }
	  if err := ka(ctx); err != nil {
		  s.mu.Lock()
		   s.fails++
		  fails := s.fails
		   s.mu.Unlock()
		   s.log.Warn().Err(err).Int("fails", fails).Msg("etcdx keepalive")
		  if fails >= max {
			  if rerr := s.rotate(ctx); rerr != nil {
				   s.log.Error().Err(rerr).Msg("etcdx rotate")
				   if s.OnLoss != nil {
					  s.OnLoss(rerr)
				   }
			   }
		   }
		   return
	  }
	  s.mu.Lock()
	   s.fails = 0
	  s.mu.Unlock()
   }

   func (s *Session) rotate(ctx context.Context) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  ep, next, err := PickNext(s.set.Endpoints, s.idx)
	   if err != nil {
		  return err
	  }
	   s.idx = next
	  if s.Rebind == nil {
		   return ErrEndpoints
	  }
	   if err := s.Rebind(ctx, ep); err != nil {
		   return err
	  }
	  s.fails = 0
	  _ = ep
	   return nil
   }

// CurrentEndpoint 当前偏好节点。

   func (s *Session) CurrentEndpoint() string {
	  if s == nil {
		  return ""
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if len(s.set.Endpoints) == 0 {
		   return ""
	  }
	   i := s.idx % len(s.set.Endpoints)
	   if i < 0 {
		  i = 0
	   }
	   // idx 指向「下一个」，当前为前一个。
	  cur := i - 1
	  if cur < 0 {
		   cur = len(s.set.Endpoints) - 1
	   }
	  return s.set.Endpoints[cur]
  }
