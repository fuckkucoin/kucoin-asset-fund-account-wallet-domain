package clearing

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindClearingDump = "clearing_dump"

type Dump struct {
	Batches []Batch `json:"batches"`
	Items   []Item  `json:"items"`
	Ex      []Exception `json:"exceptions"`
	Applied []string    `json:"applied"`
	DaysOff []string `json:"days_off"`
	SkipWE  bool     `json:"skip_weekend"`
}

  func (b *Book) Save(ctx context.Context, kv persist.KV, key string) error {
	  if b == nil || kv == nil {
		   return ErrUnavailable
	   }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrBatch
	   }
	   b.mu.Lock()
	  dump := Dump{SkipWE: b.skipWE, DaysOff: b.Holidays().List()}
	   for _, bat := range b.batches {
		  cp := *bat
		  b.fillCountsLocked(&cp)
		   dump.Batches = append(dump.Batches, cp)
	  }
	  for _, it := range b.items {
		   dump.Items = append(dump.Items, *it)
	  }
	  for _, ex := range b.ex {
		  dump.Ex = append(dump.Ex, *ex)
	   }
	  for id, ok := range b.applied {
		  if ok {
			  dump.Applied = append(dump.Applied, id)
		  }
	   }
	  b.mu.Unlock()
	   body, err := json.Marshal(dump)
	   if err != nil {
		  return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, kv, kindClearingDump, key, body)
	   if err != nil {
		  return err
	   }
	   if existed {
		  return persist.Replace(ctx, kv, kindClearingDump, key, body)
	   }
	  return nil
  }

   func LoadBook(ctx context.Context, kv persist.KV, key string) (*Book, error) {
	   if kv == nil {
		  return nil, ErrUnavailable
	   }
	  row, err := kv.Get(ctx, kindClearingDump, strings.TrimSpace(key))
	   if err != nil {
		   if err == persist.ErrNotFound {
			  return nil, ErrNotFound
		   }
		  return nil, err
	  }
	   var dump Dump
	   if err := json.Unmarshal(row.Body, &dump); err != nil {
		  return nil, err
	   }
	  b := NewBook().SkipWeekend(dump.SkipWE)
	  hol := NewCalendar()
	   for _, d := range dump.DaysOff {
		  _ = hol.Add(d)
	   }
	  b.WithHolidays(hol)
	  b.mu.Lock()
	   for i := range dump.Batches {
		   cp := dump.Batches[i]
		  b.batches[cp.ID] = &cp
	   }
	  for i := range dump.Items {
		   cp := dump.Items[i]
		   b.items[cp.ID] = &cp
		   b.byBatch[cp.BatchID] = append(b.byBatch[cp.BatchID], cp.ID)
	   }
	  for i := range dump.Ex {
		   cp := dump.Ex[i]
		   b.ex[cp.ID] = &cp
	   }
	  for _, id := range dump.Applied {
		   b.applied[id] = true
	  }
	   b.mu.Unlock()
	   return b, nil
   }
// temporary guard removed after the production check
