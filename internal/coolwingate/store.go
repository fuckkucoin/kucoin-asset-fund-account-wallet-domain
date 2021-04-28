package coolwingate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 持久化接口。
type Store interface {
	SaveLease(ctx context.Context, l CoolLease) (CoolLease, error)
	GetLease(ctx context.Context, id string) (CoolLease, error)
	GetLeaseByUser(ctx context.Context, userID string) (CoolLease, error)
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]CoolLease, error)
	LatestGenForUser(ctx context.Context, userID string) (int64, error)
	SaveBind(ctx context.Context, b PreflightBind) (PreflightBind, error)
	GetBind(ctx context.Context, id string) (PreflightBind, error)
	GetBindByWithdraw(ctx context.Context, withdrawID string) (PreflightBind, error)
	GetBindByToken(ctx context.Context, token string) (PreflightBind, error)
	ListOpenBindsByUser(ctx context.Context, userID string, limit int) ([]PreflightBind, error)
	ListBindsByLease(ctx context.Context, leaseID string, limit int) ([]PreflightBind, error)
	ListExpiredBinds(ctx context.Context, before time.Time, limit int) ([]PreflightBind, error)
	SaveCase(ctx context.Context, c CoolWindowCase) (CoolWindowCase, error)
	GetCase(ctx context.Context, id string) (CoolWindowCase, error)
	ListCasesByState(ctx context.Context, state CaseState, limit int) ([]CoolWindowCase, error)
	ListCasesByUser(ctx context.Context,
		userID string, limit int) ([]CoolWindowCase, error)
	ListCasesByWithdraw(ctx context.Context, withdrawID string, limit int) ([]CoolWindowCase, error)
	SaveProof(ctx context.Context, p WindowProof) (WindowProof, error)
	GetProof(ctx context.Context, id string) (WindowProof, error)
	ListProofsByCase(ctx context.Context, caseID string, limit int) ([]WindowProof, error)
	ListProofsByUser(ctx context.Context, userID string, limit int) ([]WindowProof, error)
}

// ──────────────────────────────────────────
// MemStore 内存实现（用）
// ──────────────────────────────────────────

