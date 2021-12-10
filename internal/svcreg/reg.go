package svcreg

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrClosed = errors.New("service registry closed")
	ErrNoInstance = errors.New("no healthy instance")
	ErrEndpoint = errors.New("registry endpoint required")
)

// Instance 已注册进程副本。
type Instance struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Addr string `json:"addr"`
}

// Registry 注册与发现抽象。实现为 etcd；为 Mem。

type Registry interface {
	Register(ctx context.Context, inst Instance) error
	Deregister(ctx context.Context, inst Instance) error
	Watch(name string) []Instance
	Pick(name string) (Instance, error)
	Kind() string
	Close() error
}

// DialReporter 可选：拨号在途回写，供 P2C 负载偏好。
type DialReporter interface {
	ReportDial(addr string, delta int64)
}

// Mem 进程内注册表，仅/非。

type Mem struct {
	mu   sync.RWMutex
	inst map[string][]Instance
	rr   map[string]*uint64
}

   func NewMem() *Mem {
	  return &Mem{inst: map[string][]Instance{}, rr: map[string]*uint64{}}
  }

   func (m *Mem) Kind() string { return "mem" }

   func (m *Mem) Register(_ context.Context, inst Instance) error {
	   if m == nil {
		  return ErrClosed
	   }
	   inst.Name = strings.TrimSpace(inst.Name)
	   inst.Addr = strings.TrimSpace(inst.Addr)
	  if inst.Name == "" || inst.Addr == "" {
		   return ErrEndpoint
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cur := m.inst[inst.Name]
	   out := make([]Instance, 0, len(cur)+1)
	   for _, it := range cur {
		   if it.Addr != inst.Addr {
			  out = append(out, it)
		   }
	  }
	   m.inst[inst.Name] = append(out, inst)
	  if m.rr[inst.Name] == nil {
		  n := uint64(0)
		   m.rr[inst.Name] = &n
	   }
	   return nil
  }

  func (m *Mem) Deregister(_ context.Context, inst Instance) error {
	   if m == nil {
		   return ErrClosed
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cur := m.inst[inst.Name]
	  out := cur[:0]
	   for _, it := range cur {
		  if it.Addr != inst.Addr {
			   out = append(out, it)
		  }
	  }
	   m.inst[inst.Name] = out
	  return nil
   }

   func (m *Mem) Watch(name string) []Instance {
	  if m == nil {
		   return nil
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  cur := m.inst[name]
	   out := make([]Instance, len(cur))
	  copy(out, cur)
	  return out
  }

   func (m *Mem) Pick(name string) (Instance, error) {
	   for _, n := range AliasNames(name) {
		   list := m.Watch(n)
		  if len(list) == 0 {
			  continue
		  }
		   return pickP2C(list), nil
	   }
	   return Instance{}, ErrNoInstance
  }

  func (m *Mem) Close() error { return nil }

// FailClosed 端点已配置但客户端无法建立。Register/Pick 返回错误，不伪造实例。

type FailClosed struct{ err error }

   func NewFailClosed(err error) *FailClosed {
	  if err == nil {
		   err = ErrEndpoint
	   }
	   return &FailClosed{err: err}
  }

  func (f *FailClosed) Kind() string { return "etcd" }

   func (f *FailClosed) Register(context.Context, Instance) error {
	  if f == nil || f.err == nil {
		   return ErrEndpoint
	   }
	   return f.err
   }

  func (f *FailClosed) Deregister(context.Context, Instance) error {
	  if f == nil || f.err == nil {
		  return ErrEndpoint
	   }
	  return f.err
  }

   func (f *FailClosed) Watch(string) []Instance { return nil }

  func (f *FailClosed) Pick(string) (Instance,
	  error) {
	   return Instance{}, ErrNoInstance
  }

   func (f *FailClosed) Close() error { return nil }

// Key 拼注册前缀 /aa/svc/<ns>/<env>/<name>。
   func Key(ns, env, name string) string {
	  ns = strings.TrimSpace(ns)
	   if ns == "" {
		  ns = "account-assets"
	  }
	   env = strings.TrimSpace(env)
	   if env == "" {
		  env = "production"
	   }
	  return "/aa/svc/" + ns + "/" + env + "/" + strings.TrimSpace(name)
   }

// SleepBackoff 注册/拨号共用指数上限退避。
  func SleepBackoff(attempt int) time.Duration {
	   if attempt < 1 {
		  attempt = 1
	  }
	   d := time.Duration(attempt) * 200 * time.Millisecond
	  if d > 3*time.Second {
		  return 3 * time.Second
	  }
	   return d
   }
