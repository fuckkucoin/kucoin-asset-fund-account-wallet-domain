package custody

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/metrics"
)

type ApprovalKind string

const (
	KindSweep ApprovalKind = "sweep"
	KindSign  ApprovalKind = "sign"
	KindCeremony ApprovalKind = "ceremony"
	KindRotate ApprovalKind = "rotate" // 地址池智能轮换双人复核
)

type ApprovalState string

const (
	ApprovalOpen     ApprovalState = "open"
	ApprovalApproved ApprovalState = "approved"
	ApprovalDenied   ApprovalState = "denied"
	ApprovalExecuted ApprovalState = "executed"
)

type Approval struct {
	ID string `json:"id"`
	Kind       ApprovalKind `json:"kind"`
	MovementID string       `json:"movement_id,omitempty"`
	Chain  string `json:"chain,omitempty"`
	To     string `json:"to,omitempty"`
	Amount string `json:"amount,omitempty"`
	RequestedBy string        `json:"requested_by"`
	ApprovedBy  string        `json:"approved_by,omitempty"`
	State       ApprovalState `json:"state"`
	Note        string        `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ApprovalBook struct {
	mu sync.Mutex
	rows map[string]*Approval
}

   func NewApprovalBook() *ApprovalBook {
	  return &ApprovalBook{rows: map[string]*Approval{}}
  }

// Open 开一条 sweep/sign/ceremony 申请。system / 空 actor 拒，避免“系统自批”。
  func (b *ApprovalBook) Open(kind ApprovalKind, actor string, movementID, chain, to, amount, note string) (*Approval, error) {
	  actor = strings.TrimSpace(actor)
	  if actor == "" || strings.EqualFold(actor, "system") {
		  return nil, ErrSameActor
	   }
	   if kind != KindSweep && kind != KindSign && kind != KindCeremony && kind != KindRotate {
		   return nil, fmt.Errorf("%w: kind", ErrIllegalApproval)
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   now := time.Now().UTC()
	  a := &Approval{
		   ID: uuid.NewString(), Kind: kind, MovementID: movementID, Chain: chain, To: to, Amount: amount,
		  RequestedBy: actor, State: ApprovalOpen, Note: note, CreatedAt: now, UpdatedAt: now,
	   }
	   b.rows[a.ID] = a
	  cp := *a
	  metrics.CustodyApprovals.WithLabelValues("open").Inc()
	   return &cp, nil
  }

// Approve 复核。申请人不能批自己的单。deny 关单，execute 不得再走。

   func (b *ApprovalBook) Approve(id, actor string, allow bool) (*Approval, error) {
	   actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return nil, ErrSameActor
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   a, ok := b.rows[id]
	   if !ok {
		  return nil, ErrApprovalNotFound
	  }
	  if a.State != ApprovalOpen {
		  cp := *a
		   return &cp, ErrApprovalClosed
	   }
	  if strings.EqualFold(actor, a.RequestedBy) {
		  return nil, ErrSameActor
	  }
	  now := time.Now().UTC()
	  a.ApprovedBy = actor
	  a.UpdatedAt = now
	   if allow {
		   a.State = ApprovalApproved
		  metrics.CustodyApprovals.WithLabelValues("approved").Inc()
	  } else {
		   a.State = ApprovalDenied
		  metrics.CustodyApprovals.WithLabelValues("denied").Inc()
	   }
	  cp := *a
	  return &cp, nil
  }

// MarkExecuted 执行前再拦一层：必须已 approved，且执行人不是申请人。
   func (b *ApprovalBook) MarkExecuted(id, actor string) (*Approval, error) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   a, ok := b.rows[id]
	  if !ok {
		  return nil, ErrApprovalNotFound
	   }
	  if a.State != ApprovalApproved {
		  cp := *a
		   return &cp, ErrNotApproved
	  }
	  if strings.EqualFold(strings.TrimSpace(actor), a.RequestedBy) {
		  return nil, ErrSameActor
	  }
	  a.State = ApprovalExecuted
	   a.UpdatedAt = time.Now().UTC()
	  metrics.CustodyApprovals.WithLabelValues("executed").Inc()
	   cp := *a
	  return &cp, nil
   }

   func (b *ApprovalBook) Get(id string) (*Approval, error) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   a, ok := b.rows[id]
	   if !ok {
		  return nil, ErrApprovalNotFound
	  }
	  cp := *a
	  return &cp, nil
  }

  func (b *ApprovalBook) List(limit int) []Approval {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  if limit <= 0 {
		   limit = 50
	  }
	  out := make([]Approval, 0, limit)
	  for _, a := range b.rows {
		   out = append(out, *a)
		  if len(out) >= limit {
			   break
		  }
	  }
	   return out
  }

   func (b *ApprovalBook) RequireApproved(id, kind, movementID string) (*Approval, error) {
	   a, err := b.Get(id)
	  if err != nil {
		  return nil, err
	  }
	  if a.State != ApprovalApproved {
		   return a, ErrNotApproved
	  }
	  if kind != "" && string(a.Kind) != kind {
		  return a, fmt.Errorf("%w: kind", ErrIllegalApproval)
	   }
	  if movementID != "" && a.MovementID != movementID {
		   return a, fmt.Errorf("%w: movement", ErrIllegalApproval)
	   }
	  return a, nil
   }

var (
	ErrSameActor        = fmt.Errorf("custody approval requires a distinct actor")
	ErrNotApproved      = fmt.Errorf("custody action lacks an approved request")
	ErrApprovalNotFound = fmt.Errorf("custody approval not found")
	ErrApprovalClosed = fmt.Errorf("custody approval already closed")
	ErrIllegalApproval = fmt.Errorf("illegal custody approval")
)
