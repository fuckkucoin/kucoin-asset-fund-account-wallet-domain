package breachclosegate

import (
	"sync"
	"time"
	"github.com/google/uuid"
)

// Store breachclosegate 持久化接口。

type Store interface {
	// CreateLease 创建新 BreachLease。同一 breach_gen 已存在时返回 ErrDuplicateBreach。
	CreateLease(lease BreachLease) error
	// GetLeaseByID 按 ID 查询。
	GetLeaseByID(id string) (*BreachLease,
		error)
	// GetLeaseByCase 按 CaseID 查询活跃 BreachLease（State == blocking）。
	// 若存在多条（不应发生），返回最新创建的一条。
	GetLeaseByCase(caseID string) (*BreachLease, error)
	// GetLeaseByMovement 按 MovementID 查询活跃 BreachLease。
	GetLeaseByMovement(movementID string) (*BreachLease,
		error)
	// GetLeaseByBreachGen 按 breach_gen 令牌查询。
	GetLeaseByBreachGen(breachGen string) (*BreachLease, error)
	// UpdateLeaseState 原子更新 BreachLease 状态字段。
	UpdateLeaseState(id string, newState BreachState, update func(*BreachLease)) error
	  // ListActiveLeases 列出所有 State == blocking 的 BreachLease（最多 limit 条）。
	  ListActiveLeases(limit int) []BreachLease
	  // ListLeasesByCase 按 CaseID 列出所有 BreachLease（含历史）。
	   ListLeasesByCase(caseID string, limit int) []BreachLease
	   // CreateProof 创建违约证明（幂等：同一 breach_lease_id + block_outcome 已存在则跳过）。
	  CreateProof(proof SlaBreachProof) error
	   // ListProofsByLease 按 BreachLeaseID 列出所有证明。
	   ListProofsByLease(leaseID string, limit int) []SlaBreachProof
	  // ListAllProofs 列出所有证明（最多 limit 条，按 RecordedAt 倒序）。
	   ListAllProofs(limit int) []SlaBreachProof
	  // CreateClearance 创建四眼清障工单（同一 BreachLeaseID 只能有一个 pending 工单）。
	   CreateClearance(c BreachClearance) error
	  // GetClearanceByID 按 ID 查询。
	  GetClearanceByID(id string) (*BreachClearance, error)
	  // GetPendingClearanceByLease 查询 BreachLeaseID 对应的 pending 清障工单。
	   GetPendingClearanceByLease(leaseID string) (*BreachClearance, error)
	  // UpdateClearanceState 原子更新清障工单状态。
	   UpdateClearanceState(id string, update func(*BreachClearance)) error
	  // ListClearances 列出所有清障工单（最多 limit 条，按 ProposedAt 倒序）。
	  ListClearances(limit int) []BreachClearance
  }

   // MemStore 内存实现。
  type MemStore struct {
	   mu sync.Mutex
	  leases     map[string]*BreachLease     // id → lease
	  byCase     map[string]string           // caseID → lease id（仅活跃）
	  byMovement map[string]string           // movementID → lease id（仅活跃）
	  byGen      map[string]string           // breachGen → lease id
	  proofs     map[string]*SlaBreachProof  // id → proof
	   proofByKey map[string]string           // leaseID+":"+outcome → proof id（幂等键）
	  clears     map[string]*BreachClearance // id → clearance
	  pendingClr map[string]string // leaseID → clearance id（pending）
   }

   // NewMemStore 创建内存 Store。
   func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: map[string]*BreachLease{},
		   byCase: map[string]string{},
		  byMovement: map[string]string{},
		   byGen:      map[string]string{},
		   proofs:     map[string]*SlaBreachProof{},
		   proofByKey: map[string]string{},
		  clears:     map[string]*BreachClearance{},
		   pendingClr: map[string]string{},
	   }
   }

