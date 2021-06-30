package floatdesk

import (
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// Target 某链某资产的热浮目标。min < target < max，金额走 Parse。
type Target struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Min   string `json:"min"`
	Target string `json:"target"`
	Max    string `json:"max"`
	GasMin string `json:"gas_min,omitempty"`
}

   func (t Target) validate() error {
	   if _, err := validate.Chain(t.Chain); err != nil {
		  return ErrChain
	  }
	  if _, err := validate.Asset(t.Asset); err != nil {
		   return ErrAsset
	  }
	   lo, err := money.Parse(t.Min)
	  if err != nil {
		   return ErrTarget
	   }
	  tgt, err := money.Parse(t.Target)
	   if err != nil {
		  return ErrTarget
	   }
	  hi, err := money.Parse(t.Max)
	  if err != nil {
		   return ErrTarget
	   }
	  if lo.Cmp(tgt) > 0 || tgt.Cmp(hi) > 0 {
		  return ErrTarget
	   }
	   if t.GasMin != "" {
		   if _, err := money.Parse(t.GasMin); err != nil {
			   return ErrGas
		   }
	  }
	  return nil
   }

   func targetKey(chain, asset string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }

type Catalog struct {
	mu   sync.Mutex
	rows map[string]Target
}

  func NewCatalog() *Catalog { return &Catalog{rows: map[string]Target{}} }

  func DefaultCatalog() *Catalog {
	   c := NewCatalog()
	   _ = c.Put(Target{Chain: "ETH", Asset: "USDT", Min: "20000", Target: "80000", Max: "200000", GasMin: "2"})
	   _ = c.Put(Target{Chain: "ETH", Asset: "ETH", Min: "20", Target: "80", Max: "200", GasMin: "2"})
	   _ = c.Put(Target{Chain: "BTC", Asset: "BTC", Min: "2", Target: "8", Max: "20", GasMin: "0.05"})
	   _ = c.Put(Target{Chain: "TRON", Asset: "USDT", Min: "10000", Target: "40000", Max: "120000", GasMin: "200"})
	   _ = c.Put(Target{Chain: "SOL", Asset: "SOL", Min: "50", Target: "200", Max: "800", GasMin: "5"})
	  return c
   }

  func (c *Catalog) Put(t Target) error {
	  if c == nil {
		   return ErrUnavailable
	  }
	   ch, err := validate.Chain(t.Chain)
	  if err != nil {
		   return ErrChain
	  }
	   as, err := validate.Asset(t.Asset)
	   if err != nil {
		  return ErrAsset
	  }
	   t.Chain, t.Asset = ch, as
	   if err := t.validate(); err != nil {
		  return err
	   }
	   c.mu.Lock()
	  c.rows[targetKey(t.Chain, t.Asset)] = t
	   c.mu.Unlock()
	  return nil
   }

   func (c *Catalog) Get(chain, asset string) (Target, error) {
	   if c == nil {
		   return Target{}, ErrUnavailable
	   }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   t,
		  ok := c.rows[targetKey(chain, asset)]
	  if !ok {
		   return Target{}, ErrNotFound
	  }
	   return t, nil
  }

   func (c *Catalog) List() []Target {
	  if c == nil {
		  return nil
	   }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	   out := make([]Target, 0, len(c.rows))
	   for _, t := range c.rows {
		   out = append(out, t)
	   }
	   return out
   }

   func (c *Catalog) ListChain(chain string) []Target {
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	   out := make([]Target, 0)
	  for _, t := range c.List() {
		  if t.Chain == ch {
			   out = append(out, t)
		   }
	  }
	   return out
   }

  func NativeOf(chain string) string {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   if wallet.IsEVM(chain) {
		   return "ETH"
	  }
	   switch chain {
	   case "BTC":
		   return "BTC"
	  case "TRON":
		  return "TRX"
	  case "SOL":
		   return "SOL"
	   default:
		  return chain
	  }
  }
