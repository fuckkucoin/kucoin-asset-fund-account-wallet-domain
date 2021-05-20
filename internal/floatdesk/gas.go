package floatdesk

import (
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Tank 链上 gas 仓。低于 GasMin 才出补仓，金额 = target - avail。
type Tank struct {
	Chain  string `json:"chain"`
	Native string `json:"native"`
	Avail  string `json:"available"`
	Min    string `json:"min"`
	Target string `json:"target"`
}

type Refill struct {
	Chain string `json:"chain"`
	Native string `json:"native"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
}

   func (t Tank) validate() error {
	  if _, err := validate.Chain(t.Chain); err != nil {
		   return ErrChain
	   }
	   if _, err := validate.Asset(t.Native); err != nil {
		   return ErrAsset
	   }
	  if _, err := money.ParseBook(orZ(t.Avail)); err != nil {
		  return ErrGas
	  }
	  if _, err := money.Parse(t.Min); err != nil {
		  return ErrGas
	   }
	   if _, err := money.Parse(t.Target); err != nil {
		   return ErrGas
	  }
	  return nil
   }

  func PlanRefill(t Tank) (Refill, error) {
	   if err := t.validate(); err != nil {
		   return Refill{}, err
	  }
	  av, err := money.ParseBook(orZ(t.Avail))
	  if err != nil {
		  return Refill{}, err
	   }
	  min, err := money.Parse(t.Min)
	   if err != nil {
		   return Refill{}, err
	  }
	   tgt, err := money.Parse(t.Target)
	   if err != nil {
		  return Refill{}, err
	  }
	  if av.Cmp(min) >= 0 {
		   return Refill{}, ErrPlan
	  }
	  if tgt.Cmp(av) <= 0 {
		   return Refill{}, ErrPlan
	  }
	   need, err := tgt.Sub(av)
	   if err != nil || !need.IsPositive() {
		  return Refill{}, ErrPlan
	  }
	   if _, err := money.Parse(need.String()); err != nil {
		  return Refill{}, fmt.Errorf("%w: %v", ErrGas, err)
	   }
	   return Refill{
		  Chain: t.Chain, Native: t.Native, Amount: need.String(),
		  Reason: "gas below min",
	  }, nil
   }

type Tanks struct {
	mu   sync.Mutex
	rows map[string]Tank
}

   func NewTanks() *Tanks { return &Tanks{rows: map[string]Tank{}} }

   func DefaultTanks() *Tanks {
	  t := NewTanks()
	   _ = t.Put(Tank{Chain: "ETH", Native: "ETH", Avail: "1", Min: "2", Target: "8"})
	   _ = t.Put(Tank{Chain: "BTC", Native: "BTC", Avail: "0.2", Min: "0.05", Target: "0.4"})
	   _ = t.Put(Tank{Chain: "TRON", Native: "TRX", Avail: "80", Min: "200", Target: "800"})
	   _ = t.Put(Tank{Chain: "SOL", Native: "SOL", Avail: "3", Min: "5", Target: "20"})
	   return t
   }

   func (b *Tanks) Put(t Tank) error {
	   if b == nil {
		  return ErrUnavailable
	  }
	   if err := t.validate(); err != nil {
		  return err
	  }
	  t.Chain = strings.ToUpper(strings.TrimSpace(t.Chain))
	   t.Native = strings.ToUpper(strings.TrimSpace(t.Native))
	  b.mu.Lock()
	   b.rows[t.Chain] = t
	  b.mu.Unlock()
	  return nil
   }

  func (b *Tanks) Get(chain string) (Tank, error) {
	  if b == nil {
		  return Tank{}, ErrUnavailable
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  t, ok := b.rows[strings.ToUpper(strings.TrimSpace(chain))]
	  if !ok {
		   return Tank{}, ErrNotFound
	  }
	   return t, nil
  }

   func (b *Tanks) List() []Tank {
	  if b == nil {
		   return nil
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Tank, 0, len(b.rows))
	  for _, t := range b.rows {
		  out = append(out, t)
	   }
	   return out
   }

  func (b *Tanks) ScanRefill() []Refill {
	  out := make([]Refill, 0)
	   for _, t := range b.List() {
		  r, err := PlanRefill(t)
		   if err != nil {
			  continue
		  }
		  out = append(out, r)
	  }
	  return out
   }
