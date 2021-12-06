package svcdial

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
)

// Guarded 熔断拨号客户端。
type Guarded struct {
	inner  *Client
	mu     sync.Mutex
	circ   map[string]*Circuit
	region string
	thresh int
	reset time.Duration
}

   func NewGuarded(reg svcreg.Registry, log zerolog.Logger) *Guarded {
	  return &Guarded{
		   inner: New(reg, log),
		  circ:  map[string]*Circuit{},
		  thresh: 5,
		  reset: 2 * time.Second,
	  }
  }

// Prefer 设置区位偏好（sg / tyo）。
   func (g *Guarded) Prefer(region string) *Guarded {
	  if g != nil {
		   g.region = strings.ToLower(strings.TrimSpace(region))
	  }
	  return g
  }

  func (g *Guarded) circuit(svc string) *Circuit {
	   g.mu.Lock()
	  defer g.mu.Unlock()
	   c := g.circ[svc]
	  if c == nil {
		   c = NewCircuit(g.thresh, g.reset)
		   g.circ[svc] = c
	  }
	  return c
   }

// Call 经熔断拨号。打开时返回错误。

   func (g *Guarded) Call(ctx context.Context, service, method, path string, body []byte) (int, []byte, error) {
	  if g == nil || g.inner == nil {
		   return 0, nil, svcreg.ErrClosed
	  }
	   c := g.circuit(service)
	  if !c.Allow() {
		   return 0, nil, fmt.Errorf("svcdial circuit open for %s", service)
	   }
	   _ = g.region
	  code, raw, err := g.inner.Call(ctx, service, method, path, body)
	  if err != nil {
		   c.Failure()
		  return code, raw, err
	  }
	   if code >= 500 {
		  c.Failure()
		  return code, raw, fmt.Errorf("svcdial upstream %d", code)
	  }
	   c.Success()
	  return code, raw, nil
  }

// CircuitOpen 只读熔断态。

  func (g *Guarded) CircuitOpen(service string) bool {
	  if g == nil {
		   return false
	  }
	   return g.circuit(service).Open()
   }
