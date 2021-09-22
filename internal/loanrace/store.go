package loanrace

import (
	"sync"
	"time"
)

// Store 内存竞态存储。环境替换为 SQL 实现。

type Store struct {
	mu     sync.Mutex
	leases map[string]*OverdueLease
	gates     map[string]*PayGate
	writeoffs map[string]*WriteoffCase
	proofs   map[string]*UnfreezeProof
	suspense map[string]*SuspenseEntry
	states   map[string]LoanRaceState
	frozen map[string]bool
	users map[string]string // loan_id → user_id
	byLoanLease map[string]string // loan_id → active lease id
	byLoanGate     map[string]string // loan_id → open gate id
	byLoanWriteoff map[string]string // loan_id → writeoff id
	byLoanProof map[string]string // loan_id → proof id
}

  func NewStore() *Store {
	  return &Store{
		  leases:    make(map[string]*OverdueLease),
		   gates:     make(map[string]*PayGate),
		  writeoffs: make(map[string]*WriteoffCase),
		   proofs:    make(map[string]*UnfreezeProof),
		   suspense: make(map[string]*SuspenseEntry),
		   states: make(map[string]LoanRaceState),
		  frozen: make(map[string]bool),
		   users: make(map[string]string),
		  byLoanLease: make(map[string]string),
		   byLoanGate:     make(map[string]string),
		   byLoanWriteoff: make(map[string]string),
		   byLoanProof:    make(map[string]string),
	   }
   }

// ── Loan state ──

   func (st *Store) RegisterLoan(loanID, userID string) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  if _, ok := st.states[loanID]; !ok {
		   st.states[loanID] = LRActive
	  }
	   st.users[loanID] = userID
  }

   func (st *Store) GetRaceState(loanID string) (LoanRaceState, bool) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  s, ok := st.states[loanID]
	  return s, ok
  }

  func (st *Store) SetRaceState(loanID string, s LoanRaceState) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   st.states[loanID] = s
  }

   func (st *Store) IsFrozen(loanID string) bool {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	   return st.frozen[loanID]
   }

   func (st *Store) SetFrozen(loanID string, v bool) {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	  st.frozen[loanID] = v
  }

   func (st *Store) GetUserID(loanID string) (string, bool) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  uid, ok := st.users[loanID]
	  return uid, ok
  }

// ── Leases ──

  func (st *Store) PutLease(l *OverdueLease) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  st.leases[l.ID] = l
	  if l.State == LeaseAcquired || l.State == LeaseApplied {
		   st.byLoanLease[l.LoanID] = l.ID
	   }
   }

  func (st *Store) GetLease(id string) (*OverdueLease, bool) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  l, ok := st.leases[id]
	   return l, ok
  }

  func (st *Store) ActiveLeaseForLoan(loanID string) (*OverdueLease, bool) {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	  lid, ok := st.byLoanLease[loanID]
	  if !ok {
		  return nil, false
	  }
	  l, ok := st.leases[lid]
	  if !ok {
		   return nil, false
	   }
	  if l.State == LeaseReleased || l.State == LeaseExpired {
		   delete(st.byLoanLease, loanID)
		  return nil, false
	   }
	  return l, true
  }

  func (st *Store) ClearLeaseIndex(loanID string) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   delete(st.byLoanLease, loanID)
  }

  func (st *Store) AllLeases() []*OverdueLease {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  out := make([]*OverdueLease, 0, len(st.leases))
	  for _, l := range st.leases {
		  out = append(out, l)
	   }
	  return out
  }

// ── PayGates ──

  func (st *Store) PutGate(g *PayGate) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   st.gates[g.ID] = g
	   if g.State == GateOpen || g.State == GateCleared {
		   st.byLoanGate[g.LoanID] = g.ID
	  }
   }

   func (st *Store) GetGate(id string) (*PayGate, bool) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	   g, ok := st.gates[id]
	  return g, ok
   }

  func (st *Store) ActiveGateForLoan(loanID string) (*PayGate, bool) {
	   st.mu.Lock()
	  defer st.mu.Unlock()
	  gid, ok := st.byLoanGate[loanID]
	  if !ok {
		   return nil, false
	   }
	  g, ok := st.gates[gid]
	  if !ok {
		  return nil, false
	  }
	  if g.State == GateClosed || g.State == GateBlocked {
		   delete(st.byLoanGate, loanID)
		   return nil, false
	  }
	  return g, true
  }

   func (st *Store) ClearGateIndex(loanID string) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   delete(st.byLoanGate, loanID)
  }

