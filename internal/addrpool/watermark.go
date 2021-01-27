package addrpool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const (
	kindPoolWater  = "addrpool.watermark"
	kindPoolTicket = "addrpool.topup.ticket"
	defaultMinFree = 4
)

// Watermark 单资产最低空闲。

type Watermark struct {
	Asset string `json:"asset"`
	Chain string `json:"chain,omitempty"`
	MinFree int `json:"min_free"`
}

// TopupTicket 自动补给工单。
type TopupTicket struct {
	ID string `json:"id"`
	Asset string `json:"asset"`
	Chain   string `json:"chain"`
	Free    int    `json:"free"`
	MinFree int    `json:"min_free"`
	Need int `json:"need"`
	Reason string `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// WaterBook 资产水位表。
type WaterBook struct {
	mu   sync.RWMutex
	rows map[string]Watermark
	kv persist.KV
}

  func NewWaterBook(kv persist.KV) *WaterBook {
	   return &WaterBook{rows: map[string]Watermark{}, kv: kv}
   }

   func waterKey(asset, chain string) string {
	   a := strings.ToUpper(strings.TrimSpace(asset))
	   c := strings.ToUpper(strings.TrimSpace(chain))
	  if a == "" {
		   a = "DEFAULT"
	   }
	   return a + "|" + c
   }

   func (w *WaterBook) Put(ctx context.Context, row Watermark) error {
	   if w == nil {
		  return fmt.Errorf("addrpool watermark unavailable")
	   }
	  asset := strings.ToUpper(strings.TrimSpace(row.Asset))
	   if asset != "" {
		   got, err := validate.Asset(asset)
		  if err != nil {
			  return err
		   }
		  asset = got
	   } else {
		  asset = "DEFAULT"
	  }
	   chain := strings.ToUpper(strings.TrimSpace(row.Chain))
	  if chain != "" {
		  ch, err := validate.Chain(chain)
		  if err != nil {
			   return err
		   }
		   chain = ch
	  }
	  if row.MinFree < 0 {
		   row.MinFree = 0
	  }
	   row.Asset, row.Chain = asset, chain
	  w.mu.Lock()
	   if w.rows == nil {
		  w.rows = map[string]Watermark{}
	  }
	  w.rows[waterKey(asset, chain)] = row
	  w.mu.Unlock()
	  if w.kv == nil {
		   return nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   body, err := json.Marshal(row)
	   if err != nil {
		  return err
	  }
	  key := waterKey(asset, chain)
	  _, existed, err := persist.GetOrInit(ctx, w.kv, kindPoolWater, key, body)
	  if err != nil {
		   return err
	  }
	  if existed {
		   return persist.Replace(ctx, w.kv, kindPoolWater, key, body)
	  }
	  return nil
   }

   func (w *WaterBook) Lookup(asset, chain string) Watermark {
	  if w == nil {
		  return Watermark{Asset: strings.ToUpper(strings.TrimSpace(asset)), MinFree: defaultMinFree}
	   }
	  w.mu.RLock()
	  defer w.mu.RUnlock()
	   for _, k := range []string{
		  waterKey(asset, chain),
		   waterKey(asset, ""),
		  waterKey("DEFAULT", chain),
		   waterKey("DEFAULT", ""),
	   } {
		  if r, ok := w.rows[k]; ok {
			  return r
		  }
	   }
	   return Watermark{Asset: strings.ToUpper(strings.TrimSpace(asset)), Chain: strings.ToUpper(strings.TrimSpace(chain)), MinFree: defaultMinFree}
  }

  func (w *WaterBook) List() []Watermark {
	  if w == nil {
		   return nil
	   }
	   w.mu.RLock()
	   defer w.mu.RUnlock()
	   out := make([]Watermark, 0, len(w.rows))
	  for _, r := range w.rows {
		   out = append(out, r)
	   }
	  return out
   }

   func (w *WaterBook) Reload(ctx context.Context) error {
	  if w == nil || w.kv == nil {
		   return nil
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   rows, err := w.kv.List(ctx, kindPoolWater, 400)
	   if err != nil {
		   return err
	   }
	  next := map[string]Watermark{}
	  for _, row := range rows {
		   var m Watermark
		   if json.Unmarshal(row.Body, &m) != nil {
			   continue
		  }
		   next[waterKey(m.Asset, m.Chain)] = m
	  }
	  w.mu.Lock()
	   w.rows = next
	  w.mu.Unlock()
	  return nil
  }

// InspectWatermark 读水位；低于 min-free 开补给工单。

   func InspectWatermark(inv *Inventory, shared *SharedMemoPool, water *WaterBook, asset, chain string) (TopupView, *TopupTicket) {
	   mark := Watermark{MinFree: defaultMinFree}
	  if water != nil {
		  mark = water.Lookup(asset, chain)
	   }
	  view := InspectTopup(inv, shared, chain, mark.MinFree)
	   if !view.Request {
		   return view, nil
	   }
	   ticket := &TopupTicket{
		   ID: uuid.NewString(), Asset: strings.ToUpper(strings.TrimSpace(asset)),
		  Chain: strings.ToUpper(strings.TrimSpace(chain)),
		  Free:    view.Free,
		  MinFree: mark.MinFree, Need: view.Need, Reason: "below_watermark",
		  CreatedAt: time.Now().UTC(),
	  }
	  if water != nil {
		  _ = water.openTicket(context.Background(), *ticket)
	  }
	  return view, ticket
  }

   func (w *WaterBook) openTicket(ctx context.Context, t TopupTicket) error {
	   if w == nil || w.kv == nil {
		   return nil
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   body, err := json.Marshal(t)
	  if err != nil {
		   return err
	   }
	   metrics.FundGates.WithLabelValues("addrpool", "watermark_ticket").Inc()
	   _, _, err = persist.GetOrInit(ctx, w.kv, kindPoolTicket, t.ID, body)
	   return err
   }

   func (w *WaterBook) ListTickets(ctx context.Context, limit int) []TopupTicket {
	  if w == nil || w.kv == nil {
		   return []TopupTicket{}
	  }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  if limit <= 0 || limit > 200 {
		   limit = 50
	   }
	  rows, err := w.kv.List(ctx, kindPoolTicket, limit)
	   if err != nil {
		  return []TopupTicket{}
	  }
	  out := make([]TopupTicket, 0, len(rows))
	  for i := len(rows) - 1; i >= 0; i-- {
		   var t TopupTicket
		  if json.Unmarshal(rows[i].Body, &t) != nil {
			  continue
		  }
		   out = append(out, t)
	  }
	   return out
  }
