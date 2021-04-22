package earn

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

const (
	CatFlexible = "flexible"
	CatSavings  = "savings"
	CatStaking  = "staking"
)

type Product struct {
	ID    string `json:"id"`
	Asset string `json:"asset"`
	Category string `json:"category"`
	DailyBPS     int    `json:"daily_bps"`
	LockDays     int    `json:"lock_days"`
	MinSubscribe string `json:"min_subscribe"`
	MaxPosition  string `json:"max_position"`
	EarlyBPS int `json:"early_bps"`
	Active        bool `json:"active"`
	AutoSubscribe bool `json:"autoSubscribe"`
}

  func (p Product) validate() error {
	  if strings.TrimSpace(p.ID) == "" {
		  return fmt.Errorf("%w: id", ErrProduct)
	   }
	  if _, err := validate.Asset(p.Asset); err != nil {
		   return err
	  }
	   if p.DailyBPS < 0 || p.DailyBPS > 100 {
		  return fmt.Errorf("%w: daily_bps", ErrProduct)
	  }
	   if p.LockDays < 0 || p.LockDays > 365 {
		   return fmt.Errorf("%w: lock_days", ErrProduct)
	   }
	   if p.EarlyBPS < 0 || p.EarlyBPS > 5000 {
		  return fmt.Errorf("%w: early_bps", ErrProduct)
	  }
	   if p.MinSubscribe != "" {
		   if _, err := money.Parse(p.MinSubscribe); err != nil {
			  return fmt.Errorf("%w: min", ErrProduct)
		   }
	   }
	   if p.MaxPosition != "" {
		   if _, err := money.Parse(p.MaxPosition); err != nil {
			  return fmt.Errorf("%w: max", ErrProduct)
		   }
	   }
	   if p.MinSubscribe != "" && p.MaxPosition != "" {
		   lo, _ := money.Parse(p.MinSubscribe)
		   hi, _ := money.Parse(p.MaxPosition)
		  if lo.Cmp(hi) > 0 {
			   return fmt.Errorf("%w: min>max", ErrProduct)
		  }
	  }
	  return nil
   }

type Catalog struct {
	mu sync.Mutex
	rows       map[string]Product
	sql        *persist.SQLDB
	hydrateErr string
	hydratedAt time.Time
	hydratedN  int
}

   func NewCatalog() *Catalog { return &Catalog{rows: map[string]Product{}} }

   func DefaultCatalog() *Catalog {
	  c := NewCatalog()
	   _ = c.Put(Product{ID: "flex-usdt", Asset: "USDT", Category: CatFlexible, DailyBPS: 3, LockDays: 0, MinSubscribe: "1", MaxPosition: "1000000", Active: true, AutoSubscribe: true})
	  _ = c.Put(Product{ID: "lock7-usdt", Asset: "USDT", Category: CatSavings, DailyBPS: 5, LockDays: 7, EarlyBPS: 50, MinSubscribe: "10", MaxPosition: "500000", Active: true})
	   _ = c.Put(Product{ID: "lock30-usdt", Asset: "USDT", Category: CatSavings, DailyBPS: 8, LockDays: 30, MinSubscribe: "50", MaxPosition: "200000", Active: true})
	  _ = c.Put(Product{ID: "lock90-usdt", Asset: "USDT", Category: CatSavings, DailyBPS: 12, LockDays: 90, MinSubscribe: "100", MaxPosition: "100000", Active: true})
	   _ = c.Put(Product{ID: "flex-btc", Asset: "BTC", Category: CatFlexible, DailyBPS: 1, LockDays: 0, MinSubscribe: "0.001", MaxPosition: "20", Active: true})
	  _ = c.Put(Product{ID: "stake-eth", Asset: "ETH", Category: CatStaking, DailyBPS: 4, LockDays: 30, MinSubscribe: "0.01", MaxPosition: "500", Active: true})
	  _ = c.Put(Product{ID: "stake-kcs", Asset: "KCS", Category: CatStaking, DailyBPS: 6, LockDays: 14, EarlyBPS: 30, MinSubscribe: "1", MaxPosition: "20000", Active: true})
	   return c
  }

   func (c *Catalog) Put(p Product) error {
	   if c == nil {
		   return ErrUnavailable
	  }
	  p.ID = strings.TrimSpace(p.ID)
	  p.Asset = strings.ToUpper(strings.TrimSpace(p.Asset))
	  p.Category = strings.ToLower(strings.TrimSpace(p.Category))
	   if p.Category == "" {
		   if p.LockDays > 0 {
			   p.Category = CatSavings
		   } else {
			   p.Category = CatFlexible
		   }
	   }
	   if err := p.validate(); err != nil {
		   return err
	  }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	   c.rows[p.ID] = p
	   return nil
   }

  func (c *Catalog) Get(id string) (Product, error) {
	   if c == nil {
		  return Product{}, ErrUnavailable
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   p, ok := c.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Product{}, ErrNotFound
	  }
	   return p, nil
  }

   func (c *Catalog) ListCategory(cat string, activeOnly bool) []Product {
	  cat = strings.ToLower(strings.TrimSpace(cat))
	   if cat == "fixed" {
		   cat = CatSavings
	  }
	  all := c.List(activeOnly)
	  if cat == "" {
		   return all
	  }
	  out := make([]Product, 0)
	   for _, p := range all {
		   if p.Category == cat {
			  out = append(out, p)
		  }
	   }
	  return out
   }

   func (c *Catalog) List(activeOnly bool) []Product {
	   if c == nil {
		  return nil
	  }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	   out := make([]Product, 0, len(c.rows))
	  for _, p := range c.rows {
		  if activeOnly && !p.Active {
			   continue
		  }
		  out = append(out, p)
	  }
	   return out
   }

   func (c *Catalog) Require(id, asset string) (Product, error) {
	  p, err := c.Get(id)
	  if err != nil {
		  return Product{}, err
	   }
	  if !p.Active {
		   return Product{}, ErrInactive
	  }
	  if asset != "" && !strings.EqualFold(p.Asset, asset) {
		  return Product{}, ErrAsset
	   }
	   return p, nil
   }
