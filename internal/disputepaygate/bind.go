package disputepaygate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"github.com/google/uuid"
)

// LedgerPost 账本落账意图（不实现余额公式）。
type LedgerPost struct {
	LedgerID string
	CaseID   string
	UserID   string
	Asset     string
	Amount    string
	Direction string // credit | debit
	LeaseID   string
	IdemKey string
}

// PostedEntry 绑定后的落账回执。
type PostedEntry struct {
	LedgerID  string `json:"ledger_id"`
	LeaseID   string `json:"dispute_pay_lease_id"`
	Direction string `json:"direction"`
	Allowed   bool   `json:"allowed"`
	ProofID   string `json:"proof_id,omitempty"`
	Consumed bool `json:"consumed"`
}

// LedgerPoster 外部账本提交面；本包不改写贷记公式。
type LedgerPoster interface {
	Post(ctx context.Context, p LedgerPost) (ledgerID string, err error)
}

// MemoryPoster 进程内落账登记（业务 map 持久于引擎生命周期，非伪造余额公式）。
type MemoryPoster struct {
	mu sync.Mutex
	rows map[string]LedgerPost
}

  func NewMemoryPoster() *MemoryPoster {
	  return &MemoryPoster{rows: map[string]LedgerPost{}}
   }

  func (m *MemoryPoster) Post(_ context.Context, p LedgerPost) (string, error) {
	  if m == nil {
		   return "", ErrUnavailable
	   }
	   if strings.TrimSpace(p.IdemKey) == "" {
		  return "", ErrInvalidInput
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if exist, ok := m.rows[p.IdemKey]; ok {
		  return exist.LedgerID, nil
	  }
	  if p.LedgerID == "" {
		   p.LedgerID = uuid.NewString()
	   }
	   m.rows[p.IdemKey] = p
	  return p.LedgerID, nil
  }

   func (m *MemoryPoster) Get(idem string) (LedgerPost, bool) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.rows[idem]
	   return p, ok
   }

// Binder 租约复验后提交账本。
type Binder struct {
	eng    *Engine
	poster LedgerPoster
}

  func NewBinder(eng *Engine, poster LedgerPoster) *Binder {
	   if poster == nil {
		  poster = NewMemoryPoster()
	   }
	  return &Binder{eng: eng, poster: poster}
   }

// CreditWithLease 贷记：必须持有效 pay_lease。
  func (b *Binder) CreditWithLease(ctx context.Context, p LedgerPost) (*PostedEntry, error) {
	   if b == nil || b.eng == nil {
		  return nil, ErrUnavailable
	   }
	   p.Direction = "credit"
	  ar, err := b.eng.AssertPayLease(ctx, AssertPayInput{
		  LeaseID: p.LeaseID, CaseID: p.CaseID, Amount: p.Amount, Asset: p.Asset,
		   LedgerID: p.LedgerID, WriteKind: "credit",
	  })
	  if err != nil {
		   return &PostedEntry{Allowed: false, ProofID: proofOf(ar), LeaseID: p.LeaseID}, err
	  }
	  lid, err := b.poster.Post(ctx, p)
	   if err != nil {
		  return nil, err
	  }
	   consumed, cerr := b.eng.ConsumePayLease(ctx, p.LeaseID, lid)
	  if cerr != nil {
		  return &PostedEntry{LedgerID: lid, LeaseID: p.LeaseID, Direction: "credit", Allowed: true, Consumed: false}, cerr
	   }
	   return &PostedEntry{LedgerID: lid, LeaseID: consumed.LeaseID, Direction: "credit", Allowed: true, Consumed: true}, nil
   }

// DebitWithClaw 借记/扣回：必须持有效 claw_lease。

   func (b *Binder) DebitWithClaw(ctx context.Context, p LedgerPost) (*PostedEntry, error) {
	   if b == nil || b.eng == nil {
		   return nil, ErrUnavailable
	  }
	   p.Direction = "debit"
	  ar, err := b.eng.AssertClawLease(ctx, AssertPayInput{
		  LeaseID: p.LeaseID, CaseID: p.CaseID, Amount: p.Amount, Asset: p.Asset,
		  LedgerID: p.LedgerID, WriteKind: "debit",
	  })
	  if err != nil {
		   return &PostedEntry{Allowed: false, ProofID: proofOf(ar), LeaseID: p.LeaseID}, err
	  }
	  if p.LeaseID == "" && ar.Claw != nil {
		  p.LeaseID = ar.Claw.LeaseID
	   }
	  lid, err := b.poster.Post(ctx, p)
	  if err != nil {
		   return nil, err
	  }
	   consumed, cerr := b.eng.ConsumeClawLease(ctx, p.LeaseID, lid)
	   if cerr != nil {
		   return &PostedEntry{LedgerID: lid, LeaseID: p.LeaseID, Direction: "debit", Allowed: true, Consumed: false}, cerr
	  }
	  return &PostedEntry{LedgerID: lid, LeaseID: consumed.LeaseID, Direction: "debit", Allowed: true, Consumed: true}, nil
   }

   func proofOf(ar *AssertResult) string {
	  if ar == nil {
		  return ""
	  }
	   return ar.ProofID
   }

// BindGuard 供他包在 ledger.Create 前只读断言。

  func (e *Engine) BindGuard(ctx context.Context, leaseID, caseID, amount, asset, direction string) error {
	  in := AssertPayInput{LeaseID: leaseID,
		  CaseID: caseID, Amount: amount, Asset: asset}
	  if strings.EqualFold(direction, "debit") || strings.EqualFold(direction, "claw") {
		  _, err := e.AssertClawLease(ctx, in)
		  return err
	   }
	  _,
		  err := e.AssertPayLease(ctx, in)
	   return err
  }

// MustHavePayLease 空租约直接失败，用于包装既有贷记入口。
  func MustHavePayLease(leaseID string) error {
	   if strings.TrimSpace(leaseID) == "" {
		  return fmt.Errorf("%w", ErrNoLeaseForCredit)
	   }
	  return nil
   }