// ── Writeoffs ──

   func (st *Store) PutWriteoff(w *WriteoffCase) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   st.writeoffs[w.ID] = w
	  st.byLoanWriteoff[w.LoanID] = w.ID
  }

  func (st *Store) GetWriteoff(id string) (*WriteoffCase, bool) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   w, ok := st.writeoffs[id]
	  return w, ok
   }

   func (st *Store) WriteoffForLoan(loanID string) (*WriteoffCase, bool) {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	  wid, ok := st.byLoanWriteoff[loanID]
	   if !ok {
		  return nil, false
	  }
	  w, ok := st.writeoffs[wid]
	  return w, ok
  }

// ── UnfreezeProofs ──

   func (st *Store) PutProof(p *UnfreezeProof) {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	  st.proofs[p.ID] = p
	   st.byLoanProof[p.LoanID] = p.ID
   }

  func (st *Store) GetProof(id string) (*UnfreezeProof, bool) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  p, ok := st.proofs[id]
	  return p, ok
  }

  func (st *Store) ProofForLoan(loanID string) (*UnfreezeProof, bool) {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	   pid, ok := st.byLoanProof[loanID]
	   if !ok {
		   return nil, false
	   }
	   p, ok := st.proofs[pid]
	  return p, ok
  }

// ── Suspense ──

  func (st *Store) PutSuspense(e *SuspenseEntry) {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	  st.suspense[e.ID] = e
   }

   func (st *Store) SuspenseForLoan(loanID string) []SuspenseEntry {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	  out := make([]SuspenseEntry, 0)
	  for _, e := range st.suspense {
		  if e.LoanID == loanID {
			  out = append(out, e.Copy())
		  }
	   }
	   return out
  }

// ── View ──

   func (st *Store) BuildView(loanID string) LoanRaceView {
	   st.mu.Lock()
	   defer st.mu.Unlock()
	  v := LoanRaceView{
		  LoanID: loanID,
		   UserID: st.users[loanID],
	   }
	   v.RaceState = st.states[loanID]
	  v.Frozen = st.frozen[loanID]
	   if lid, ok := st.byLoanLease[loanID]; ok {
		  if l, ok2 := st.leases[lid]; ok2 {
			   v.HasLease = l.State == LeaseAcquired || l.State == LeaseApplied
		   }
	  }
	  if gid, ok := st.byLoanGate[loanID]; ok {
		  if g, ok2 := st.gates[gid]; ok2 {
			  v.HasGate = g.State == GateOpen || g.State == GateCleared
		  }
	  }
	   if wid, ok := st.byLoanWriteoff[loanID]; ok {
		   if w, ok2 := st.writeoffs[wid]; ok2 {
			   v.WrittenOff = w.State == WOApplied
		   }
	   }
	   if pid, ok := st.byLoanProof[loanID]; ok {
		  if p, ok2 := st.proofs[pid]; ok2 {
			  v.HasProof = p.State == ProofValid
		  }
	  }
	  return v
   }

// ── Overdue scanning helper ──

   func (st *Store) AllRegisteredLoans() []string {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	  out := make([]string, 0, len(st.states))
	   for k := range st.states {
		   out = append(out, k)
	  }
	  return out
   }

   func (st *Store) LoansInState(s LoanRaceState) []string {
	  st.mu.Lock()
	  defer st.mu.Unlock()
	   out := make([]string, 0)
	  for k, v := range st.states {
		   if v == s {
			  out = append(out, k)
		  }
	   }
	  return out
   }

// ExpiredLeases 返回所有已超时但未标记过期的租约。
  func (st *Store) ExpiredLeases(now time.Time) []*OverdueLease {
	  st.mu.Lock()
	   defer st.mu.Unlock()
	   out := make([]*OverdueLease, 0)
	   for _, l := range st.leases {
		  if (l.State == LeaseAcquired || l.State == LeaseApplied) && now.After(l.ExpiresAt) {
			  out = append(out, l)
		  }
	   }
	  return out
  }
