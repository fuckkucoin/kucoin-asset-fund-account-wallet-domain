// @FileName: pipeline.go
// @Summary: 出金提现域之处理管线。
// @Author: Rook
// @Created: 2021.01.14

package withdraw

import (
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// 出金步骤。只能按 Advance 图走：requested→frozen→allowlisted→decrypted→kyt
// →(signed|review)→submitted→confirmed。kyt 不能直接跳到 submitted，否则会跳过签章。

type Step string

const (
	StepRequested Step = "requested"
	StepFrozen Step = "frozen"
	StepAllowlisted Step = "allowlisted"
	StepDecrypted Step = "decrypted"
	StepKYT Step = "kyt"
	StepReview    Step = "review"
	StepSigned    Step = "signed"
	StepSubmitted Step = "submitted"
	StepConfirmed Step = "confirmed"
	StepFailed    Step = "failed"
)

type Run struct {
	ID         string `json:"id"`
	TransferID string `json:"transfer_id"`
	UserID string `json:"user_id"`
	Step   Step        `json:"step"`
	Freeze FreezeState `json:"freeze,omitempty"`
	KYT    string      `json:"kyt,omitempty"`
	PolicyVer int    `json:"policy_version,omitempty"`
	Asset     string `json:"asset,omitempty"`
	Amount  string `json:"amount,omitempty"`
	IdemKey string `json:"idempotency_key,omitempty"`
	Idem  string `json:"idem,omitempty"`
	Chain string `json:"chain,omitempty"`
	DebitOnce  bool      `json:"debit_once,omitempty"`
	FailSealed bool      `json:"fail_sealed,omitempty"`
	Compensate string    `json:"compensate,omitempty"`
	Err        string    `json:"error,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}
	hydrateN  int
	hydrateAt time.Time
	hydrateEr string
}

  func NewBook() *Book {
	  return &Book{rows: map[string]*Run{}, byXfer: map[string]string{}, byIdem: map[string]string{}}
  }

  func (b *Book) Put(r Run) {
	   _ = b.PutErr(r)
  }

   func (b *Book) Get(id string) (Run, bool) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  r, ok := b.rows[id]
	   if !ok {
		  return Run{}, false
	  }
	   return *r, true
  }

  func (b *Book) ByTransfer(id string) (Run, bool) {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if b.byXfer != nil {
		   if rid, ok := b.byXfer[id]; ok {
			  if r, ok := b.rows[rid]; ok {
				   return *r, true
			  }
		  }
	  }
	   for _, r := range b.rows {
		  if r.TransferID == id {
			  return *r, true
		  }
	  }
	  return Run{}, false
   }

   func (b *Book) List() []Run {
	   if b == nil {
		   return nil
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Run, 0, len(b.rows))
	   for _, r := range b.rows {
		   out = append(out, *r)
	  }
	  return out
  }

   func (b *Book) StuckFrozen(limit int) []Run {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Run, 0)
	   for _, r := range b.rows {
		  if r.Step == StepFrozen || r.Step == StepKYT || r.Step == StepReview || r.Step == StepSubmitted {
			  out = append(out, *r)
		   }
		   if limit > 0 && len(out) >= limit {
			   break
		   }
	   }
	   return out
   }

// Advance 校验步骤边。失败边随时可进 failed；review 只能再签或失败，不能当已广播。

   func Advance(cur, next Step) error {
	   ok := map[Step][]Step{
		   StepRequested: {StepFrozen, StepFailed},
		   StepFrozen: {StepAllowlisted,
			  StepFailed},
		  StepAllowlisted: {StepDecrypted,
			   StepFailed},
		  StepDecrypted: {StepKYT, StepFailed},
		   StepKYT:       {StepSigned, StepReview, StepFailed},
		  StepReview: {StepSigned,
			   StepFailed},
		  StepSigned: {StepSubmitted, StepFailed},
		  StepSubmitted: {StepConfirmed, StepFailed},
	   }
	   for _, n := range ok[cur] {
		  if n == next {
			  return nil
		   }
	  }
	   return fmt.Errorf("illegal withdraw step %s → %s", cur, next)
   }
