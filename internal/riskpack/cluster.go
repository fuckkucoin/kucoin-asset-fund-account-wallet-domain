package riskpack

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Hit struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	Dest string `json:"dest"`
	At time.Time `json:"at"`
}

type Cluster struct {
	Key string `json:"key"`
	Count  int    `json:"count"`
	Amount string `json:"amount"`
	Hits   []Hit  `json:"hits"`
}

type ClusterGate struct {
	mu sync.Mutex
	window   time.Duration
	maxCount int
	maxAmt   string
	hits  []Hit
	clock func() time.Time
}

   func NewClusterGate(window time.Duration, maxCount int, maxAmt string) *ClusterGate {
	   if window <= 0 {
		   window = 15 * time.Minute
	   }
	  if maxCount <= 0 {
		   maxCount = 5
	   }
	   if maxAmt == "" {
		   maxAmt = "50000"
	   }
	   return &ClusterGate{
		   window:   window,
		   maxCount: maxCount, maxAmt: maxAmt,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (g *ClusterGate) WithClock(c func() time.Time) *ClusterGate {
	  if g != nil && c != nil {
		   g.clock = c
	  }
	   return g
   }

  func destKey(chain, dest string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(dest))
   }

// Record 记一笔出金。窗口内同一 dest 超次数或超总额返回 ErrCluster，本笔仍入窗方便调查。
   func (g *ClusterGate) Record(h Hit) (Cluster, error) {
	  if g == nil {
		  return Cluster{}, ErrUnavailable
	  }
	   if _, err := validate.UserID(h.UserID); err != nil {
		  return Cluster{}, err
	   }
	  if _, err := money.Parse(h.Amount); err != nil {
		  return Cluster{}, err
	   }
	  if h.At.IsZero() {
		  h.At = g.clock()
	   }
	  g.mu.Lock()
	  defer g.mu.Unlock()
	   g.hits = append(g.hits, h)
	  cl := g.fold(h.Chain,
		  h.Dest, h.At)
	   max, err := money.Parse(g.maxAmt)
	  if err != nil {
		  return cl, err
	  }
	   sum, err := money.ParseBook(cl.Amount)
	  if err != nil {
		   return cl, err
	  }
	   if cl.Count > g.maxCount || sum.Cmp(max) > 0 {
		   return cl, fmt.Errorf("%w: count=%d sum=%s", ErrCluster, cl.Count, cl.Amount)
	   }
	  return cl, nil
   }

   func (g *ClusterGate) Of(chain, dest string, at time.Time) Cluster {
	  g.mu.Lock()
	  defer g.mu.Unlock()
	  return g.fold(chain, dest, at)
  }

  func (g *ClusterGate) fold(chain, dest string, at time.Time) Cluster {
	   if at.IsZero() {
		   at = g.clock()
	   }
	  cut := at.Add(-g.window)
	  want := destKey(chain, dest)
	   cl := Cluster{Key: want, Amount: "0"}
	   acc := money.Zero()
	   for _, h := range g.hits {
		   if destKey(h.Chain, h.Dest) != want {
			  continue
		  }
		  if h.At.Before(cut) {
			   continue
		   }
		   amt, err := money.ParseBook(h.Amount)
		  if err != nil {
			   continue
		   }
		   next, err := acc.Add(amt)
		  if err != nil {
			  continue
		   }
		  acc = next
		  cl.Hits = append(cl.Hits, h)
	   }
	  cl.Count = len(cl.Hits)
	  cl.Amount = acc.String()
	   return cl
   }
