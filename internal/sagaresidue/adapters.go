package sagaresidue

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// ── NopLedgerReader ───────────────────────────────────────────────────────────
// NopLedgerReader LedgerReader 的占位实现（须替换为ledger 适配器）。

type NopLedgerReader struct{}

// NewNopLedgerReader 构造 NopLedgerReader。
   func NewNopLedgerReader() LedgerReader { return &NopLedgerReader{} }

  func (n *NopLedgerReader) GetByIdem(_ context.Context, _ string) (*ledger.Transfer, error) {
	   return nil, ErrUnavailable
  }

// ── NopLedgerWriter ───────────────────────────────────────────────────────────
// NopLedgerWriter LedgerWriter 的占位实现。

type NopLedgerWriter struct{}

// NewNopLedgerWriter 构造 NopLedgerWriter。

   func NewNopLedgerWriter() LedgerWriter { return &NopLedgerWriter{} }

  func (n *NopLedgerWriter) ReverseTransfer(_ context.Context, _, _ string) (*ledger.Transfer, error) {
	  return nil, ErrUnavailable
  }

   func (n *NopLedgerWriter) SuspenseTransfer(_ context.Context, _, _, _, _ string) (*ledger.Transfer, error) {
	   return nil, ErrUnavailable
   }

// ── NopSagaRunLister ──────────────────────────────────────────────────────────
// NopSagaRunLister SagaRunLister 的占位实现（总是返回空列表）。

type NopSagaRunLister struct{}

// NewNopSagaRunLister 构造 NopSagaRunLister。

   func NewNopSagaRunLister() SagaRunLister { return &NopSagaRunLister{} }

   func (n *NopSagaRunLister) ListPartialFailed(_ context.Context, _ string, _ int) ([]string, error) {
	  return nil, nil
  }

// ── SQLStore 实现 IdemBlockStore ──────────────────────────────────────────────
// 编译时确认 SQLStore IdemBlockStore 接口。
var _ IdemBlockStore = (*SQLStore)(nil)

// Block 代理到 SQLStore.BlockIdem，实现 IdemBlockStore 接口。
   func (s *SQLStore) Block(ctx context.Context, idemKey, reason string) error {
	   return s.BlockIdem(ctx, idemKey, reason)
   }

// ── MemStore：内存 Store 实现（供）────────────────────────────────────────
// MemStore Store 的内存实现，非线程安全，仅供单元使用。
type MemStore struct {
	runs map[string]*ResidueRun // key: run.ID 和 "saga:"+saga_run_id
	items map[string]*ResidueItem // key: item.ID
	reports map[string][]*ResidueReport // key: saga_run_id
	plans map[string]*CloseoutPlan // key: plan.ID
	legs map[string]*CloseoutLeg // key: leg.ID
	proofs     map[string]*CloseProof // key: proof.ID
	proofByRun map[string]*CloseProof // key: residue_run_id
	blocks  map[string]string // idem_key → reason
	mutexes *MemMutexStore
}

// NewMemStore 构造内存 Store。
  func NewMemStore() *MemStore {
	  return &MemStore{
		   runs: map[string]*ResidueRun{},
		   items:   map[string]*ResidueItem{},
		   reports: map[string][]*ResidueReport{},
		   plans: map[string]*CloseoutPlan{},
		  legs:  map[string]*CloseoutLeg{},
		  proofs:     map[string]*CloseProof{},
		   proofByRun: map[string]*CloseProof{},
		   blocks: map[string]string{},
		   mutexes: NewMemMutexStore(),
	   }
  }

// 编译时确认 MemStore Store 接口。

