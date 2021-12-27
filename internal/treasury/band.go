package treasury

import (
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Band struct {
	Asset string `json:"asset"`
	Low    string `json:"low"`
	Target string `json:"target"`
	High   string `json:"high"`
}

type Level string

const (
	LevelLow Level = "below_low"
	LevelIn Level = "in_band"
	LevelHigh  Level = "above_high"
	LevelEmpty Level = "empty"
)

  func (b Band) validate() error {
	  if _, err := validate.Asset(b.Asset); err != nil {
		  return err
	   }
	  low, err := money.Parse(b.Low)
	  if err != nil {
		   return err
	  }
	  tgt, err := money.Parse(b.Target)
	   if err != nil {
		   return err
	  }
	   hi, err := money.Parse(b.High)
	   if err != nil {
		   return err
	  }
	   if low.Cmp(tgt) > 0 || tgt.Cmp(hi) > 0 {
		   return ErrBand
	   }
	  return nil
	   }
	  low, err := money.Parse(b.Low)
	  if err != nil {
		  return "", err
	   }
	  hi, err := money.Parse(b.High)
	   if err != nil {
		   return "", err
	   }
	  if h.Cmp(low) < 0 {
		  return LevelLow, nil
	  }
	  if h.Cmp(hi) > 0 {
		   return LevelHigh, nil
	   }
	  return LevelIn, nil
   }

type Catalog struct {
	mu   sync.Mutex
	rows map[string]Band
}

  func NewCatalog() *Catalog { return &Catalog{rows: map[string]Band{}} }

  func DefaultCatalog() *Catalog {
	   c := NewCatalog()
	  _ = c.Put(Band{Asset: "USDT", Low: "50000", Target: "200000", High: "500000"})
	  _ = c.Put(Band{Asset: "BTC", Low: "5", Target: "20", High: "80"})
	  _ = c.Put(Band{Asset: "ETH", Low: "50", Target: "200", High: "800"})
	  return c
  }

   func (c *Catalog) Put(b Band) error {
	  if c == nil {
		   return ErrUnavailable
	  }
	   b.Asset = strings.ToUpper(strings.TrimSpace(b.Asset))
	   if err := b.validate(); err != nil {
		  return err
	   }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	  c.rows[b.Asset] = b
	   return nil
   }

  func (c *Catalog) Get(asset string) (Band, error) {
	  if c == nil {
		   return Band{}, ErrUnavailable
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	  b,
		   ok := c.rows[strings.ToUpper(strings.TrimSpace(asset))]
	   if !ok {
		  return Band{}, ErrAsset
	   }
	   return b, nil
   }

  func (c *Catalog) List() []Band {
	  if c == nil {
		  return nil
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   out := make([]Band,
		  0, len(c.rows))
	   for _, b := range c.rows {
		   out = append(out, b)
	  }
	   return out
   }

   func orZ(s string) string {
	   if strings.TrimSpace(s) == "" {
		  return "0"
	  }
	   return s
  }
