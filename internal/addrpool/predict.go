package addrpool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

const (
	kindAssignRate = "addrpool.assign.rate"
	predictDays    = 7
	predictHorizon = 2
)

// RateBook 近 7 日领址计数。
type RateBook struct {
	mu sync.Mutex
	kv persist.KV
}

   func NewRateBook(kv persist.KV) *RateBook {
	  return &RateBook{kv: kv}
  }

type dayCount struct {
	Chain string `json:"chain"`
	Day   string `json:"day"`
	N     int    `json:"n"`
}

  func rateKey(chain string, day time.Time) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + day.UTC().Format("2006-01-02")
   }

// NoteAssign 记一笔领址。KV 故障静默，调用方已成功分配。
  func (r *RateBook) NoteAssign(ctx context.Context, chain string) {
	  if r == nil || r.kv == nil {
		  return
	   }
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	  if ch == "" {
		  return
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  now := time.Now().UTC()
	   key := rateKey(ch, now)
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  row, err := r.kv.Get(ctx, kindAssignRate, key)
	   cur := dayCount{Chain: ch, Day: now.Format("2006-01-02")}
	   missing := errors.Is(err, persist.ErrNotFound)
	   if err == nil {
		   _ = json.Unmarshal(row.Body, &cur)
	  } else if !missing {
		   return
	   }
	  cur.N++
	  body, err := json.Marshal(cur)
	  if err != nil {
		  return
	  }
	  if missing {
		  _, _, _ = persist.GetOrInit(ctx, r.kv, kindAssignRate, key, body)
		   return
	   }
	   _ = persist.Replace(ctx, r.kv, kindAssignRate, key, body)
  }

// Sum7d 近 7 日领址合计。
  func (r *RateBook) Sum7d(ctx context.Context, chain string) int {
	  if r == nil || r.kv == nil {
		  return 0
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	  now := time.Now().UTC()
	   n := 0
	   for i := 0; i < predictDays; i++ {
		   day := now.AddDate(0, 0, -i)
		  row, err := r.kv.Get(ctx, kindAssignRate, rateKey(ch, day))
		  if err != nil {
			   continue
		  }
		  var cur dayCount
		   if json.Unmarshal(row.Body, &cur) != nil {
			  continue
		  }
		  n += cur.N
	  }
	   return n
   }

// PredictView 预测短缺面。
type PredictView struct {
	Chain      string  `json:"chain"`
	Assign7d   int     `json:"assign_7d"`
	RatePerDay float64 `json:"rate_per_day"`
	Free       int     `json:"free"`
	DaysLeft   float64 `json:"days_left"`
	Short      bool    `json:"short"`
	Horizon    int     `json:"horizon_days"`
	Reason string `json:"reason,omitempty"`
}

// InspectPredict 按 7 日速率与空闲库存估短缺。

  func InspectPredict(inv *Inventory, rates *RateBook, chain string) (PredictView, error) {
	  ch, err := validate.Chain(chain)
	  if err != nil {
		   return PredictView{}, err
	   }
	   view := PredictView{Chain: ch, Horizon: predictHorizon}
	   if inv != nil {
		   view.Free = inv.Counts(ch).Free
	  }
	  if rates != nil {
		   view.Assign7d = rates.Sum7d(context.Background(), ch)
	  }
	   if view.Assign7d > 0 {
		   view.RatePerDay = float64(view.Assign7d) / float64(predictDays)
	  }
	   if wallet.RequiresFamMemo(ch) {
		   view.Reason = "fam_shared"
		  return view, nil
	  }
	  if view.RatePerDay <= 0 {
		  return view, nil
	   }
	  view.DaysLeft = float64(view.Free) / view.RatePerDay
	   if view.DaysLeft < float64(predictHorizon) {
		  view.Short = true
		  view.Reason = "predictive_shortage"
		   metrics.FundGates.WithLabelValues("addrpool", "predict_short").Inc()
		  slo.FireMiddlewareAlertDetail("addrpool.predict", view.DaysLeft, float64(predictHorizon), ch)
	   }
	  return view, nil
  }
