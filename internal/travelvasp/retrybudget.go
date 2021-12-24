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
	kindRetryBudget = "travel.retry.budget"
	kindDeadLetter = "travel.retry.dead"
	defaultBudget  = 16
)

// CorridorBudget 走廊重试额度。
type CorridorBudget struct {
	Corridor string `json:"corridor"`
	Max      int    `json:"max"`
	Used     int    `json:"used"`
	Remain   int    `json:"remain"`
	Updated time.Time `json:"updated_at"`
}

// DeadLetter 死信条。

type DeadLetter struct {
	CaseID string `json:"case_id"`
	Corridor string `json:"corridor"`
	Peer     string `json:"peer"`
	Last     string `json:"last,omitempty"`
	Tries int       `json:"tries"`
	At    time.Time `json:"at"`
}

// RetryBook 走廊预算簿。

type RetryBook struct {
	mu sync.Mutex
	rows map[string]CorridorBudget
	kv   persist.KV
}

   func NewRetryBook(kv persist.KV) *RetryBook {
	   return &RetryBook{rows: map[string]CorridorBudget{},
		   kv: kv}
   }

   func (s *Service) BindRetry(r *RetryBook) *Service {
	  if s != nil && r != nil {
		  s.retry = r
	  }
	   return s
  }

   func budgetKey(corridor string) string {
	  return strings.ToUpper(strings.TrimSpace(corridor))
   }

   func (r *RetryBook) inspect(corridor string) CorridorBudget {
	  key := budgetKey(corridor)
	  if key == "" {
		  key = "DEFAULT"
	  }
	  if r == nil {
		  return CorridorBudget{Corridor: key, Max: defaultBudget, Remain: defaultBudget}
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  if cur, ok := r.rows[key]; ok {
		   cur.Remain = cur.Max - cur.Used
		  if cur.Remain < 0 {
			  cur.Remain = 0
		   }
		   return cur
	   }
	   if r.kv != nil {
		  row, err := r.kv.Get(context.Background(), kindRetryBudget, key)
		  if err == nil {
			   var cur CorridorBudget
			  if json.Unmarshal(row.Body, &cur) == nil {
				  cur.Remain = cur.Max - cur.Used
				   if cur.Remain < 0 {
					  cur.Remain = 0
				  }
				  r.rows[key] = cur
				  return cur
			   }
		   }
	   }
	   cur := CorridorBudget{Corridor: key,
		   Max: defaultBudget, Remain: defaultBudget}
	  r.rows[key] = cur
	   return cur
   }

   func (r *RetryBook) persist(cur CorridorBudget) {
	  if r == nil {
		   return
	  }
	  cur.Updated = time.Now().UTC()
	   cur.Remain = cur.Max - cur.Used
	  if cur.Remain < 0 {
		  cur.Remain = 0
	   }
	  r.mu.Lock()
	  r.rows[cur.Corridor] = cur
	  r.mu.Unlock()
	  if r.kv == nil {
		  return
	   }
	   body, err := json.Marshal(cur)
	  if err != nil {
		  return
	   }
	  _, existed, err := persist.GetOrInit(context.Background(), r.kv, kindRetryBudget, cur.Corridor, body)
	  if err != nil {
		  return
	  }
	   if existed {
		   _ = persist.Replace(context.Background(), r.kv, kindRetryBudget, cur.Corridor, body)
	   }
   }

// ConsumeBudget 消费一次走廊预算。耗尽返回错误。

  func (r *RetryBook) ConsumeBudget(corridor string) error {
	  cur := r.inspect(corridor)
	   if cur.Max <= 0 {
		   cur.Max = defaultBudget
	   }
	   if cur.Used >= cur.Max {
		   return ErrRetryBudget
	   }
	  cur.Used++
	  r.persist(cur)
	  return nil
   }

  func (s *Service) consumeRetryBudget(corridor string) error {
	   if s == nil || s.retry == nil {
		  return nil
	   }
	   return s.retry.ConsumeBudget(corridor)
  }

   func (s *Service) InspectRetryBudget(corridor string) CorridorBudget {
	  if s == nil || s.retry == nil {
		  return CorridorBudget{Corridor: budgetKey(corridor), Max: defaultBudget, Remain: defaultBudget}
	   }
	   return s.retry.inspect(corridor)
   }

   func (s *Service) putDeadLetter(ctx context.Context, item OutboxItem, corridor string) {
	  if s == nil || s.outbox == nil || s.outbox.kv == nil {
		  return
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  dl := DeadLetter{
		  CaseID: item.CaseID, Corridor: budgetKey(corridor), Peer: item.Peer,
		   Last: item.Last, Tries: item.Tries, At: time.Now().UTC(),
	   }
	   body, err := json.Marshal(dl)
	   if err != nil {
		  return
	  }
	  _,
		  existed, err := persist.GetOrInit(ctx, s.outbox.kv, kindDeadLetter, item.CaseID, body)
	  if err == nil && existed {
		   _ = persist.Replace(ctx, s.outbox.kv, kindDeadLetter, item.CaseID, body)
	   }
	   metrics.FundGates.WithLabelValues("travel", "dead_letter").Inc()
	   slo.FireMiddlewareAlertDetail("travel.retry.dead", float64(item.Tries), float64(outboxMaxTry), item.CaseID)
   }

// ExportDeadLetters 导出死信。只读。
   func (s *Service) ExportDeadLetters(ctx context.Context, limit int) ([]DeadLetter, error) {
	   if s == nil || s.outbox == nil || s.outbox.kv == nil {
		  return []DeadLetter{}, nil
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	   rows, err := s.outbox.kv.List(ctx, kindDeadLetter, limit)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]DeadLetter,
		  0, len(rows))
	  for _, row := range rows {
		  var dl DeadLetter
		  if json.Unmarshal(row.Body, &dl) != nil {
			  continue
		   }
		   out = append(out, dl)
	  }
	  return out, nil
   }
