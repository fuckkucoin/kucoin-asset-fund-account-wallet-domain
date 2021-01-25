package acctlifegate

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemStore Store 的内存实现，供单元使用。
// 所有操作在 mutex 保护下完成，数据库事务的串行语义。

type MemStore struct {
	mu sync.Mutex
	// per-(userID|book) → 世代列表（末尾为最新）
	gens map[string][]BookLifeGeneration
	// leaseID → LifeLease
	leases map[string]*LifeLease
	// bindID → MoveBind
	binds map[string]*MoveBind
	// idemKey → bindID（辅助索引）
	bindsByIdem map[string]string
	// caseID → LifeDriftCase
	drifts map[string]*LifeDriftCase
}

// NewMemStore 构造可直接用于的内存 Store。
   func NewMemStore() *MemStore {
	   return &MemStore{
		   gens:        make(map[string][]BookLifeGeneration),
		   leases:      make(map[string]*LifeLease),
		   binds:       make(map[string]*MoveBind),
		   bindsByIdem: make(map[string]string),
		  drifts: make(map[string]*LifeDriftCase),
	   }
   }

  func genKey(userID, book string) string { return userID + "|" + book }

// ── BookLifeGeneration ───────────────────────────────────────────────────────

   func (m *MemStore) CurrentGeneration(_ context.Context, userID, book string) (BookLifeGeneration, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  rows := m.gens[genKey(userID, book)]
	   if len(rows) == 0 {
		   return BookLifeGeneration{UserID: userID, Book: book, Generation: 0}, nil
	   }
	  return rows[len(rows)-1], nil
   }

  func (m *MemStore) BumpGeneration(_ context.Context, userID, book, action, actor string) (BookLifeGeneration, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  k := genKey(userID, book)
	   rows := m.gens[k]
	   var cur int64
	  if len(rows) > 0 {
		   cur = rows[len(rows)-1].Generation
	  }
	  next := BookLifeGeneration{
		   GenID:  fmt.Sprintf("gen-%s-%d", k, cur+1),
		  UserID: userID,
		   Book:   book,
		  Generation: cur + 1,
		  Action:     action,
		   Actor:      actor,
		   CreatedAt:  time.Now().UTC(),
	   }
	  m.gens[k] = append(m.gens[k], next)
	  return next, nil
  }

// ── LifeLease ────────────────────────────────────────────────────────────────

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*LifeLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	   cp := *l
	  return &cp, nil
   }

   func (m *MemStore) InsertLease(_ context.Context, lease LifeLease) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.leases[lease.LeaseID]; ok {
		  return fmt.Errorf("acctlifegate: lease %s already exists", lease.LeaseID)
	  }
	  cp := lease
	   m.leases[lease.LeaseID] = &cp
	  return nil
   }

  func (m *MemStore) UpdateLeaseState(_ context.Context, leaseID string, expected, next LeaseState, now time.Time, note string) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[leaseID]
	   if !ok {
		  return ErrLeaseNotFound
	  }
	   if l.State != expected {
		  return ErrGenerationMismatch
	   }
	   l.State = next
	   switch next {
	  case LeaseStateUsed:
		   l.UsedAt = &now
	  case LeaseStateExpired:
		   l.ExpiredAt = &now
	  case LeaseStateRevoked:
		  l.RevokedAt = &now
		  l.RevokeNote = note
	   }
	   return nil
  }

  func (m *MemStore) RevokeOpenLeases(_ context.Context, userID, book string, now time.Time, note string) (int, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  count := 0
	  for _, l := range m.leases {
		   if l.UserID == userID && l.Book == book && l.State == LeaseStateOpen {
			  l.State = LeaseStateRevoked
			   l.RevokedAt = &now
			   l.RevokeNote = note
			  count++
		   }
	   }
	   return count, nil
  }

  func (m *MemStore) ListOpenLeases(_ context.Context, userID, book string) ([]LifeLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []LifeLease
	  for _, l := range m.leases {
		   if l.UserID == userID && l.Book == book && l.State == LeaseStateOpen {
			  cp := *l
			   out = append(out, cp)
		  }
	  }
	  return out, nil
   }

   func (m *MemStore) ListExpiredLeases(_ context.Context, now time.Time, limit int) ([]LifeLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []LifeLease
	  for _, l := range m.leases {
		   if l.State == LeaseStateOpen && now.After(l.ExpiresAt) {
			   cp := *l
			   out = append(out, cp)
			  if len(out) >= limit {
				  break
			  }
		   }
	  }
	  return out, nil
   }

