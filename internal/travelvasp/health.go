package travelvasp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/slo"
)

const (
	kindVASPHealth = "travel.vasp.health"
	healthOpenAt   = 20
	healthFailOpen = 5
	healthPause    = 2 * time.Minute
)

// HealthScore 对手健康面。

type HealthScore struct {
	Code     string `json:"code"`
	Score    int    `json:"score"`
	Success  int    `json:"success"`
	Failures int    `json:"failures"`
	Streak int  `json:"fail_streak"`
	Open   bool `json:"open"`
	OpenUntil time.Time `json:"open_until,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HealthBook 对手健康簿。
type HealthBook struct {
	mu   sync.Mutex
	rows map[string]HealthScore
	kv   persist.KV
}

  func NewHealthBook(kv persist.KV) *HealthBook {
	   return &HealthBook{rows: map[string]HealthScore{},
		   kv: kv}
  }

   func (s *Service) BindHealth(h *HealthBook) *Service {
	   if s != nil && h != nil {
		   s.health = h
	  }
	   return s
   }

  func (h *HealthBook) key(code string) string {
	  return strings.ToUpper(strings.TrimSpace(code))
  }

  func (h *HealthBook) load(code string) HealthScore {
	   code = h.key(code)
	   if h == nil || code == "" {
		  return HealthScore{Code: code, Score: 100}
	   }
	   h.mu.Lock()
	   defer h.mu.Unlock()
	   if cur, ok := h.rows[code]; ok {
		   return cur
	  }
	  if h.kv != nil {
		  row,
			  err := h.kv.Get(context.Background(), kindVASPHealth, code)
		   if err == nil {
			  var cur HealthScore
			   if json.Unmarshal(row.Body, &cur) == nil {
				  h.rows[code] = cur
				   return cur
			   }
		  }
	  }
	  cur := HealthScore{Code: code, Score: 100}
	   h.rows[code] = cur
	   return cur
   }

  func (h *HealthBook) save(cur HealthScore) {
	  if h == nil {
		   return
	   }
	  cur.UpdatedAt = time.Now().UTC()
	   h.mu.Lock()
	  h.rows[cur.Code] = cur
	  h.mu.Unlock()
	   if h.kv == nil {
		   return
	   }
	  body, err := json.Marshal(cur)
	   if err != nil {
		   return
	   }
	  _, existed, err := persist.GetOrInit(context.Background(), h.kv, kindVASPHealth, cur.Code, body)
	  if err != nil {
		   return
	  }
	   if existed {
		  _ = persist.Replace(context.Background(), h.kv, kindVASPHealth, cur.Code, body)
	  }
  }

   func clipScore(n int) int {
	   if n < 0 {
		   return 0
	   }
	   if n > 100 {
		  return 100
	  }
	  return n
  }

   func (h *HealthBook) Record(code string, ok bool) HealthScore {
	  cur := h.load(code)
	   if ok {
		  cur.Success++
		  cur.Streak = 0
		   cur.Score = clipScore(cur.Score + 5)
		  if cur.Open && time.Now().UTC().After(cur.OpenUntil) {
			  cur.Open = false
			   cur.OpenUntil = time.Time{}
		   }
	   } else {
		   cur.Failures++
		   cur.Streak++
		   cur.Score = clipScore(cur.Score - 15)
		   if cur.Score <= healthOpenAt || cur.Streak >= healthFailOpen {
			  cur.Open = true
			   cur.OpenUntil = time.Now().UTC().Add(healthPause)
			  metrics.FundGates.WithLabelValues("travel", "peer_circuit").Inc()
			  slo.FireMiddlewareAlertDetail("travel.peer.circuit", float64(cur.Score), float64(healthOpenAt), cur.Code)
		  }
	  }
	   h.save(cur)
	   return cur
  }

   func (h *HealthBook) Inspect(code string) HealthScore {
	  cur := h.load(code)
	  if cur.Open && !cur.OpenUntil.After(time.Now().UTC()) {
		  cur.Open = false
		  cur.OpenUntil = time.Time{}
		  h.save(cur)
	   }
	   return cur
   }

// RequirePeerHealth 熔断开启则失败关闭。LOCAL/空码放行。
  func (s *Service) RequirePeerHealth(code, proto string) error {
	   if s == nil || s.health == nil {
		   return nil
	  }
	  if proto == ProtoLOCAL || proto == ProtoUNHOSTED || strings.TrimSpace(code) == "" {
		  return nil
	   }
	  got := s.health.Inspect(code)
	  if got.Open {
		  return ErrPeerHealth
	   }
	  return nil
   }

   func (s *Service) RecordPeer(code string, ok bool) HealthScore {
	   if s == nil || s.health == nil {
		   return HealthScore{Code: strings.ToUpper(strings.TrimSpace(code)), Score: 100}
	   }
	   return s.health.Record(code, ok)
  }

   func (s *Service) InspectPeerHealth(code string) HealthScore {
	  if s == nil || s.health == nil {
		  return HealthScore{Code: strings.ToUpper(strings.TrimSpace(code)), Score: 100}
	  }
	   return s.health.Inspect(code)
   }
