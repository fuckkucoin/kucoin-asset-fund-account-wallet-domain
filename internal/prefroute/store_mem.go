package prefroute

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

// MemStore 进程内 prefroute Store 实现（非）。
type MemStore struct {
	mu sync.RWMutex
	generations     map[string][]fundpref.PrefGeneration // key: userID
	defaultPrefs    map[string]fundpref.DefaultAccountPref
	pendingSwitches map[string]fundpref.PendingSwitch
	leases map[string]PrefLease
	routeSnaps map[string]RouteSnap
	driftCases      map[string]PrefDriftCase
	driftProofs     map[string]DriftProof
	inflightCredits map[string]InflightCredit
}

// NewMemStore 构造内存 Store。

  func NewMemStore() *MemStore {
	  return &MemStore{
		   generations:  make(map[string][]fundpref.PrefGeneration),
		   defaultPrefs: make(map[string]fundpref.DefaultAccountPref),
		  pendingSwitches: make(map[string]fundpref.PendingSwitch),
		   leases:     make(map[string]PrefLease),
		  routeSnaps: make(map[string]RouteSnap),
		   driftCases:      make(map[string]PrefDriftCase),
		  driftProofs:     make(map[string]DriftProof),
		  inflightCredits: make(map[string]InflightCredit),
	   }
  }

// ── PrefGeneration ──────────────────────────────────────────────────────────

   func (s *MemStore) GetGeneration(_ context.Context, userID string) (fundpref.PrefGeneration, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  gens, ok := s.generations[userID]
	   if !ok || len(gens) == 0 {
		  return fundpref.PrefGeneration{}, fundpref.ErrNotFound
	  }
	  return gens[len(gens)-1], nil
  }

  func (s *MemStore) SaveGeneration(_ context.Context, g fundpref.PrefGeneration) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   s.generations[g.UserID] = append(s.generations[g.UserID], g)
	   return nil
  }

  func (s *MemStore) ListGenerations(_ context.Context, userID string, limit int) ([]fundpref.PrefGeneration, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   gens := s.generations[userID]
	   if limit > 0 && len(gens) > limit {
		   return gens[len(gens)-limit:], nil
	  }
	   out := make([]fundpref.PrefGeneration, len(gens))
	  copy(out, gens)
	  return out, nil
   }

// ── DefaultAccountPref ──────────────────────────────────────────────────────

  func (s *MemStore) GetDefaultPref(_ context.Context, userID string) (fundpref.DefaultAccountPref, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  p, ok := s.defaultPrefs[userID]
	  if !ok {
		   return fundpref.DefaultAccountPref{}, fundpref.ErrNotFound
	  }
	  return p, nil
   }

  func (s *MemStore) SaveDefaultPref(_ context.Context, p fundpref.DefaultAccountPref) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   s.defaultPrefs[p.UserID] = p
	  return nil
  }

// ── PendingSwitch ───────────────────────────────────────────────────────────

   func (s *MemStore) SavePendingSwitch(_ context.Context, ps fundpref.PendingSwitch) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  s.pendingSwitches[ps.ID] = ps
	   return nil
  }

   func (s *MemStore) GetPendingSwitch(_ context.Context, id string) (fundpref.PendingSwitch, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  ps, ok := s.pendingSwitches[id]
	  if !ok {
		   return fundpref.PendingSwitch{}, fundpref.ErrNotFound
	   }
	  return ps, nil
  }

  func (s *MemStore) ListPendingSwitches(_ context.Context, userID string, state fundpref.PendingSwitchState) ([]fundpref.PendingSwitch, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []fundpref.PendingSwitch
	   for _, ps := range s.pendingSwitches {
		  if ps.UserID == userID && ps.State == state {
			   out = append(out, ps)
		  }
	  }
	   return out, nil
  }

  func (s *MemStore) UpdatePendingSwitch(_ context.Context,
	   ps fundpref.PendingSwitch) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.pendingSwitches[ps.ID]; !ok {
		  return fundpref.ErrNotFound
	  }
	  s.pendingSwitches[ps.ID] = ps
	  return nil
  }

// ── PrefLease ───────────────────────────────────────────────────────────────

   func (s *MemStore) SavePrefLease(_ context.Context, l PrefLease) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  s.leases[l.ID] = l
	  return nil
   }

   func (s *MemStore) GetPrefLease(_ context.Context, id string) (PrefLease, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  l, ok := s.leases[id]
	   if !ok {
		   return PrefLease{}, ErrNotFound
	  }
	  return l, nil
  }

   func (s *MemStore) UpdatePrefLease(_ context.Context, l PrefLease) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.leases[l.ID]; !ok {
		   return ErrNotFound
	   }
	   s.leases[l.ID] = l
	   return nil
   }

   func (s *MemStore) ListActiveLeases(_ context.Context, userID string) ([]PrefLease, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []PrefLease
	  for _, l := range s.leases {
		   if l.UserID == userID && (l.State == LeaseStateOpen || l.State == LeaseStateBound) {
			  out = append(out, l)
		  }
	  }
	   return out, nil
   }

   func (s *MemStore) ExpireStaleLeases(_ context.Context, userID string, newGen int64, now time.Time) ([]PrefLease, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  var expired []PrefLease
	   for id, l := range s.leases {
		  if l.UserID != userID {
			  continue
		   }
		   if (l.State == LeaseStateOpen || l.State == LeaseStateBound) && l.Generation < newGen {
			   l.State = LeaseStateSwitched
			  l.SwitchedAt = &now
			   l.BlockReason = fmt.Sprintf("generation advanced to %d", newGen)
			  s.leases[id] = l
			  expired = append(expired, l)
		   }
	  }
	   return expired, nil
  }