// ── BreachLease ──────────────────────────────────────────────────────────────

   func (m *MemStore) CreateLease(lease BreachLease) error {
	   if lease.ID == "" {
		  lease.ID = uuid.NewString()
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, dup := m.byGen[lease.BreachGen]; dup {
		   return ErrDuplicateBreach
	   }
	  cp := lease
	  m.leases[cp.ID] = &cp
	   if cp.State == BreachStateBlocking {
		  m.byCase[cp.CaseID] = cp.ID
		  m.byMovement[cp.MovementID] = cp.ID
	   }
	  m.byGen[cp.BreachGen] = cp.ID
	   return nil
   }

   func (m *MemStore) GetLeaseByID(id string) (*BreachLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[id]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *l
	  return &cp, nil
   }

   func (m *MemStore) GetLeaseByCase(caseID string) (*BreachLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  id, ok := m.byCase[caseID]
	   if !ok {
		  return nil, ErrNoActiveBreach
	   }
	  l, ok := m.leases[id]
	  if !ok || !l.Active() {
		   delete(m.byCase, caseID)
		   return nil, ErrNoActiveBreach
	  }
	   cp := *l
	   return &cp, nil
  }

   func (m *MemStore) GetLeaseByMovement(movementID string) (*BreachLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   id, ok := m.byMovement[movementID]
	   if !ok {
		  return nil, ErrNoActiveBreach
	  }
	  l, ok := m.leases[id]
	   if !ok || !l.Active() {
		  delete(m.byMovement, movementID)
		  return nil, ErrNoActiveBreach
	   }
	   cp := *l
	  return &cp, nil
  }

  func (m *MemStore) GetLeaseByBreachGen(breachGen string) (*BreachLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   id, ok := m.byGen[breachGen]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  l := m.leases[id]
	  if l == nil {
		  return nil, ErrLeaseNotFound
	   }
	  cp := *l
	   return &cp, nil
   }

  func (m *MemStore) UpdateLeaseState(id string, newState BreachState, update func(*BreachLease)) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[id]
	  if !ok {
		   return ErrLeaseNotFound
	  }
	   prev := l.State
	   if update != nil {
		  update(l)
	   }
	  l.State = newState
	  l.UpdatedAt = time.Now().UTC()
	   // 若状态不再是 blocking，清理正向索引。
	   if prev == BreachStateBlocking && newState != BreachStateBlocking {
		   delete(m.byCase, l.CaseID)
		   delete(m.byMovement, l.MovementID)
	  }
	  return nil
  }

   func (m *MemStore) ListActiveLeases(limit int) []BreachLease {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 100
	   }
	  out := make([]BreachLease, 0, limit)
	   for _, l := range m.leases {
		  if !l.Active() {
			  continue
		  }
		   out = append(out, *l)
		   if len(out) >= limit {
			   break
		   }
	  }
	   return out
  }

  func (m *MemStore) ListLeasesByCase(caseID string, limit int) []BreachLease {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	  out := make([]BreachLease, 0)
	  for _, l := range m.leases {
		   if l.CaseID != caseID {
			  continue
		   }
		  out = append(out, *l)
		  if len(out) >= limit {
			  break
		   }
	   }
	  return out
  }

// ── SlaBreachProof ───────────────────────────────────────────────────────────

   func (m *MemStore) CreateProof(proof SlaBreachProof) error {
	   if proof.ID == "" {
		  proof.ID = uuid.NewString()
	   }
	  key := proof.BreachLeaseID + ":" + proof.BlockOutcome
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, dup := m.proofByKey[key]; dup {
		  return nil // 幂等：已有则跳过
	   }
	  cp := proof
	  m.proofs[cp.ID] = &cp
	   m.proofByKey[key] = cp.ID
	   return nil
   }

  func (m *MemStore) ListProofsByLease(leaseID string, limit int) []SlaBreachProof {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	  out := make([]SlaBreachProof, 0)
	  for _, p := range m.proofs {
		   if p.BreachLeaseID != leaseID {
			  continue
		   }
		   out = append(out, *p)
		   if len(out) >= limit {
			  break
		  }
	  }
	   return out
  }

   func (m *MemStore) ListAllProofs(limit int) []SlaBreachProof {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 200
	   }
	  out := make([]SlaBreachProof, 0, limit)
	   for _, p := range m.proofs {
		  out = append(out, *p)
		   if len(out) >= limit {
			  break
		   }
	  }
	   return out
   }

// ── BreachClearance ──────────────────────────────────────────────────────────

  func (m *MemStore) CreateClearance(c BreachClearance) error {
	   if c.ID == "" {
		   c.ID = uuid.NewString()
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if existID, dup := m.pendingClr[c.BreachLeaseID]; dup {
		  if ec, ok := m.clears[existID]; ok && ec.State == ClearanceStatePending {
			   return ErrDuplicateBreach // 已有 pending 工单
		  }
	   }
	   cp := c
	  m.clears[cp.ID] = &cp
	   if cp.State == ClearanceStatePending {
		  m.pendingClr[cp.BreachLeaseID] = cp.ID
	   }
	   return nil
   }

   func (m *MemStore) GetClearanceByID(id string) (*BreachClearance, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.clears[id]
	   if !ok {
		  return nil, ErrClearanceNotFound
	   }
	   cp := *c
	  return &cp, nil
  }

   func (m *MemStore) GetPendingClearanceByLease(leaseID string) (*BreachClearance, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.pendingClr[leaseID]
	   if !ok {
		  return nil, ErrClearanceNotFound
	  }
	  c, ok := m.clears[id]
	   if !ok || c.State != ClearanceStatePending {
		   delete(m.pendingClr, leaseID)
		   return nil, ErrClearanceNotFound
	   }
	  cp := *c
	   return &cp, nil
  }

   func (m *MemStore) UpdateClearanceState(id string, update func(*BreachClearance)) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.clears[id]
	  if !ok {
		  return ErrClearanceNotFound
	  }
	  if update != nil {
		   update(c)
	   }
	  if c.State != ClearanceStatePending {
		   delete(m.pendingClr, c.BreachLeaseID)
	  }
	  return nil
  }

   func (m *MemStore) ListClearances(limit int) []BreachClearance {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 100
	  }
	  out := make([]BreachClearance, 0, limit)
	   for _, c := range m.clears {
		   out = append(out, *c)
		  if len(out) >= limit {
			  break
		  }
	  }
	  return out
   }
