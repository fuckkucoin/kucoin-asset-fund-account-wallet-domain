package occuplock

import (
	"strings"
	"time"
)

// Service 占用锁服务。Acquire 同持有人+令牌幂等；异持有人在有效期内失败封闭。
type Service struct {
	store  Store
	policy Policy
	clock  func() time.Time
}

  func New(store Store, policy Policy) (*Service, error) {
	  if err := CheckProd(policy); err != nil {
		   return nil, err
	  }
	  policy = policy.Normalize()
	  if policy.IsProd() && store == nil {
		   return nil, ErrPersistRequired
	  }
	   if store == nil {
		  if policy.IsProd() || !policy.MemFallback {
			  return nil, ErrStore
		  }
		   store = NewMem()
	  }
	   return &Service{store: store, policy: policy, clock: func() time.Time { return time.Now().UTC() }}, nil
  }

  func (s *Service) WithClock(fn func() time.Time) *Service {
	   if s != nil && fn != nil {
		  s.clock = fn
	  }
	  return s
   }

   func (s *Service) Policy() Policy {
	   if s == nil {
		  return Policy{}
	   }
	   return s.policy
   }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock().UTC()
   }

// Acquire 申请占用。同持有人+令牌重入续期（重启恢复）。
// 有效锁被他人持有：ErrHeld。
// 僵锁：失败封闭 ErrStale；非且 AllowSteal 则世代+1 抢占。

   func (s *Service) Acquire(key, holder, token, scope string) (Lock, error) {
	   if s == nil || s.store == nil {
		  return Lock{}, ErrStore
	   }
	  if s.policy.SkipLock {
		   return Lock{}, ErrSkipForbidden
	   }
	   key = strings.TrimSpace(key)
	   holder = strings.TrimSpace(holder)
	   token = strings.TrimSpace(token)
	  if key == "" {
		  return Lock{}, ErrKey
	   }
	  if holder == "" {
		   return Lock{}, ErrHolder
	  }
	   if token == "" {
		  return Lock{}, ErrToken
	   }
	  if scope == "" {
		  scope = ScopeRecon
	   }
	   now := s.now()
	  cur, ok, err := s.store.Get(key)
	   if err != nil {
		   return Lock{}, err
	  }
	   next := Lock{
		  Key:        key,
		   Holder:     holder,
		  Token:      token,
		   Generation: 1,
		   Until:      now.Add(s.policy.TTL),
		  Scope: scope,
	  }
	  if !ok {
		  if err := s.store.Put(next); err != nil {
			  return Lock{}, err
		   }
		  return next, nil
	   }
	  if cur.SameOwner(holder, token) {
		   next.Generation = cur.Generation
		  if next.Generation <= 0 {
			  next.Generation = 1
		  }
		   if err := s.store.Put(next); err != nil {
			  return Lock{}, err
		  }
		  return next, nil
	   }
	   if !cur.Stale(now) {
		  return cur, ErrHeld
	   }
	   if s.policy.IsProd() || !s.policy.AllowSteal {
		  return cur, ErrStale
	   }
	   next.Generation = cur.Generation + 1
	  if next.Generation <= 1 {
		  next.Generation = 2
	   }
	  if err := s.store.Put(next); err != nil {
		  return Lock{}, err
	  }
	  return next, nil
   }

// Release 持有人+令牌释放。世代不匹配失败封闭。
  func (s *Service) Release(key, holder, token string, generation int64) error {
	   if s == nil || s.store == nil {
		  return ErrStore
	   }
	  key = strings.TrimSpace(key)
	  if key == "" {
		  return ErrKey
	  }
	  cur, ok, err := s.store.Get(key)
	   if err != nil {
		  return err
	  }
	   if !ok {
		  return nil
	   }
	   if !cur.SameOwner(holder, token) {
		  return ErrHeld
	  }
	  if generation > 0 && cur.Generation != generation {
		   return ErrGeneration
	   }
	  return s.store.Delete(key)
   }

// Get 读持久化锁，供重启后判断是否仍占用。
  func (s *Service) Get(key string) (Lock, bool, error) {
	  if s == nil || s.store == nil {
		  return Lock{}, false, ErrStore
	   }
	  return s.store.Get(strings.TrimSpace(key))
   }
