package redisx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnavailable = errors.New("redis unavailable")
	ErrNotFound = errors.New("redis key not found")
	ErrLockHeld = errors.New("redis lock held")
)

// Client 抽象；接集群，单测可挂 Mem。
type Client interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	TryLock(ctx context.Context, key string, ttl time.Duration) (token string, err error)
	Unlock(ctx context.Context,
		key, token string) error
	Ping(ctx context.Context) error
	Kind() string
	Close() error
}

// Config 来自部署配置，禁止硬编码密码。
type Config struct {
	Addrs    []string
	Password string
	DB       int
	PoolSize int
}

   func NormalizeAddrs(addrs []string) []string {
	   out := make([]string, 0, len(addrs))
	  for _, a := range addrs {
		  a = strings.TrimSpace(a)
		   if a != "" {
			  out = append(out, a)
		  }
	   }
	   return out
   }

// Mem 仅夹具。

type Mem struct {
	mu   sync.Mutex
	kv   map[string]memVal
	lock map[string]memLock
}

type memVal struct {
	b []byte
	exp time.Time
}

type memLock struct {
	token string
	exp time.Time
}

  func NewMem() *Mem {
	  return &Mem{kv: map[string]memVal{}, lock: map[string]memLock{}}
   }

   func (m *Mem) Kind() string { return "mem" }

   func (m *Mem) Close() error { return nil }

   func (m *Mem) Ping(context.Context) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	  return nil
   }

   func (m *Mem) Get(_ context.Context, key string) ([]byte, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  v, ok := m.kv[key]
	   if !ok || (!v.exp.IsZero() && time.Now().After(v.exp)) {
		   return nil, ErrNotFound
	  }
	   cp := append([]byte(nil), v.b...)
	  return cp, nil
   }

  func (m *Mem) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var exp time.Time
	  if ttl > 0 {
		   exp = time.Now().Add(ttl)
	   }
	   m.kv[key] = memVal{b: append([]byte(nil), val...), exp: exp}
	  return nil
  }

  func (m *Mem) Del(_ context.Context, key string) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   delete(m.kv, key)
	   return nil
   }

   func (m *Mem) TryLock(_ context.Context, key string, ttl time.Duration) (string, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if cur, ok := m.lock[key]; ok && time.Now().Before(cur.exp) {
		  return "", ErrLockHeld
	  }
	   tok := fmt.Sprintf("lk-%d", time.Now().UnixNano())
	  exp := time.Now().Add(ttl)
	   if ttl <= 0 {
		  exp = time.Now().Add(5 * time.Second)
	  }
	   m.lock[key] = memLock{token: tok, exp: exp}
	   return tok, nil
  }

   func (m *Mem) Unlock(_ context.Context, key, token string) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.lock[key]
	  if !ok || cur.token != token {
		  return ErrLockHeld
	  }
	   delete(m.lock, key)
	   return nil
  }
