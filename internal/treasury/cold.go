package treasury

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Receive struct {
	ID     string `json:"id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	TxHash string `json:"tx_hash"`
	Chain  string `json:"chain"`
	Confirms int `json:"confirmations"`
	Need int `json:"need"`
	OK bool `json:"ok"`
	At time.Time `json:"at"`
}

type Vault struct {
	mu   sync.Mutex
	need map[string]int
	rows map[string]*Receive
	clock func() time.Time
}

   func NewVault() *Vault {
	  return &Vault{
		  need: map[string]int{"ETH": 12, "BTC": 3, "TRON": 19, "SOL": 32},
		   rows:  map[string]*Receive{},
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (v *Vault) Need(chain string, n int) {
	   v.mu.Lock()
	  defer v.mu.Unlock()
	  v.need[strings.ToUpper(strings.TrimSpace(chain))] = n
   }

// ConfirmReceive 冷库入金观察。确认数不够不得当已入库。

  func (v *Vault) ConfirmReceive(chain, hash, asset, amount string, confirms int) (Receive, error) {
	   if v == nil {
		   return Receive{},
			   ErrUnavailable
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		   return Receive{}, err
	  }
	  hash, err = validate.TxHash(hash)
	   if err != nil {
		   return Receive{}, err
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return Receive{}, err
	  }
	  if _, err := money.Parse(amount); err != nil {
		  return Receive{}, err
	  }
	   v.mu.Lock()
	  defer v.mu.Unlock()
	   need := 12
	   if n, ok := v.need[ch]; ok && n > 0 {
		   need = n
	  }
	   r := Receive{
		   ID: uuid.NewString(), Asset: as, Amount: amount, TxHash: hash, Chain: ch,
		  Confirms: confirms, Need: need, At: v.clock(),
	   }
	  if confirms >= need {
		   r.OK = true
	  }
	  cp := r
	  v.rows[ch+"|"+hash] = &cp
	  if !r.OK {
		  return r, ErrConfirm
	   }
	   return r, nil
   }

   func (v *Vault) Get(chain, hash string) (Receive, bool) {
	   v.mu.Lock()
	   defer v.mu.Unlock()
	   r, ok := v.rows[strings.ToUpper(strings.TrimSpace(chain))+"|"+strings.TrimSpace(hash)]
	  if !ok {
		  return Receive{}, false
	  }
	  return *r, true
   }

   func (v *Vault) Pending() []Receive {
	  v.mu.Lock()
	  defer v.mu.Unlock()
	   out := make([]Receive, 0)
	  for _, r := range v.rows {
		  if !r.OK {
			  out = append(out, *r)
		  }
	   }
	  return out
   }
