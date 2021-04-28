package clrcreditgate

import (
	"context"
	"sync"
	"time"
)

// MemStore 基于内存的 Store 实现（专用）。

type MemStore struct {
	mu sync.Mutex
	// leases 按 LeaseID 索引
	leases map[string]*ReleaseCreditLease
	// leasesByItem 按 ItemID 索引（每个 Item 当前最新 Lease）
	leasesByItem map[string]*ReleaseCreditLease
	// proofs 按 ProofID 索引
	proofs map[string]*ClearingCreditProof
	// okProofsByItem 按 ItemID 索引（Kind=credit_ok 防双贷）
	okProofsByItem map[string]*ClearingCreditProof
	// drains 按 BatchID 索引
	drains map[string]*BatchDrainState
	clock func() time.Time
}

// NewMemStore 构造内存 Store。
  func NewMemStore() *MemStore {
	  return &MemStore{
		   leases:         make(map[string]*ReleaseCreditLease),
		   leasesByItem:   make(map[string]*ReleaseCreditLease),
		  proofs:         make(map[string]*ClearingCreditProof),
		  okProofsByItem: make(map[string]*ClearingCreditProof),
		   drains:         make(map[string]*BatchDrainState),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟。

  func (m *MemStore) WithClock(fn func() time.Time) *MemStore {
	   if fn != nil {
		  m.clock = fn
	   }
	   return m
  }

  // ── ReleaseCreditLease ───────────────────────────────────────────────────────

  func (m *MemStore) SaveLease(_ context.Context, l *ReleaseCreditLease) error {
	  if m == nil || l == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  // 幂等：同 LeaseID 已存在则忽略（重复写请求）
	   if _, ok := m.leases[l.LeaseID]; ok {
		   return nil
	   }
	   // 同 ItemID issued Lease 不可重复签发
	  if cur, ok := m.leasesByItem[l.ItemID]; ok && cur.State == LeaseIssued {
		   return ErrLeaseAlreadyIssued
	  }
	  cp := *l
	  m.leases[l.LeaseID] = &cp
	  m.leasesByItem[l.ItemID] = &cp
	   return nil
   }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*ReleaseCreditLease, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	  if !ok {
		   return nil, ErrLeaseNotFound
	   }
	   cp := *l
	   return &cp, nil
  }

   func (m *MemStore) GetLeaseByItem(_ context.Context, itemID string) (*ReleaseCreditLease, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leasesByItem[itemID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *l
	  return &cp, nil
   }

   func (m *MemStore) ConsumeLease(_ context.Context, leaseID string, fence Fence, proofID string, at time.Time) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	   if !ok {
		  return ErrLeaseNotFound
	  }
	   switch l.State {
	   case LeaseConsumed:
		  return ErrLeaseConsumed
	  case LeaseExpired:
		   return ErrLeaseExpired
	  case LeaseVoid:
		   return ErrLeaseVoid
	  }
	  // Fence 校验
	   if l.Fence.Epoch != fence.Epoch || l.Fence.Token != fence.Token {
		  return ErrLeaseFenceStale
	   }
	   l.State = LeaseConsumed
	  l.ConsumedAt = at
	  l.ConsumedByProof = proofID
	  l.UpdatedAt = at
	  // 同步更新 leasesByItem 索引
	   if cur := m.leasesByItem[l.ItemID]; cur != nil && cur.LeaseID == leaseID {
		   m.leasesByItem[l.ItemID] = l
	  }
	   return nil
  }

  func (m *MemStore) MarkLeaseExpired(_ context.Context, leaseID string, at time.Time) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	  if !ok {
		   return ErrLeaseNotFound
	  }
	  if l.State != LeaseIssued {
		  return ErrInvalidTransition
	  }
	  l.State = LeaseExpired
	  l.ExpiredAt = at
	  l.UpdatedAt = at
	   return nil
   }

  func (m *MemStore) MarkLeaseVoid(_ context.Context, leaseID string, reason string, at time.Time) error {
	   if m == nil {
		  return ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[leaseID]
	  if !ok {
		   return ErrLeaseNotFound
	  }
	   if l.State != LeaseIssued {
		  return ErrInvalidTransition
	   }
	  l.State = LeaseVoid
	  l.VoidAt = at
	   l.VoidReason = reason
	   l.UpdatedAt = at
	   return nil
   }

   func (m *MemStore) ListLeases(_ context.Context, f LeaseListFilter) ([]*ReleaseCreditLease, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	   f.Fill()
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  stateSet := make(map[LeaseState]struct{}, len(f.States))
	  for _, s := range f.States {
		   stateSet[s] = struct{}{}
	   }
	  out := make([]*ReleaseCreditLease, 0)
	  for _, l := range m.leases {
		  if f.BatchID != "" && l.BatchID != f.BatchID {
			  continue
		   }
		   if f.ItemID != "" && l.ItemID != f.ItemID {
			  continue
		   }
		  if len(stateSet) > 0 {
			   if _, ok := stateSet[l.State]; !ok {
				  continue
			   }
		  }
		   if !f.ExpiredBefore.IsZero() && !l.ExpiresAt.Before(f.ExpiredBefore) {
			   continue
		   }
		   if !f.IssuedBefore.IsZero() && !l.IssuedAt.Before(f.IssuedBefore) {
			   continue
		  }
		   cp := *l
		  out = append(out, &cp)
		   if len(out) >= f.Limit {
			  break
		  }
	   }
	   return out, nil
   }

   func (m *MemStore) CountUnconsumedLeases(_ context.Context, batchID string, now time.Time) (int, error) {
	   if m == nil {
		  return 0, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  count := 0
	   for _, l := range m.leases {
		   if l.BatchID != batchID {
			  continue
		   }
		  if l.State == LeaseIssued && now.Before(l.ExpiresAt) {
			   count++
		  }
	   }
	   return count, nil
  }

// ── ClearingCreditProof ──────────────────────────────────────────────────────

  func (m *MemStore) SaveProof(_ context.Context, p *ClearingCreditProof) error {
	  if m == nil || p == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   // 幂等：同 ProofID 已存在则忽略
	  if _, ok := m.proofs[p.ProofID]; ok {
		  return nil
	   }
	   // 防双贷：同 ItemID credit_ok 不可重复写
	   if p.Kind == ProofCreditOK {
		   if _, exists := m.okProofsByItem[p.ItemID]; exists {
			   return ErrAlreadyProven
		   }
	  }
	   cp := *p
	   m.proofs[p.ProofID] = &cp
	   if p.Kind == ProofCreditOK {
		   m.okProofsByItem[p.ItemID] = &cp
	  }
	   return nil
   }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (*ClearingCreditProof, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.proofs[proofID]
	   if !ok {
		  return nil, ErrProofNotFound
	   }
	  cp := *p
	  return &cp, nil
   }

   func (m *MemStore) GetOKProofByItem(_ context.Context, itemID string) (*ClearingCreditProof, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.okProofsByItem[itemID]
	  if !ok {
		   return nil, ErrProofNotFound
	   }
	  cp := *p
	  return &cp, nil
   }

  func (m *MemStore) ListProofs(_ context.Context, f ProofListFilter) ([]*ClearingCreditProof, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	   f.Fill()
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   kindSet := make(map[ProofKind]struct{}, len(f.Kinds))
	  for _, k := range f.Kinds {
		  kindSet[k] = struct{}{}
	   }
	  out := make([]*ClearingCreditProof, 0)
	   for _, p := range m.proofs {
		  if f.BatchID != "" && p.BatchID != f.BatchID {
			  continue
		   }
		  if f.ItemID != "" && p.ItemID != f.ItemID {
			  continue
		  }
		   if len(kindSet) > 0 {
			  if _, ok := kindSet[p.Kind]; !ok {
				   continue
			   }
		   }
		   cp := *p
		  out = append(out, &cp)
		   if len(out) >= f.Limit {
			  break
		  }
	  }
	  return out, nil
   }

// ── BatchDrainState ──────────────────────────────────────────────────────────

  func (m *MemStore) GetOrCreateBatchDrain(_ context.Context, batchID string, now time.Time) (*BatchDrainState, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if d, ok := m.drains[batchID]; ok {
		  cp := *d
		   return &cp, nil
	  }
	  d := &BatchDrainState{
		  BatchID: batchID,
		   Status:     DrainPending,
		  SnapshotAt: now,
		  CreatedAt:  now,
		  UpdatedAt: now,
	  }
	  cp := *d
	  m.drains[batchID] = &cp
	   return d, nil
  }

  func (m *MemStore) UpdateBatchDrainStatus(_ context.Context, batchID string, from, to DrainStatus, patch *BatchDrainState) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   d, ok := m.drains[batchID]
	   if !ok {
		   return ErrBatchNotFound
	  }
	  if d.Status != from {
		   return ErrInvalidTransition
	   }
	   d.Status = to
	   if patch != nil {
		   if !patch.DrainStartedAt.IsZero() {
			   d.DrainStartedAt = patch.DrainStartedAt
		  }
		  if !patch.DrainTimeoutAt.IsZero() {
			   d.DrainTimeoutAt = patch.DrainTimeoutAt
		   }
		  if patch.TotalItems > 0 {
			  d.TotalItems = patch.TotalItems
		  }
		   if patch.ReleasedItems > 0 {
			   d.ReleasedItems = patch.ReleasedItems
		   }
		   if patch.AppliedItems > 0 {
			   d.AppliedItems = patch.AppliedItems
		  }
		  d.UnconsumedLeases = patch.UnconsumedLeases
		   d.InFlightApplies = patch.InFlightApplies
		   d.ExceptedItems = patch.ExceptedItems
		   if !patch.SnapshotAt.IsZero() {
			   d.SnapshotAt = patch.SnapshotAt
		   }
		  if !patch.UpdatedAt.IsZero() {
			   d.UpdatedAt = patch.UpdatedAt
		  }
	  }
	   return nil
  }

   func (m *MemStore) GetBatchDrain(_ context.Context, batchID string) (*BatchDrainState, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  d, ok := m.drains[batchID]
	  if !ok {
		  return nil, ErrBatchNotFound
	  }
	  cp := *d
	  return &cp, nil
   }
