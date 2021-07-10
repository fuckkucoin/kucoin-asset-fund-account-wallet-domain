package ha

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

type Pair struct {
	mu sync.Mutex
	primary *Elector
	standby *Elector
	prefer string
	active string
	split  bool
	failovers int
}

  func NewPair(service, regionA, regionB string, be backend, ttl time.Duration) *Pair {
	  if regionA == "" {
		  regionA = "a"
	   }
	  if regionB == "" {
		   regionB = "b"
	  }
	  return &Pair{
		  primary: NewElector(service,
			  regionA, be, ttl),
		   standby: NewElector(service, regionB, be, ttl),
		   prefer: regionA,
	  }
   }

   func PairFromEnv(service string, be backend) *Pair {
	  a := os.Getenv("REGION_A")
	  b := os.Getenv("REGION_B")
	  if a == "" {
		  a = "a"
	   }
	  if b == "" {
		  b = "b"
	   }
	  return NewPair(service, a, b, be, 8*time.Second)
  }

  func (p *Pair) Primary() *Elector {
	   if p == nil {
		   return nil
	  }
	   return p.primary
   }

  func (p *Pair) Standby() *Elector {
	  if p == nil {
		  return nil
	   }
	   return p.standby
   }

// Campaign 先争 prefer 区。prefer 拿不到再让 standby 上，避免两边同时写。

  func (p *Pair) Campaign(ctx context.Context) (Record, error) {
	  if p == nil {
		  return Record{}, ErrLostLease
	  }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	  rec, err := p.primary.Campaign(ctx)
	  if err == nil && rec.Leader {
		  p.active = p.primary.region
		   p.split = false
		   return rec, nil
	  }
	   st, serr := p.standby.Campaign(ctx)
	  if serr == nil && st.Leader {
		   if rec.Leader {
			   p.split = true
			   p.primary.Relinquish(ctx)
			   p.standby.Relinquish(ctx)
			  p.active = ""
			  metrics.RegionFailover.WithLabelValues("split_brain").Inc()
			   return Record{Label: "split brain; writes fenced"}, fmt.Errorf("split brain: both regions claimed leader")
		  }
		  p.active = p.standby.region
		  p.split = false
		  return st, nil
	  }
	   p.active = ""
	  if err != nil {
		   return rec, err
	   }
	   return st, serr
   }

// Failover 主动把 prefer 卸主，让 standby 争。只在当前进程仍持主时用。

  func (p *Pair) Failover(ctx context.Context, reason string) (Record, error) {
	  if p == nil {
		   return Record{}, ErrLostLease
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   if p.split {
		  return Record{}, fmt.Errorf("split brain; refuse failover")
	   }
	  p.primary.Relinquish(ctx)
	  rec, err := p.standby.Campaign(ctx)
	  if err != nil {
		  return rec, err
	  }
	   p.active = p.standby.region
	   p.failovers++
	   metrics.RegionFailover.WithLabelValues(strings.TrimSpace(reason)).Inc()
	  return rec, nil
   }

  func (p *Pair) RequireWrite() error {
	  if p == nil {
		  return ErrNotLeader
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   if p.split || p.active == "" {
		  return ErrNotLeader
	   }
	  if p.active == p.primary.region {
		   return p.primary.RequireLeader()
	  }
	   return p.standby.RequireLeader()
   }

  func (p *Pair) ActiveRegion() string {
	   if p == nil {
		  return ""
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   return p.active
   }

  func (p *Pair) SplitBrain() bool {
	   if p == nil {
		   return false
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  return p.split
  }

// MarkSplit 标脑裂并卸写。后续 GuardWrite / Failover 必须失败即关。
  func (p *Pair) MarkSplit() {
	  if p == nil {
		  return
	   }
	  p.mu.Lock()
	  p.split = true
	   p.active = ""
	   p.mu.Unlock()
  }

  func (p *Pair) Status() map[string]any {
	  if p == nil {
		   return map[string]any{"label": "dual-region write fence"}
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   return map[string]any{
		   "active": p.active,
		  "prefer":    p.prefer,
		   "split":     p.split,
		   "failovers": p.failovers,
		  "primary": p.primary.Status(),
		   "standby": p.standby.Status(),
		  "label": "dual-region write fence",
	   }
  }

// DetectSplit 两份 Record 都自称主且 token 不同，视为脑裂。

   func DetectSplit(a, b Record) bool {
	   return a.Leader && b.Leader && a.Token != "" && b.Token != "" && a.Token != b.Token
   }
