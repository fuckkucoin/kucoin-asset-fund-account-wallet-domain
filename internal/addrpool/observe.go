package addrpool

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Sighting struct {
	Chain string `json:"chain"`
	Address string `json:"address"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	TxHash string `json:"tx_hash"`
	Confirms int `json:"confirmations"`
	UserID string    `json:"user_id,omitempty"`
	Ready  bool      `json:"ready"`
	SeenAt time.Time `json:"seen_at"`
}

type Observer struct {
	mu   sync.Mutex
	need map[string]int
	seen map[string]*Sighting
	clock func() time.Time
	minConf int
}

  func NewObserver() *Observer {
	  return &Observer{
		  need: map[string]int{"BTC": 3, "ETH": 12, "TRON": 20, "SOL": 32},
		  seen:    map[string]*Sighting{},
		  clock:   func() time.Time { return time.Now().UTC() },
		  minConf: 1,
	   }
   }

   func (o *Observer) SetNeed(chain string, n int) error {
	  if o == nil {
		   return fmt.Errorf("address observer unavailable")
	   }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		   return err
	  }
	   if n < 0 {
		  return fmt.Errorf("confirmations")
	  }
	  o.mu.Lock()
	   o.need[ch] = n
	  o.mu.Unlock()
	   return nil
   }

   func sightKey(chain, hash string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(hash)
   }

// Observe 记一笔链上入金观察。确认数不够不得 ready，调用方不得 credit。
  func (o *Observer) Observe(chain, addr, asset, amount, hash string, confirms int) (Sighting, error) {
	   if o == nil {
		   return Sighting{}, fmt.Errorf("address observer unavailable")
	  }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		   return Sighting{}, err
	   }
	  a, err := validate.Address(ch, addr)
	  if err != nil {
		  return Sighting{}, err
	  }
	  as, err := validate.Asset(asset)
	   if err != nil {
		  return Sighting{}, err
	   }
	   if _, err := money.Parse(amount); err != nil {
		   return Sighting{}, err
	  }
	   h, err := validate.TxHash(hash)
	  if err != nil {
		   return Sighting{}, err
	  }
	   if confirms < 0 {
		   return Sighting{}, fmt.Errorf("confirmations")
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   need := o.need[ch]
	   if need <= 0 {
		  need = o.minConf
	   }
	  row := &Sighting{
		   Chain: ch, Address: a, Asset: as, Amount: amount, TxHash: h,
		   Confirms: confirms,
		  Ready: confirms >= need, SeenAt: o.clock(),
	  }
	   o.seen[sightKey(ch, h)] = row
	   return *row, nil
  }

   func (o *Observer) BindUser(chain, hash, userID string) (Sighting, error) {
	  if o == nil {
		   return Sighting{}, fmt.Errorf("address observer unavailable")
	   }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return Sighting{}, err
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   row, ok := o.seen[sightKey(chain, hash)]
	   if !ok {
		   return Sighting{}, fmt.Errorf("sighting not found")
	   }
	   if !row.Ready {
		   return *row, fmt.Errorf("confirmations not met")
	  }
	  row.UserID = uid
	   return *row, nil
  }

   func (o *Observer) Get(chain, hash string) (Sighting, bool) {
	   if o == nil {
		   return Sighting{}, false
	  }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	  row, ok := o.seen[sightKey(chain, hash)]
	  if !ok {
		   return Sighting{}, false
	   }
	  return *row, true
   }

  func (o *Observer) List(chain string, limit int) []Sighting {
	  if o == nil {
		   return nil
	  }
	   if limit <= 0 {
		   limit = 50
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   out := make([]Sighting, 0, limit)
	   for _, row := range o.seen {
		  if chain != "" && row.Chain != chain {
			   continue
		  }
		   out = append(out, *row)
		  if len(out) >= limit {
			   break
		  }
	  }
	   return out
   }

   func DeriveFor(userID, chain string) (string, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return "", err
	  }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		  return "", err
	  }
	  addr := wallet.DeriveAddress(uid,
		   ch)
	  if addr == "" || wallet.ValidChainAddress(ch, addr) != nil {
		   return "", fmt.Errorf("derive failed for %s", ch)
	  }
	  return addr, nil
  }
