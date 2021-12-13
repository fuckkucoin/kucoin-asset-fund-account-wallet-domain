package travelvasp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindTravelSunrise = "travel.sunrise"

// SunriseRow 单法域日出日程。

type SunriseRow struct {
	Jurisdiction string `json:"jurisdiction"`
	Effective    string `json:"effective"`
	MinAmount    string `json:"min_amount,omitempty"`
	Required     bool   `json:"required"`
}

// SunriseBook 法域日出表。
type SunriseBook struct {
	mu sync.RWMutex
	rows map[string]SunriseRow
	kv persist.KV
}

   func NewSunriseBook() *SunriseBook {
	   return &SunriseBook{rows: map[string]SunriseRow{}}
   }

   func (b *SunriseBook) BindKV(kv persist.KV) *SunriseBook {
	  if b != nil && kv != nil {
		  b.kv = kv
	   }
	  return b
   }

  func jurisKey(j string) string {
	   j = strings.ToUpper(strings.TrimSpace(j))
	   if j == "" {
		   j = "DEFAULT"
	  }
	  return j
   }

  func (b *SunriseBook) Put(ctx context.Context, row SunriseRow) error {
	   if b == nil {
		   return ErrUnavailable
	  }
	   row.Jurisdiction = jurisKey(row.Jurisdiction)
	   row.Effective = strings.TrimSpace(row.Effective)
	  row.MinAmount = strings.TrimSpace(row.MinAmount)
	   if row.Effective != "" {
		   if _, err := time.Parse("2006-01-02", row.Effective); err != nil {
			   return fmt.Errorf("travel sunrise date invalid")
		  }
	   }
	   b.mu.Lock()
	  if b.rows == nil {
		   b.rows = map[string]SunriseRow{}
	  }
	   b.rows[row.Jurisdiction] = row
	  b.mu.Unlock()
	  if b.kv == nil {
		  return nil
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   body, err := json.Marshal(row)
	   if err != nil {
		   return err
	  }
	   _, existed, err := persist.GetOrInit(ctx, b.kv, kindTravelSunrise, row.Jurisdiction, body)
	   if err != nil {
		  return err
	   }
	  if existed {
		   return persist.Replace(ctx, b.kv, kindTravelSunrise, row.Jurisdiction, body)
	  }
	  return nil
  }

   func (b *SunriseBook) Lookup(jurisdiction string) (SunriseRow, bool) {
	  if b == nil {
		   return SunriseRow{}, false
	   }
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	   if r, ok := b.rows[jurisKey(jurisdiction)]; ok {
		   return r, true
	   }
	  if r, ok := b.rows["DEFAULT"]; ok {
		   return r, true
	   }
	  return SunriseRow{}, false
  }

   func (b *SunriseBook) List() []SunriseRow {
	  if b == nil {
		   return nil
	  }
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	  out := make([]SunriseRow, 0, len(b.rows))
	   for _, r := range b.rows {
		   out = append(out, r)
	  }
	  return out
   }

  func (b *SunriseBook) Reload(ctx context.Context) error {
	   if b == nil || b.kv == nil {
		  return nil
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   rows, err := b.kv.List(ctx, kindTravelSunrise, 400)
	   if err != nil {
		  return err
	  }
	   next := map[string]SunriseRow{}
	   for _, row := range rows {
		  var r SunriseRow
		   if json.Unmarshal(row.Body, &r) != nil {
			   continue
		   }
		   r.Jurisdiction = jurisKey(r.Jurisdiction)
		  next[r.Jurisdiction] = r
	   }
	  b.mu.Lock()
	  b.rows = next
	  b.mu.Unlock()
	  return nil
   }

// Active 当前时刻该法域是否已日出。未登记视为不强制。
  func (b *SunriseBook) Active(jurisdiction string, now time.Time) (SunriseRow, bool) {
	   row, ok := b.Lookup(jurisdiction)
	  if !ok {
		   return SunriseRow{}, false
	   }
	   if row.Effective == "" {
		  return row, row.Required
	  }
	   day, err := time.Parse("2006-01-02", row.Effective)
	  if err != nil {
		   return row, true
	  }
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   return row, !now.Before(day.UTC())
  }

  func (s *Service) BindSunrise(book *SunriseBook) *Service {
	  if s != nil && book != nil {
		  s.sunrise = book
	  }
	  return s
  }

  func (s *Service) Sunrise() *SunriseBook {
	  if s == nil {
		   return nil
	   }
	  return s.sunrise
  }
