// @FileName: saga.go
// @Summary: 充值入账域之Saga 编排。
// @Author: EliotVoss
// @Created: 2020.10.18

package deposit

import (
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// 充值步骤。credited 只能进 unwound；不能从 credited 再 credit。
// Inspect 属于观察步，禁止包进账本事务。
type Step string

const (
	StepObserved Step = "observed"
	StepAccepted  Step = "accepted"
	StepConfirmed Step = "confirmed"
	StepCredited Step = "credited"
	StepHeld Step = "held"
	StepUnwound Step = "unwound"
	StepFailed  Step = "failed"
)

type Run struct {
	ID         string `json:"id"`
	TransferID string `json:"transfer_id"`
	Chain      string `json:"chain"`
	TxHash      string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	UserID      string `json:"user_id,omitempty"`
	Asset       string    `json:"asset,omitempty"`
	FromAddress string    `json:"from_address,omitempty"` // 观测回填的付款方
	Step        Step      `json:"step"`
	Amount      string    `json:"amount"`
	Err         string    `json:"error,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Book struct {
	mu    sync.Mutex
	byID  map[string]*Run
	byTx  map[string]string
	byOut map[string]string
	sql *persist.SQLDB
	store runStore
	prod      bool
	attachErr string
	hydrateN  int
	hydrateAt time.Time
	hydrateEr string
}

   func NewBook() *Book {
	   return &Book{byID: map[string]*Run{},
		   byTx: map[string]string{}, byOut: map[string]string{}}
   }

   func (b *Book) Put(r Run) {
	  _ = b.PutErr(r)
  }

  func (b *Book) Get(id string) (Run, bool) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   r, ok := b.byID[id]
	   if !ok {
		  return Run{}, false
	  }
	   return *r, true
  }

  func (b *Book) ByHash(chain, hash string) (Run, bool) {
	   return b.ByHashOut(chain, hash, 0)
   }

   func (b *Book) ByHashOut(chain, hash string, outputIndex int) (Run, bool) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if id, ok := b.byOut[outKey(chain, hash, outputIndex)]; ok {
		   if r, ok := b.byID[id]; ok {
			  return *r, true
		  }
	   }
	  if outputIndex == 0 {
		   if id, ok := b.byTx[chain+"|"+hash]; ok {
			   if r, ok := b.byID[id]; ok {
				  return *r, true
			  }
		  }
	  }
	   return Run{}, false
  }

   func (b *Book) Pending(limit int) []Run {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Run, 0)
	  for _, r := range b.byID {
		  if !pendingStep(r.Step) {
			   continue
		   }
		   out = append(out, *r)
		  if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	   return out
   }

   func pendingStep(s Step) bool {
	  switch s {
	  case StepAccepted,
		   StepConfirmed, StepHeld:
		   return true
	  default:
		  return false
	  }
   }

   func terminalStep(s Step) bool {
	   switch s {
	   case StepCredited, StepUnwound, StepFailed:
		   return true
	  default:
		   return false
	  }
   }

   func outKey(chain, hash string, vout int) string {
	  return chain + "|" + hash + "|" + itoaOut(vout)
  }

// Advance 校验充值边。held 可以回到 accepted（观察对上了）或 unwind，不能直接当 credited。

  func Advance(cur Step, next Step) error {
	   ok := map[Step][]Step{
		  StepObserved: {StepAccepted, StepFailed},
		  StepAccepted: {StepConfirmed,
			  StepHeld, StepFailed},
		   StepConfirmed: {StepCredited, StepHeld, StepFailed},
		  StepCredited: {StepUnwound},
		   StepHeld:     {StepAccepted, StepFailed, StepUnwound},
	   }
	   for _, n := range ok[cur] {
		   if n == next {
			  return nil
		  }
	  }
	  return fmt.Errorf("illegal deposit step %s → %s", cur, next)
   }