// ── RouteSnap ───────────────────────────────────────────────────────────────

  func (s *MemStore) SaveRouteSnap(_ context.Context, r RouteSnap) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   s.routeSnaps[r.ID] = r
	  return nil
  }

  func (s *MemStore) GetRouteSnap(_ context.Context, id string) (RouteSnap, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  r, ok := s.routeSnaps[id]
	  if !ok {
		  return RouteSnap{}, ErrNotFound
	   }
	   return r, nil
  }

   func (s *MemStore) UpdateRouteSnap(_ context.Context, r RouteSnap) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.routeSnaps[r.ID]; !ok {
		   return ErrNotFound
	  }
	   s.routeSnaps[r.ID] = r
	  return nil
   }

   func (s *MemStore) ListRouteSnapsByLease(_ context.Context, leaseID string) ([]RouteSnap, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   var out []RouteSnap
	   for _, r := range s.routeSnaps {
		  if r.LeaseID == leaseID {
			  out = append(out, r)
		   }
	   }
	   return out, nil
   }

// ── PrefDriftCase ───────────────────────────────────────────────────────────

   func (s *MemStore) SaveDriftCase(_ context.Context, d PrefDriftCase) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   s.driftCases[d.ID] = d
	  return nil
   }

   func (s *MemStore) GetDriftCase(_ context.Context, id string) (PrefDriftCase, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   d, ok := s.driftCases[id]
	  if !ok {
		   return PrefDriftCase{}, ErrNotFound
	  }
	  return d, nil
  }

  func (s *MemStore) UpdateDriftCase(_ context.Context, d PrefDriftCase) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.driftCases[d.ID]; !ok {
		  return ErrNotFound
	   }
	  s.driftCases[d.ID] = d
	   return nil
   }

  func (s *MemStore) ListOpenDriftCases(_ context.Context, userID string) ([]PrefDriftCase, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   var out []PrefDriftCase
	  for _, d := range s.driftCases {
		   if d.UserID == userID && d.State == DriftStateOpen {
			   out = append(out, d)
		  }
	   }
	   return out, nil
  }

  func (s *MemStore) HasOpenDriftCase(_ context.Context, userID string) (bool, string, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  for _, d := range s.driftCases {
		   if d.UserID == userID && d.State == DriftStateOpen {
			   return true, d.ID, nil
		   }
	   }
	  return false, "", nil
  }

// ── DriftProof ──────────────────────────────────────────────────────────────

  func (s *MemStore) SaveDriftProof(_ context.Context, p DriftProof) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   s.driftProofs[p.ID] = p
	  return nil
   }

   func (s *MemStore) GetDriftProof(_ context.Context, id string) (DriftProof, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  p, ok := s.driftProofs[id]
	  if !ok {
		  return DriftProof{}, ErrNotFound
	  }
	  return p, nil
   }

   func (s *MemStore) ListDriftProofs(_ context.Context, driftCaseID string) ([]DriftProof, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []DriftProof
	  for _, p := range s.driftProofs {
		   if p.DriftCaseID == driftCaseID {
			   out = append(out, p)
		   }
	   }
	  return out, nil
   }

// ── InflightCredit ──────────────────────────────────────────────────────────

   func (s *MemStore) RegisterInflightCredit(_ context.Context, ic InflightCredit) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   s.inflightCredits[ic.TxnID] = ic
	  return nil
   }

  func (s *MemStore) GetInflightCredit(_ context.Context, txnID string) (InflightCredit, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  ic, ok := s.inflightCredits[txnID]
	   if !ok {
		  return InflightCredit{}, ErrNotFound
	  }
	  return ic, nil
   }

  func (s *MemStore) UpdateInflightCredit(_ context.Context, ic InflightCredit) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.inflightCredits[ic.TxnID]; !ok {
		   return ErrNotFound
	  }
	  s.inflightCredits[ic.TxnID] = ic
	  return nil
   }

   func (s *MemStore) ListInflightCredits(_ context.Context, userID string, state InflightCreditState) ([]InflightCredit, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []InflightCredit
	   for _, ic := range s.inflightCredits {
		   if ic.UserID == userID && ic.State == state {
			  out = append(out, ic)
		  }
	   }
	  return out, nil
   }

// ── 辅助：HasInflightCredit（实现 fundpref.InflightChecker）──────────────────
// HasInflightCredit 检查用户是否有 pending 在途贷记。
   func (s *MemStore) HasInflightCredit(ctx context.Context, userID string) (bool, error) {
	  credits, err := s.ListInflightCredits(ctx, userID, InflightStatePending)
	   if err != nil {
		  return false, err
	   }
	   return len(credits) > 0, nil
  }
