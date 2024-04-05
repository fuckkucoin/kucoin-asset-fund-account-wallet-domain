package cerbind

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// Store 是仪式编排的持久化接口。
type Store interface {
	PutPolicy(ctx context.Context, p Policy) error
	Policies(ctx context.Context) ([]Policy, error)
	PutSession(ctx context.Context, s Session) error
	Session(ctx context.Context, id string) (Session, bool, error)
	LiveSessions(ctx context.Context) ([]Session,
		error)
	ListSessions(ctx context.Context, limit int) ([]Session, error)
	PutBinding(ctx context.Context, b Binding) error
	BindingsBySession(ctx context.Context,
		sessionID string) ([]Binding, error)
	ActiveByBatch(ctx context.Context, batchID string) (Binding, bool, error)
	ActiveBindings(ctx context.Context) ([]Binding, error)
	PutReceipt(ctx context.Context,
		r Receipt) error
	Receipts(ctx context.Context, sessionID string) ([]Receipt, error)
	PutAudit(ctx context.Context, a Audit) error
	Audits(ctx context.Context, sessionID, batchID string, limit int) ([]Audit, error)
	PutSnap(ctx context.Context, s Snapshot) error
	Snap(ctx context.Context,
		batchID string) (Snapshot, bool, error)
	PutAlias(ctx context.Context, ref, batchID, kind string) error
	Alias(ctx context.Context, ref string) (batchID string, ok bool, err error)
}

// Mem 是进程内存储。

type Mem struct {
	mu  sync.Mutex
	pol map[string]Policy
	ses map[string]Session
	bind map[string]Binding
	rcpt  map[string]Receipt
	snap  map[string]Snapshot
	alias map[string]aliasRow
	aud []Audit
}

type aliasRow struct {
	BatchID string
	Kind string
}

