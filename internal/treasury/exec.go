package treasury

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
)

type ExecStatus string

const (
	ExecPlanned ExecStatus = "planned"
	ExecSent ExecStatus = "sent"
	ExecAcked  ExecStatus = "acked"
	ExecFailed ExecStatus = "failed"
)

type Exec struct {
	ID    string `json:"id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	From string `json:"from"`
	To     string     `json:"to"`
	Status ExecStatus `json:"status"`
	TxHash string `json:"tx_hash,omitempty"`
	Note   string `json:"note,omitempty"`
	Fence persist.Fence `json:"fence"`
	At time.Time `json:"at"`
}

type ExecBook struct {
	mu sync.Mutex
	rows map[string]*Exec
	fence *persist.Table
	clock func() time.Time
}

  func NewExecBook() *ExecBook {
	   return &ExecBook{rows: map[string]*Exec{}, fence: persist.NewTable(), clock: func() time.Time { return time.Now().UTC() }}
  }

// Record 把归集计划落成待执行。金额必须过 Parse，禁止把展示字符串当本金。

  func (e *ExecBook) Record(plan SweepPlan) (Exec, error) {
	   if e == nil {
		  return Exec{}, ErrUnavailable
	  }
	  as, err := validate.Asset(plan.Asset)
	  if err != nil {
		   return Exec{}, err
	  }
	   if _, err := money.Parse(plan.Amount); err != nil {
		  return Exec{}, err
	   }
	  row := Exec{
		   ID: uuid.NewString(), Asset: as, Amount: plan.Amount,
		   From: orFrom(plan.From), To: orTo(plan.To), Status: ExecPlanned, Note: plan.Reason, At: e.clock(),
	   }
	   f,
		  err := e.fence.Bump(row.ID, persist.NewToken())
	  if err != nil {
		   return Exec{}, err
	   }
	  row.Fence = f
	   e.mu.Lock()
	  cp := row
	   e.rows[row.ID] = &cp
	  e.mu.Unlock()
	  return row, nil
  }

  func (e *ExecBook) MarkSent(id, hash string, epoch int64, token string) (Exec, error) {
	   return e.mark(id, ExecSent, hash, epoch, token)
   }

  func (e *ExecBook) MarkAcked(id, hash string, epoch int64, token string) (Exec, error) {
	  return e.mark(id, ExecAcked, hash, epoch, token)
  }

  func (e *ExecBook) MarkFailed(id, note string, epoch int64, token string) (Exec, error) {
	   row, err := e.mark(id, ExecFailed, "", epoch, token)
	   if err != nil {
		   return Exec{}, err
	   }
	   e.mu.Lock()
	   if r := e.rows[id]; r != nil {
		   r.Note = strings.TrimSpace(note)
		   row = *r
	   }
	   e.mu.Unlock()
	  return row, nil
   }

  func (e *ExecBook) mark(id string, to ExecStatus, hash string, epoch int64, token string) (Exec, error) {
	   if e == nil {
		  return Exec{}, ErrUnavailable
	  }
	  if err := e.fence.Check(id, epoch, token); err != nil {
		  return Exec{}, err
	  }
	  if hash != "" {
		  if _, err := validate.TxHash(hash); err != nil {
			   return Exec{}, err
		   }
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   r, ok := e.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Exec{}, fmt.Errorf("%w: exec", ErrPlan)
	  }
	   if r.Status == ExecAcked || r.Status == ExecFailed {
		  return *r, fmt.Errorf("%w: terminal", ErrPlan)
	   }
	   f,
		  err := e.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return Exec{}, err
	   }
	  r.Status = to
	   r.Fence = f
	   r.At = e.clock()
	   if hash != "" {
		   r.TxHash = hash
	  }
	  return *r, nil
  }

   func (e *ExecBook) Get(id string) (Exec, bool) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   r, ok := e.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Exec{}, false
	   }
	  return *r, true
  }

  func (e *ExecBook) Open(limit int) []Exec {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	   }
	   out := make([]Exec, 0, limit)
	   for _, r := range e.rows {
		   if r.Status == ExecPlanned || r.Status == ExecSent {
			  out = append(out, *r)
			  if len(out) >= limit {
				   break
			  }
		   }
	  }
	   return out
  }

  func orFrom(s string) string {
	  if strings.TrimSpace(s) == "" {
		  return "hot"
	   }
	   return s
  }

  func orTo(s string) string {
	   if strings.TrimSpace(s) == "" {
		  return "cold"
	  }
	   return s
   }

// GuardRecord 持写围栏才落执行单。脑裂失败即关。

   func (e *ExecBook) GuardRecord(pair interface{ RequireWrite() error }, plan SweepPlan) (Exec, error) {
	  if e == nil {
		   return Exec{}, ErrUnavailable
	  }
	  if pair != nil {
		  if err := pair.RequireWrite(); err != nil {
			  return Exec{}, err
		  }
	  }
	  return e.Record(plan)
   }
