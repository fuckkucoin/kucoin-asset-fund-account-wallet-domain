package freeze

import (
	"fmt"
	"strings"
	"sync"
	"github.com/shopspring/decimal"
)

const ReasonPartialLiqRelease = "partial_liq_release"

var (
	ErrAmount       = fmt.Errorf("freeze amount required")
	ErrOverRelease  = fmt.Errorf("freeze over-release refused")
	ErrIdemMismatch = fmt.Errorf("freeze release idem mismatch")
)

// AmountAPI 金额占用口。Hold 记锁仓额，Release 按成交释出。

type AmountAPI interface {
	Hold(accountID, asset, amount, reason string) error
	Release(accountID, asset, amount, idemKey string) error
	FrozenAmount(accountID, asset string) string
	ReleaseCalls(accountID string) int
	ReleasedAmount(accountID, asset string) string
}

// AmountBook 内存占用账。做什么：按账户+资产累计冻结与已释。
// 金额须非负；幂等键已见则金额必须相同。
// 不变量：frozen + released == held；frozen 不得为负。
type AmountBook struct {
	mu sync.Mutex
	held     map[string]decimal.Decimal
	frozen   map[string]decimal.Decimal
	released map[string]decimal.Decimal
	idem     map[string]string
	calls    map[string]int
}

   func NewAmountBook() *AmountBook {
	   return &AmountBook{
		   held: map[string]decimal.Decimal{},
		   frozen:   map[string]decimal.Decimal{},
		  released: map[string]decimal.Decimal{},
		  idem: map[string]string{},
		  calls: map[string]int{},
	   }
   }

  func amtKey(accountID, asset string) string {
	  return strings.TrimSpace(accountID) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

  func parseAmt(raw string) (decimal.Decimal, error) {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return decimal.Zero, ErrAmount
	  }
	  d, err := decimal.NewFromString(raw)
	  if err != nil || d.IsNegative() {
		   return decimal.Zero, ErrAmount
	  }
	  return d, nil
  }

   func (b *AmountBook) Hold(accountID, asset, amount, reason string) error {
	  if b == nil {
		  return ErrAccount
	   }
	  accountID = strings.TrimSpace(accountID)
	   if accountID == "" {
		  return ErrAccount
	  }
	   if strings.TrimSpace(reason) == "" {
		   return ErrReason
	  }
	  amt, err := parseAmt(amount)
	   if err != nil || !amt.IsPositive() {
		  return ErrAmount
	   }
	   k := amtKey(accountID, asset)
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   b.held[k] = b.held[k].Add(amt)
	   b.frozen[k] = b.frozen[k].Add(amt)
	  return nil
   }

  func (b *AmountBook) Release(accountID, asset, amount, idemKey string) error {
	   if b == nil {
		  return ErrAccount
	  }
	  accountID = strings.TrimSpace(accountID)
	  idemKey = strings.TrimSpace(idemKey)
	   if accountID == "" || idemKey == "" {
		  return ErrAccount
	   }
	   amt, err := parseAmt(amount)
	  if err != nil {
		   return err
	   }
	  k := amtKey(accountID, asset)
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if prev, ok := b.idem[idemKey]; ok {
		   if prev != amt.String() {
			   return ErrIdemMismatch
		  }
		   return nil
	   }
	  if amt.IsZero() {
		   b.idem[idemKey] = amt.String()
		  return nil
	   }
	  cur := b.frozen[k]
	   if amt.GreaterThan(cur) {
		  return ErrOverRelease
	   }
	   b.frozen[k] = cur.Sub(amt)
	   b.released[k] = b.released[k].Add(amt)
	  b.idem[idemKey] = amt.String()
	  b.calls[accountID]++
	   return nil
  }

   func (b *AmountBook) FrozenAmount(accountID, asset string) string {
	   if b == nil {
		  return "0"
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   return b.frozen[amtKey(accountID, asset)].String()
   }

   func (b *AmountBook) ReleasedAmount(accountID, asset string) string {
	   if b == nil {
		  return "0"
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   return b.released[amtKey(accountID, asset)].String()
  }

   func (b *AmountBook) ReleaseCalls(accountID string) int {
	   if b == nil {
		  return 0
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   return b.calls[strings.TrimSpace(accountID)]
  }

// Snapshot 导出占用快照，供进程重入恢复。

   func (b *AmountBook) Snapshot() map[string]string {
	   if b == nil {
		   return nil
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := map[string]string{}
	   for k, v := range b.frozen {
		  out["frozen:"+k] = v.String()
	   }
	  for k, v := range b.released {
		   out["released:"+k] = v.String()
	  }
	  for k, v := range b.held {
		  out["held:"+k] = v.String()
	   }
	  for k, v := range b.idem {
		   out["idem:"+k] = v
	   }
	  return out
   }

// Restore 从快照恢复占用账。

  func (b *AmountBook) Restore(snap map[string]string) {
	   if b == nil || snap == nil {
		   return
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   for k, v := range snap {
		  d, err := decimal.NewFromString(v)
		   if err != nil {
			  continue
		   }
		   switch {
		  case strings.HasPrefix(k, "frozen:"):
			   b.frozen[strings.TrimPrefix(k, "frozen:")] = d
		   case strings.HasPrefix(k, "released:"):
			  b.released[strings.TrimPrefix(k, "released:")] = d
		   case strings.HasPrefix(k, "held:"):
			  b.held[strings.TrimPrefix(k, "held:")] = d
		   case strings.HasPrefix(k, "idem:"):
			  b.idem[strings.TrimPrefix(k, "idem:")] = v
		  }
	  }
   }