// NewMem 返回空存储。
   func NewMem() *Mem {
	   return &Mem{
		   pol:  map[string]Policy{},
		   ses:  map[string]Session{},
		  bind: map[string]Binding{},
		   rcpt:  map[string]Receipt{},
		   snap:  map[string]Snapshot{},
		  alias: map[string]aliasRow{},
	   }
  }

  func (m *Mem) PutPolicy(_ context.Context, p Policy) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.pol[p.ID] = p.Copy()
	  return nil
  }

  func (m *Mem) Policies(_ context.Context) ([]Policy, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]Policy, 0, len(m.pol))
	  for _, p := range m.pol {
		  out = append(out, p.Copy())
	   }
	  sort.Slice(out, func(i, j int) bool {
		  if out[i].Priority != out[j].Priority {
			   return out[i].Priority > out[j].Priority
		   }
		  return out[i].ID < out[j].ID
	  })
	  return out, nil
   }

  func (m *Mem) PutSession(_ context.Context, s Session) error {
	   if m == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.ses[s.ID] = s.Copy()
	  return nil
  }

   func (m *Mem) Session(_ context.Context, id string) (Session, bool, error) {
	   if m == nil {
		  return Session{}, false, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  s, ok := m.ses[id]
	  if !ok {
		   return Session{}, false, nil
	  }
	  return s.Copy(), true, nil
  }

   func (m *Mem) LiveSessions(_ context.Context) ([]Session, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Session, 0)
	   for _, s := range m.ses {
		  if s.State.Live() {
			   out = append(out, s.Copy())
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	   return out, nil
   }

  func (m *Mem) ListSessions(_ context.Context, limit int) ([]Session, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	  limit = clampLimit(limit)
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Session, 0, len(m.ses))
	  for _, s := range m.ses {
		  out = append(out, s.Copy())
	  }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	  if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }

  func (m *Mem) PutBinding(_ context.Context, b Binding) error {
	   if m == nil {
		  return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.bind[b.ID] = b.Copy()
	   return nil
   }

   func (m *Mem) BindingsBySession(_ context.Context, sessionID string) ([]Binding, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]Binding, 0)
	  for _, b := range m.bind {
		   if b.SessionID == sessionID {
			  out = append(out, b.Copy())
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].BoundAt.Before(out[j].BoundAt) })
	   return out, nil
  }

   func (m *Mem) ActiveByBatch(_ context.Context, batchID string) (Binding, bool, error) {
	  if m == nil {
		   return Binding{}, false, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var found []Binding
	   for _, b := range m.bind {
		   if b.BatchID == batchID && b.State == BindActive {
			  found = append(found, b.Copy())
		   }
	  }
	   if len(found) == 0 {
		  return Binding{}, false, nil
	   }
	  if len(found) > 1 {
		   return Binding{}, false, ErrBindMode
	  }
	   return found[0], true, nil
   }

  func (m *Mem) ActiveBindings(_ context.Context) ([]Binding, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Binding, 0)
	   for _, b := range m.bind {
		   if b.State == BindActive {
			   out = append(out, b.Copy())
		   }
	   }
	   sort.Slice(out, func(i, j int) bool {
		   if out[i].BoundAt.Equal(out[j].BoundAt) {
			   return out[i].ID < out[j].ID
		  }
		   return out[i].BoundAt.Before(out[j].BoundAt)
	   })
	   return out, nil
   }

   func (m *Mem) PutReceipt(_ context.Context, r Receipt) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.rcpt[r.ID] = r.Copy()
	   return nil
   }

  func (m *Mem) Receipts(_ context.Context, sessionID string) ([]Receipt, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]Receipt, 0)
	   for _, r := range m.rcpt {
		   if r.SessionID == sessionID {
			   out = append(out, r.Copy())
		   }
	  }
	  sort.Slice(out, func(i, j int) bool {
		   if out[i].At.Equal(out[j].At) {
			   return out[i].Actor < out[j].Actor
		  }
		   return out[i].At.Before(out[j].At)
	  })
	  return out, nil
  }

  func (m *Mem) PutAudit(_ context.Context, a Audit) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.aud = append(m.aud, a.Copy())
	   return nil
   }

  func (m *Mem) Audits(_ context.Context, sessionID, batchID string, limit int) ([]Audit, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   limit = clampLimit(limit)
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Audit, 0)
	  for i := len(m.aud) - 1; i >= 0; i-- {
		   a := m.aud[i]
		   if sessionID != "" && a.SessionID != sessionID {
			  continue
		   }
		   if batchID != "" && a.BatchID != batchID {
			   continue
		  }
		  out = append(out, a.Copy())
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
  }

  func (m *Mem) PutSnap(_ context.Context, s Snapshot) error {
	   if m == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.snap[s.BatchID] = s.Copy()
	  return nil
  }

  func (m *Mem) Snap(_ context.Context, batchID string) (Snapshot, bool, error) {
	   if m == nil {
		  return Snapshot{}, false, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  s, ok := m.snap[batchID]
	   if !ok {
		  return Snapshot{}, false, nil
	   }
	  return s.Copy(), true, nil
  }

  func (m *Mem) PutAlias(_ context.Context, ref, batchID, kind string) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	  ref = strings.TrimSpace(ref)
	  if ref == "" || batchID == "" {
		   return ErrAlias
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if cur, ok := m.alias[ref]; ok && cur.BatchID != batchID {
		   return ErrAlias
	  }
	  m.alias[ref] = aliasRow{BatchID: batchID, Kind: kind}
	  return nil
   }

   func (m *Mem) Alias(_ context.Context, ref string) (string, bool, error) {
	   if m == nil {
		  return "", false, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cur, ok := m.alias[strings.TrimSpace(ref)]
	   if !ok {
		  return "", false, nil
	   }
	   return cur.BatchID, true, nil
   }

  func clampLimit(n int) int {
	  if n <= 0 {
		  return 50
	   }
	   if n > 200 {
		   return 200
	   }
	   return n
  }

var _ Store = (*Mem)(nil)
