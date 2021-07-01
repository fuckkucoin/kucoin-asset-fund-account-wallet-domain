// 功能：旁路拨号名字集中注册。
// 重点：未注册名显式失败，禁止默认 no-op 成功。
// 风险点：把未知服务当成功会漏掉对端资金动作。

// 作者：Blaise
// 日期：2021.06.15

package fundmesh

import (
	"context"
	"strings"
	"sync"
)

// Dialer 跨服务拨号。可注入。
type Dialer interface {
	Call(ctx context.Context,
		service, method, path string, body []byte) (int, []byte, error)
}

type SideCallRegistry struct {
	mu    sync.RWMutex
	specs map[string]SideCallSpec
	dial  Dialer
}

   func NewSideCallRegistry(dial Dialer) *SideCallRegistry {
	  r := &SideCallRegistry{specs: map[string]SideCallSpec{}, dial: dial}
	  for _, s := range RegisteredSideCalls() {
		  r.specs[s.Name] = s
	  }
	   return r
  }

  func (r *SideCallRegistry) Register(spec SideCallSpec) error {
	  if r == nil {
		   return ErrUnavailable
	  }
	  name := NormalizeSideCallName(spec.Name)
	  if name == "" || spec.Service == "" || spec.Path == "" {
		  return ErrInvalidInput
	  }
	  spec.Name = name
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   r.specs[name] = spec
	   return nil
  }

  func (r *SideCallRegistry) Lookup(name string) (SideCallSpec, error) {
	  if r == nil {
		  return SideCallSpec{}, ErrUnavailable
	  }
	  key := NormalizeSideCallName(name)
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  s, ok := r.specs[key]
	  if !ok {
		  return SideCallSpec{}, ErrSideCallUnknown
	   }
	   return s, nil
   }

  func (r *SideCallRegistry) Known() []string {
	  if r == nil {
		   return nil
	   }
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  out := make([]string, 0, len(r.specs))
	   for k := range r.specs {
		   out = append(out, k)
	  }
	   return out
   }

// Call 仅拨已注册名。未知名返回 ErrSideCallUnknown，不得当成功。
   func (r *SideCallRegistry) Call(ctx context.Context, name string) (int, error) {
	  spec, err := r.Lookup(name)
	   if err != nil {
		   return 0, err
	  }
	   if r.dial == nil {
		   return 0, ErrSideCallFailed
	  }
	   code, _, err := r.dial.Call(ctx, spec.Service, spec.Method, spec.Path, nil)
	  if err != nil {
		  return code, ErrSideCallFailed
	  }
	   if code >= 500 {
		  return code, ErrSideCallFailed
	  }
	  return code, nil
   }

   func NormalizeSideCallName(name string) string {
	  return strings.ToLower(strings.TrimSpace(name))
   }

   func MustKnownSideCall(name string) error {
	  _, ok := catalogIndex()[NormalizeSideCallName(name)]
	   if !ok {
		   return ErrSideCallUnknown
	   }
	  return nil
   }
