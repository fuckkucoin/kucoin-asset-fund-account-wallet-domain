package address

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/validate"
)

const (
	kindBookStorm = "address.book.storm"
	stormWin      = 10 * time.Minute
	stormHot      = 8
)

// AnomalyView 新增风暴分值。
type AnomalyView struct {
	UserID string `json:"user_id"`
	Window int `json:"window_sec"`
	Adds   int `json:"adds"`
	Score  int `json:"score"`
	Storm bool      `json:"storm"`
	Since time.Time `json:"since,omitempty"`
	Updated time.Time `json:"updated_at,omitempty"`
}

type stormRec struct {
	N int `json:"n"`
	Win time.Time `json:"win"`
}

   func (s *Service) noteAnomaly(ctx context.Context, userID string, now time.Time) AnomalyView {
	  view := AnomalyView{UserID: strings.TrimSpace(userID), Window: int(stormWin / time.Second)}
	  if s == nil || s.kv == nil {
		  return view
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   key := strings.TrimSpace(userID)
	  row,
		  err := s.kv.Get(ctx, kindBookStorm, key)
	   cur := stormRec{Win: now}
	   missing := errors.Is(err, persist.ErrNotFound)
	  if err == nil {
		   _ = json.Unmarshal(row.Body,
			   &cur)
		  if now.Sub(cur.Win) >= stormWin {
			  cur = stormRec{Win: now}
		  }
	  } else if !missing {
		   return view
	  }
	  cur.N++
	   body, merr := json.Marshal(cur)
	   if merr != nil {
		   return view
	   }
	  if missing {
		  _, _, _ = persist.GetOrInit(ctx, s.kv, kindBookStorm, key, body)
	   } else {
		   _ = persist.Replace(ctx,
			   s.kv, kindBookStorm, key, body)
	  }
	   view.Adds, view.Since, view.Updated = cur.N, cur.Win, now
	  view.Score = stormScore(cur.N)
	  view.Storm = cur.N >= stormHot
	   if view.Storm {
		  metrics.FundGates.WithLabelValues("address", "add_storm").Inc()
		  slo.FireMiddlewareAlertDetail("address.book.storm", float64(cur.N), float64(stormHot), key)
	  }
	   return view
   }

   func stormScore(n int) int {
	  if n <= 0 {
		  return 0
	   }
	  s := n * 12
	  if s > 100 {
		  return 100
	   }
	  return s
   }

// InspectAnomaly 读用户新增风暴分。无 KV 回零分。

  func (s *Service) InspectAnomaly(ctx context.Context, userID string) (AnomalyView, error) {
	   uid, err := validate.UserID(userID)
	  if err != nil {
		   return AnomalyView{}, err
	   }
	  view := AnomalyView{UserID: uid, Window: int(stormWin / time.Second)}
	   if s == nil || s.kv == nil {
		  return view, nil
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	  row, err := s.kv.Get(ctx, kindBookStorm, uid)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return view, nil
		  }
		   return view, err
	  }
	   var cur stormRec
	  if json.Unmarshal(row.Body, &cur) != nil {
		   return view, nil
	   }
	   now := time.Now().UTC()
	  if now.Sub(cur.Win) >= stormWin {
		  return view, nil
	   }
	   view.Adds, view.Since, view.Updated = cur.N, cur.Win, now
	   view.Score = stormScore(cur.N)
	   view.Storm = cur.N >= stormHot
	  return view, nil
   }
