package obsdash

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

type BurnAlert struct {
	Name string `json:"name"`
	Target float64 `json:"target"`
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	Budget   float64 `json:"budget"`
	Remaining float64 `json:"remaining"`
	BurnRate float64 `json:"burn_rate"`
	Alert  bool   `json:"alert"`
	Window string `json:"window"`
}

type sloRow struct {
	target float64
	window time.Duration
	requests int64
	errors   int64
}

// Burns 错误预算燃烧。剩余预算 <= 0 或燃烧率 >= 2 告警。

type Burns struct {
	mu   sync.Mutex
	rows map[string]*sloRow
}

   func NewBurns() *Burns {
	  b := &Burns{rows: map[string]*sloRow{}}
	  b.Register("wallet_query", 0.9995, time.Hour)
	  b.Register("chain_submit", 0.999, time.Hour)
	  b.Register("reconcile", 0.999, 24*time.Hour)
	  return b
  }

   func (b *Burns) Register(name string,
	  target float64, window time.Duration) error {
	   if b == nil {
		  return ErrUnavailable
	   }
	  name = strings.TrimSpace(name)
	   if name == "" {
		  return ErrSLO
	   }
	  if target <= 0 || target >= 1 {
		  return ErrBudget
	   }
	  if window <= 0 {
		  return ErrWindow
	   }
	   b.mu.Lock()
	  b.rows[name] = &sloRow{target: target,
		  window: window}
	  b.mu.Unlock()
	   return nil
   }

   func (b *Burns) Observe(name string, failed bool) error {
	   if b == nil {
		   return ErrUnavailable
	  }
	   name = strings.TrimSpace(name)
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   row, ok := b.rows[name]
	   if !ok {
		  return ErrSLO
	  }
	   row.requests++
	  if failed {
		  row.errors++
	   }
	  if burnOf(name, row).Alert {
		   metrics.SLOBurnAlerts.WithLabelValues(name).Inc()
	   }
	  return nil
   }

   func (b *Burns) Alert(name string) (BurnAlert, error) {
	   if b == nil {
		  return BurnAlert{}, ErrUnavailable
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  row,
		   ok := b.rows[strings.TrimSpace(name)]
	   if !ok {
		  return BurnAlert{}, ErrSLO
	   }
	   return burnOf(name, row), nil
   }

  func burnOf(name string, row *sloRow) BurnAlert {
	  budget := 1 - row.target
	   out := BurnAlert{
		   Name: name, Target: row.target, Requests: row.requests, Errors: row.errors,
		  Budget: budget, Window: row.window.String(),
	   }
	   if row.requests == 0 {
		  out.Remaining = budget
		   return out
	  }
	   errRate := float64(row.errors) / float64(row.requests)
	   out.Remaining = budget - errRate
	   if budget > 0 {
		  out.BurnRate = errRate / budget
	   }
	  out.Alert = out.Remaining <= 0 || out.BurnRate >= 2
	   return out
  }

  func (b *Burns) List() []BurnAlert {
	  if b == nil {
		   return nil
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  out := make([]BurnAlert,
		  0, len(b.rows))
	  for name, row := range b.rows {
		   out = append(out, burnOf(name, row))
	   }
	  return out
   }