var _ Store = (*MemStore)(nil)

  func (m *MemStore) SaveResidueRun(_ context.Context, r *ResidueRun) error {
	   cp := *r
	   m.runs[r.ID] = &cp
	  m.runs["saga:"+r.SagaRunID] = &cp
	   return nil
  }

  func (m *MemStore) GetResidueRun(_ context.Context, id string) (*ResidueRun, error) {
	   r := m.runs[id]
	   if r == nil {
		   return nil, ErrResidueRunNotFound
	   }
	  cp := *r
	   return &cp, nil
  }

   func (m *MemStore) GetResidueRunBySagaRunID(_ context.Context, sagaRunID string) (*ResidueRun, error) {
	  r := m.runs["saga:"+sagaRunID]
	   if r == nil {
		   return nil, ErrResidueRunNotFound
	   }
	  cp := *r
	  return &cp, nil
  }

   func (m *MemStore) ListResidueRuns(_ context.Context, req RunListReq) ([]ResidueRun, error) {
	  var out []ResidueRun
	  for id, r := range m.runs {
		   // 跳过 saga: 索引键
		   if len(id) > 5 && id[:5] == "saga:" {
			   continue
		   }
		  if req.State != "" && r.State != req.State {
			  continue
		   }
		  if req.Severity != "" && r.Severity != req.Severity {
			   continue
		  }
		  if req.UserID != "" && r.UserID != req.UserID {
			  continue
		  }
		  out = append(out, *r)
	   }
	   return out, nil
   }

   func (m *MemStore) UpdateResidueRunState(_ context.Context, id string, newState RunState, updatedAt time.Time) error {
	   r := m.runs[id]
	  if r == nil {
		  return ErrResidueRunNotFound
	  }
	  r.State = newState
	  r.UpdatedAt = updatedAt
	  return nil
   }

   func (m *MemStore) SaveResidueItem(_ context.Context, item *ResidueItem) error {
	  cp := *item
	  m.items[item.ID] = &cp
	  return nil
  }

   func (m *MemStore) ListResidueItems(_ context.Context, residueRunID string) ([]ResidueItem, error) {
	   var out []ResidueItem
	  for _, item := range m.items {
		   if item.ResidueRunID == residueRunID {
			  out = append(out, *item)
		   }
	  }
	   return out, nil
   }

   func (m *MemStore) ConfirmResidueItems(_ context.Context, ids []string, _ time.Time) error {
	   for _, id := range ids {
		   item := m.items[id]
		  if item != nil {
			  item.IsConfirmed = true
		  }
	   }
	  return nil
   }

   func (m *MemStore) SaveReport(_ context.Context, report *ResidueReport) error {
	   cp := *report
	   m.reports[report.SagaRunID] = append(m.reports[report.SagaRunID], &cp)
	  return nil
   }

  func (m *MemStore) GetLatestReport(_ context.Context, sagaRunID string) (*ResidueReport, error) {
	  reps := m.reports[sagaRunID]
	  if len(reps) == 0 {
		  return nil, ErrResidueRunNotFound
	  }
	  cp := *reps[len(reps)-1]
	  return &cp, nil
  }

  func (m *MemStore) ListReports(_ context.Context, sagaRunID string, _ int) ([]ResidueReport, error) {
	   reps := m.reports[sagaRunID]
	  out := make([]ResidueReport, 0, len(reps))
	   for _, r := range reps {
		   out = append(out, *r)
	   }
	  return out, nil
  }

  func (m *MemStore) SavePlan(_ context.Context, plan *CloseoutPlan) error {
	   cp := *plan
	   m.plans[plan.ID] = &cp
	  return nil
  }

   func (m *MemStore) GetPlan(_ context.Context, id string) (*CloseoutPlan, error) {
	   p := m.plans[id]
	  if p == nil {
		   return nil, ErrResidueRunNotFound
	   }
	   cp := *p
	  return &cp, nil
  }

   func (m *MemStore) GetActivePlan(_ context.Context, residueRunID string) (*CloseoutPlan, error) {
	  for _, p := range m.plans {
		   if p.ResidueRunID == residueRunID &&
			  (p.State == RunStateClosing || p.State == RunStateResidueOpen) {
			   cp := *p
			  return &cp, nil
		   }
	   }
	   return nil, ErrResidueRunNotFound
   }

   func (m *MemStore) SaveLeg(_ context.Context, leg *CloseoutLeg) error {
	   cp := *leg
	   m.legs[leg.ID] = &cp
	  return nil
   }

  func (m *MemStore) GetLeg(_ context.Context, id string) (*CloseoutLeg, error) {
	  l := m.legs[id]
	   if l == nil {
		  return nil, ErrLegNotFound
	  }
	   cp := *l
	   return &cp, nil
  }

  func (m *MemStore) ListLegs(_ context.Context, req LegListReq) ([]CloseoutLeg, error) {
	  var out []CloseoutLeg
	  for _, l := range m.legs {
		  if req.PlanID != "" && l.PlanID != req.PlanID {
			   continue
		   }
		   if req.ResidueRunID != "" && l.ResidueRunID != req.ResidueRunID {
			  continue
		   }
		   if req.State != "" && l.State != req.State {
			  continue
		  }
		   out = append(out, *l)
	   }
	  sortLegsBySeq(out)
	   return out, nil
  }

  func (m *MemStore) SaveProof(_ context.Context, proof *CloseProof) error {
	  cp := *proof
	  m.proofs[proof.ID] = &cp
	   m.proofByRun[proof.ResidueRunID] = &cp
	   return nil
   }

  func (m *MemStore) GetProof(_ context.Context, id string) (*CloseProof, error) {
	  p := m.proofs[id]
	  if p == nil {
		   return nil, ErrProofNotFound
	  }
	  cp := *p
	  return &cp, nil
   }

  func (m *MemStore) GetProofByRunID(_ context.Context, residueRunID string) (*CloseProof, error) {
	   p := m.proofByRun[residueRunID]
	  if p == nil {
		   return nil, ErrProofNotFound
	   }
	  cp := *p
	  return &cp, nil
   }

   func (m *MemStore) BlockIdem(_ context.Context, idemKey, reason string) error {
	   if _, ok := m.blocks[idemKey]; !ok {
		   m.blocks[idemKey] = reason
	   }
	  return nil
   }

   func (m *MemStore) IsIdemBlocked(_ context.Context, idemKey string) (bool, error) {
	   _, ok := m.blocks[idemKey]
	   return ok, nil
  }

   func (m *MemStore) Block(ctx context.Context, idemKey, reason string) error {
	  return m.BlockIdem(ctx, idemKey, reason)
   }

// MemStore 同时实现 MutexStore 接口，代理给内嵌 MemMutexStore。
var _ MutexStore = (*MemStore)(nil)

   func (m *MemStore) AcquireMutex(ctx context.Context, sagaRunID, holderID string, ttl time.Duration) (*MutexGen, error) {
	   return m.mutexes.AcquireMutex(ctx, sagaRunID, holderID, ttl)
   }

  func (m *MemStore) RefreshMutex(ctx context.Context, sagaRunID string, generation int64, holderID string, ttl time.Duration) error {
	   return m.mutexes.RefreshMutex(ctx, sagaRunID, generation, holderID, ttl)
   }

   func (m *MemStore) ReleaseMutex(ctx context.Context, sagaRunID string, generation int64, holderID string) error {
	   return m.mutexes.ReleaseMutex(ctx, sagaRunID, generation, holderID)
   }

   func (m *MemStore) GetMutex(ctx context.Context, sagaRunID string) (*MutexGen, error) {
	  return m.mutexes.GetMutex(ctx, sagaRunID)
   }

  func (m *MemStore) ReclaimExpiredMutexes(ctx context.Context) (int, error) {
	  return m.mutexes.ReclaimExpiredMutexes(ctx)
   }
