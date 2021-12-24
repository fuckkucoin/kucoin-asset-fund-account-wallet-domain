package treasury

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	TierHot = "hot"
	TierWarm = "warm"
	TierCold = "cold"
)

var (
	ErrDesk = fmt.Errorf("treasury desk unavailable")
	ErrDeskShort = fmt.Errorf("treasury desk short")
	ErrDeskFloor = fmt.Errorf("treasury desk cold floor")
	ErrDeskAsset = fmt.Errorf("treasury desk asset missing")
	ErrDeskTier = fmt.Errorf("treasury desk tier invalid")
)

type Bal struct {
	Asset string `json:"asset"`
	Hot   string `json:"hot"`
	Warm  string `json:"warm"`
	Cold  string `json:"cold"`
	Floor string `json:"cold_floor,omitempty"`
}

type Desk struct {
	mu   sync.Mutex
	rows map[string]*balRow
}

type balRow struct {
	hot, warm, cold money.Amount
	floor money.Amount
}

  func NewDesk() *Desk {
	  return &Desk{rows: map[string]*balRow{}}
  }

   func (d *Desk) Seed(asset, hot, warm, cold string) error {
	   if d == nil {
		   return ErrDesk
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		   return ErrDeskAsset
	   }
	  h,
		   err := money.ParseBook(zero(hot))
	   if err != nil {
		  return err
	   }
	  w, err := money.ParseBook(zero(warm))
	  if err != nil {
		  return err
	  }
	  c,
		  err := money.ParseBook(zero(cold))
	  if err != nil {
		   return err
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   row := d.rows[asset]
	  floor := money.Zero()
	   if row != nil {
		  floor = row.floor
	   }
	  d.rows[asset] = &balRow{hot: h, warm: w, cold: c, floor: floor}
	  return nil
   }

   func (d *Desk) SetFloor(asset, floor string) error {
	   if d == nil {
		  return ErrDesk
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  f, err := money.ParseBook(zero(floor))
	  if err != nil {
		   return err
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   row := d.rows[asset]
	  if row == nil {
		  return ErrDeskAsset
	   }
	  if row.cold.Cmp(f) < 0 {
		   return ErrDeskFloor
	  }
	   row.floor = f
	   return nil
  }

  func (d *Desk) Get(asset string) (Bal, error) {
	  if d == nil {
		  return Bal{}, ErrDesk
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  row := d.rows[asset]
	  if row == nil {
		  return Bal{}, ErrDeskAsset
	   }
	   return Bal{Asset: asset, Hot: row.hot.String(), Warm: row.warm.String(), Cold: row.cold.String(), Floor: row.floor.String()}, nil
   }

  func (d *Desk) List() []Bal {
	  if d == nil {
		   return nil
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  out := make([]Bal, 0, len(d.rows))
	   for asset, row := range d.rows {
		  out = append(out, Bal{Asset: asset, Hot: row.hot.String(), Warm: row.warm.String(), Cold: row.cold.String(), Floor: row.floor.String()})
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].Asset < out[j].Asset })
	   return out
  }

// Move 从 from 划到 to。冷端划出后不得低于地板。
   func (d *Desk) Move(asset, from, to, amount string) error {
	  if d == nil {
		   return ErrDesk
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   from = strings.ToLower(strings.TrimSpace(from))
	  to = strings.ToLower(strings.TrimSpace(to))
	  if from == to {
		  return ErrDeskTier
	   }
	   if !validTier(from) || !validTier(to) {
		   return ErrDeskTier
	  }
	  amt, err := money.Parse(amount)
	  if err != nil {
		   return err
	   }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	   row := d.rows[asset]
	   if row == nil {
		   return ErrDeskAsset
	   }
	   src, err := row.take(from, amt)
	  if err != nil {
		  return err
	  }
	   if from == TierCold && src.Cmp(row.floor) < 0 {
		   return ErrDeskFloor
	   }
	  row.put(from, src)
	   dst, err := row.add(to, amt)
	  if err != nil {
		   return err
	  }
	  row.put(to, dst)
	   return nil
   }

   func (r *balRow) take(tier string, amt money.Amount) (money.Amount, error) {
	  cur := r.of(tier)
	  if cur.Cmp(amt) < 0 {
		  return money.Zero(), ErrDeskShort
	   }
	   next, err := cur.Sub(amt)
	   if err != nil {
		  return money.Zero(), err
	  }
	  return next, nil
  }

  func (r *balRow) add(tier string, amt money.Amount) (money.Amount, error) {
	  return r.of(tier).Add(amt)
  }

  func (r *balRow) of(tier string) money.Amount {
	  switch tier {
	  case TierHot:
		  return r.hot
	  case TierWarm:
		  return r.warm
	  case TierCold:
		   return r.cold
	  default:
		  return money.Zero()
	  }
  }

  func (r *balRow) put(tier string, amt money.Amount) {
	   switch tier {
	   case TierHot:
		   r.hot = amt
	  case TierWarm:
		  r.warm = amt
	  case TierCold:
		   r.cold = amt
	  }
   }

  func validTier(s string) bool {
	   return s == TierHot || s == TierWarm || s == TierCold
  }

   func zero(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return "0"
	  }
	  return s
  }