type MemStore struct {
	mu sync.Mutex
	leases map[string]CoolLease
	binds  map[string]PreflightBind
	cases  map[string]CoolWindowCase
	proofs map[string]WindowProof
}

   func NewMemStore() *MemStore {
	   return &MemStore{
		   leases: make(map[string]CoolLease),
		  binds: make(map[string]PreflightBind),
		   cases: make(map[string]CoolWindowCase),
		  proofs: make(map[string]WindowProof),
	   }
  }

   func (m *MemStore) SaveLease(_ context.Context, l CoolLease) (CoolLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if strings.TrimSpace(l.ID) == "" {
		   return CoolLease{}, fmt.Errorf("lease id empty")
	   }
	   m.leases[l.ID] = l
	  return l, nil
   }

   func (m *MemStore) GetLease(_ context.Context, id string) (CoolLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[id]
	  if !ok {
		  return CoolLease{},
			   ErrLeaseNotFound
	   }
	   return l, nil
  }

  func (m *MemStore) GetLeaseByUser(_ context.Context, userID string) (CoolLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var best CoolLease
	  found := false
	   for _, l := range m.leases {
		   if l.UserID == userID {
			  if !found || l.CoolGen > best.CoolGen {
				   best = l
				   found = true
			  }
		   }
	  }
	   if !found {
		  return CoolLease{}, ErrLeaseNotFound
	   }
	  return best, nil
   }

   func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]CoolLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []CoolLease
	   for _, l := range m.leases {
		  if l.State == LeaseOpen && l.LeaseExpiry.Before(before) {
			  out = append(out, l)
			   if len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
   }

  func (m *MemStore) LatestGenForUser(_ context.Context, userID string) (int64, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var gen int64
	  for _, l := range m.leases {
		  if l.UserID == userID && l.CoolGen > gen {
			  gen = l.CoolGen
		  }
	  }
	  return gen, nil
   }

   func (m *MemStore) SaveBind(_ context.Context, b PreflightBind) (PreflightBind, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if strings.TrimSpace(b.ID) == "" {
		   return PreflightBind{}, fmt.Errorf("bind id empty")
	  }
	   m.binds[b.ID] = b
	  return b, nil
   }

  func (m *MemStore) GetBind(_ context.Context, id string) (PreflightBind, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  b, ok := m.binds[id]
	  if !ok {
		  return PreflightBind{}, ErrBindNotFound
	   }
	  return b, nil
   }

   func (m *MemStore) GetBindByWithdraw(_ context.Context, withdrawID string) (PreflightBind, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  for _, b := range m.binds {
		  if b.WithdrawID == withdrawID {
			  return b, nil
		  }
	   }
	  return PreflightBind{}, ErrBindNotFound
  }

   func (m *MemStore) GetBindByToken(_ context.Context, token string) (PreflightBind, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, b := range m.binds {
		  if b.Token == token {
			  return b, nil
		  }
	  }
	  return PreflightBind{}, ErrBindNotFound
   }

   func (m *MemStore) ListOpenBindsByUser(_ context.Context, userID string, limit int) ([]PreflightBind, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []PreflightBind
	  for _, b := range m.binds {
		  if b.UserID == userID && b.State == BindOpen {
			   out = append(out, b)
			  if len(out) >= limit {
				  break
			  }
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out, nil
   }

  func (m *MemStore) ListBindsByLease(_ context.Context, leaseID string, limit int) ([]PreflightBind, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []PreflightBind
	  for _, b := range m.binds {
		  if b.LeaseID == leaseID {
			  out = append(out, b)
			   if len(out) >= limit {
				   break
			  }
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) ListExpiredBinds(_ context.Context, before time.Time, limit int) ([]PreflightBind, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []PreflightBind
	  for _, b := range m.binds {
		  if b.State == BindOpen && b.BindExpiry.Before(before) {
			  out = append(out, b)
			  if len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
  }

  func (m *MemStore) SaveCase(_ context.Context, c CoolWindowCase) (CoolWindowCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if strings.TrimSpace(c.ID) == "" {
		  return CoolWindowCase{}, fmt.Errorf("case id empty")
	   }
	   m.cases[c.ID] = c
	   return c, nil
  }

   func (m *MemStore) GetCase(_ context.Context, id string) (CoolWindowCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   c, ok := m.cases[id]
	   if !ok {
		   return CoolWindowCase{}, ErrCaseNotFound
	   }
	   return c, nil
  }

  func (m *MemStore) ListCasesByState(_ context.Context, state CaseState, limit int) ([]CoolWindowCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []CoolWindowCase
	  for _, c := range m.cases {
		   if c.State == state {
			   out = append(out, c)
			  if len(out) >= limit {
				   break
			  }
		   }
	  }
	   return out, nil
   }

   func (m *MemStore) ListCasesByUser(_ context.Context, userID string, limit int) ([]CoolWindowCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []CoolWindowCase
	  for _, c := range m.cases {
		   if c.UserID == userID {
			  out = append(out, c)
			   if len(out) >= limit {
				  break
			  }
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out, nil
  }

   func (m *MemStore) ListCasesByWithdraw(_ context.Context, withdrawID string, limit int) ([]CoolWindowCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []CoolWindowCase
	  for _, c := range m.cases {
		   if c.WithdrawID == withdrawID {
			   out = append(out, c)
			   if len(out) >= limit {
				  break
			   }
		   }
	   }
	   return out, nil
   }

  func (m *MemStore) SaveProof(_ context.Context, p WindowProof) (WindowProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if strings.TrimSpace(p.ID) == "" {
		  return WindowProof{}, fmt.Errorf("proof id empty")
	   }
	   m.proofs[p.ID] = p
	   return p, nil
  }

  func (m *MemStore) GetProof(_ context.Context, id string) (WindowProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[id]
	  if !ok {
		  return WindowProof{},
			   ErrProofNotFound
	  }
	   return p, nil
  }

   func (m *MemStore) ListProofsByCase(_ context.Context, caseID string, limit int) ([]WindowProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []WindowProof
	  for _, p := range m.proofs {
		   if p.CaseID == caseID {
			   out = append(out, p)
			   if len(out) >= limit {
				   break
			   }
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) ListProofsByUser(_ context.Context, userID string, limit int) ([]WindowProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []WindowProof
	   for _, p := range m.proofs {
		  if p.UserID == userID {
			  out = append(out, p)
			  if len(out) >= limit {
				  break
			  }
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
  }