// ── MoveBind ─────────────────────────────────────────────────────────────────

   func (m *MemStore) GetMoveBind(_ context.Context, bindID string) (*MoveBind, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   b, ok := m.binds[bindID]
	  if !ok {
		  return nil, ErrMoveBindNotFound
	   }
	  cp := *b
	  return &cp, nil
  }

  func (m *MemStore) GetMoveBindByIdem(_ context.Context, idemKey string) (*MoveBind, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.bindsByIdem[idemKey]
	   if !ok {
		  return nil, ErrMoveBindNotFound
	   }
	   b := m.binds[id]
	  cp := *b
	  return &cp, nil
   }

  func (m *MemStore) InsertMoveBind(_ context.Context, bind MoveBind) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.binds[bind.BindID]; ok {
		   return fmt.Errorf("acctlifegate: move bind %s already exists", bind.BindID)
	   }
	  if _, ok := m.bindsByIdem[bind.IdemKey]; ok {
		  return fmt.Errorf("acctlifegate: move bind idem_key %s already exists", bind.IdemKey)
	   }
	  cp := bind
	   m.binds[bind.BindID] = &cp
	   m.bindsByIdem[bind.IdemKey] = bind.BindID
	  return nil
  }

   func (m *MemStore) UpdateMoveBindState(_ context.Context, bindID string, expected, next MoveBindState, now time.Time, note string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   b, ok := m.binds[bindID]
	  if !ok {
		  return ErrMoveBindNotFound
	   }
	   if b.State != expected {
		  return ErrGenerationMismatch
	   }
	   b.State = next
	  b.ResolvedAt = &now
	  b.Note = note
	  return nil
   }

  func (m *MemStore) ListPendingMoveBinds(_ context.Context, userID, book string) ([]MoveBind, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []MoveBind
	  for _, b := range m.binds {
		  if b.State == MoveBindPending && b.UserID == userID &&
			   (b.FromBook == book || b.ToBook == book) {
			  cp := *b
			   out = append(out, cp)
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) ListExpiredMoveBinds(_ context.Context, now time.Time, limit int) ([]MoveBind, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []MoveBind
	   for _, b := range m.binds {
		  if b.State == MoveBindPending && now.After(b.ExpiresAt) {
			   cp := *b
			   out = append(out, cp)
			  if len(out) >= limit {
				   break
			  }
		   }
	   }
	  return out, nil
   }

// ── LifeDriftCase ────────────────────────────────────────────────────────────

   func (m *MemStore) GetDriftCase(_ context.Context, caseID string) (*LifeDriftCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   c, ok := m.drifts[caseID]
	   if !ok {
		   return nil, ErrDriftCaseNotFound
	  }
	  cp := *c
	  return &cp, nil
  }

   func (m *MemStore) ListOpenDriftCases(_ context.Context, userID, book string) ([]LifeDriftCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []LifeDriftCase
	  for _, c := range m.drifts {
		   if c.UserID == userID && c.Book == book && c.State != DriftStateClosed {
			  cp := *c
			  out = append(out, cp)
		   }
	   }
	   return out, nil
   }

  func (m *MemStore) ListAllOpenDriftCases(_ context.Context, limit int) ([]LifeDriftCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []LifeDriftCase
	  for _, c := range m.drifts {
		   if c.State != DriftStateClosed {
			   cp := *c
			   out = append(out, cp)
			  if len(out) >= limit {
				  break
			   }
		   }
	   }
	   return out, nil
  }

  func (m *MemStore) InsertDriftCase(_ context.Context, c LifeDriftCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.drifts[c.CaseID]; ok {
		  return fmt.Errorf("acctlifegate: drift case %s already exists", c.CaseID)
	   }
	  cp := c
	   m.drifts[c.CaseID] = &cp
	  return nil
  }

   func (m *MemStore) UpdateDriftState(_ context.Context, caseID string, expected, next DriftState, now time.Time, actor1, actor2, note string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  c, ok := m.drifts[caseID]
	  if !ok {
		  return ErrDriftCaseNotFound
	   }
	  if c.State != expected {
		  return ErrGenerationMismatch
	  }
	   c.State = next
	   switch next {
	   case DriftStateProven:
		   c.ProvenAt = &now
		  c.Actor1 = actor1
		  c.Actor2 = actor2
		   c.ProofNote = note
	   case DriftStateClosed:
		  c.ClosedAt = &now
		   c.CloseNote = note
	  }
	   return nil
  }

// ── 辅助 ─────────────────────────────────────────────────────────────────
// AllLeases 返回所有租约（仅供断言使用）。
  func (m *MemStore) AllLeases() []LifeLease {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]LifeLease, 0, len(m.leases))
	   for _, l := range m.leases {
		  out = append(out, *l)
	   }
	  return out
  }

// AllBinds 返回所有在途绑定（仅供断言使用）。

  func (m *MemStore) AllBinds() []MoveBind {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]MoveBind, 0, len(m.binds))
	  for _, b := range m.binds {
		  out = append(out, *b)
	   }
	   return out
   }

// AllDriftCases 返回所有漂移案件（仅供断言使用）。
   func (m *MemStore) AllDriftCases() []LifeDriftCase {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]LifeDriftCase, 0, len(m.drifts))
	  for _, c := range m.drifts {
		   out = append(out, *c)
	   }
	  return out
  }
