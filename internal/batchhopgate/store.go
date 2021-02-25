package batchhopgate

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// Store 接口
// ─────────────────────────────────────────────────────────────────────────────
// Store 是 batchhopgate 所有实体的持久化抽象。所有实现必须保证并发安全。
type Store interface {
	// ── BatchResidualCase ────────────────────────────────────────────────────
	// SaveResidual 创建或按 ID 更新 BatchResidualCase。
	SaveResidual(ctx context.Context, c *BatchResidualCase) error
	// GetResidual 按 case_id 查询。未找到返回 ErrResidualNotFound。
	GetResidual(ctx context.Context, caseID string) (*BatchResidualCase, error)
	// FindOpenResidual 按 user_id + asset 查找非终态残留案（open/proven/resuming）。
	// 不存在时返回 nil, nil。
	FindOpenResidual(ctx context.Context,
		userID, asset string) (*BatchResidualCase, error)
	// ListResidualsByUser 返回用户所有残留案列表（含终态），按 detected_at 降序，limit 限制条数。
	ListResidualsByUser(ctx context.Context, userID string, limit int) ([]*BatchResidualCase, error)
	// ListResidualsByStatus 按状态批量查询（供 worker 扫描），按 detected_at 升序。
	ListResidualsByStatus(ctx context.Context,
		status ResidualStatus, limit int) ([]*BatchResidualCase, error)
	// ── ResumeLease ──────────────────────────────────────────────────────────
	// SaveLease 创建或按 ID 更新 ResumeLease。
	SaveLease(ctx context.Context,
		l *ResumeLease) error
	// GetLease 按 lease_id 查询。未找到返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*ResumeLease, error)
	// FindActiveLease 按 residual_case_id 查找处于 open 或 bound 的活跃租约。
	// 不存在时返回 nil, nil。
	FindActiveLease(ctx context.Context, residualCaseID string) (*ResumeLease, error)
	// ListExpiredOpenLeases 返回所有 ExpiresAt < before 且状态为 open 的租约（供 worker 标记 expired）。
	ListExpiredOpenLeases(ctx context.Context, before time.Time, limit int) ([]*ResumeLease, error)
	// ── HopOccupyProof ────────────────────────────────────────────────────────
	// SaveProof 创建或按 ID 更新 HopOccupyProof。
	SaveProof(ctx context.Context, p *HopOccupyProof) error
	// GetProof 按 proof_id 查询。未找到返回 ErrProofNotFound。
	GetProof(ctx context.Context, proofID string) (*HopOccupyProof, error)
	// GetProofByCase 按 residual_case_id 查找关联证明（幂等查找）。
	// 不存在时返回 nil, nil。
	GetProofByCase(ctx context.Context, residualCaseID string) (*HopOccupyProof, error)
	// ── BatchGeneration ──────────────────────────────────────────────────────
	// GetGeneration 返回用户资产当前 BatchGeneration 记录。首次查询若不存在，自动初始化（generation=0, status=idle）。
	GetGeneration(ctx context.Context, userID, asset string) (*BatchGeneration, error)
	// BumpGeneration CAS 递增世代（idle → active）：
	// 若 status='active' → ErrConcurrentBatch（已有并行 BatchHop）；
	// 若 generation != expected → ErrStaleGeneration；
	// 否则原子递增：generation = expected+1, status='active'。
	// 返回新世代号。
	BumpGeneration(ctx context.Context, userID, asset string, expected int64) (int64, error)
	// ReleaseGeneration 将世代状态切换回 idle（BatchHop 完成或失败收口后调用）。
	// 若 generation 不符则忽略（幂等）。
	ReleaseGeneration(ctx context.Context, userID, asset string, generation int64) error
}

// ─────────────────────────────────────────────────────────────────────────────
// MemStore — 线程安全内存实现
// ─────────────────────────────────────────────────────────────────────────────
// MemStore 是 Store 的内存实现，适用于单元和无数据库场景。

type MemStore struct {
	mu        sync.Mutex
	residuals map[string]*BatchResidualCase
	leases      map[string]*ResumeLease
	proofs      map[string]*HopOccupyProof
	proofByCase map[string]*HopOccupyProof
	gens map[string]*BatchGeneration
}

// NewMemStore 构造内存存储。
  func NewMemStore() *MemStore {
	   return &MemStore{
		   residuals: make(map[string]*BatchResidualCase),
		   leases:    make(map[string]*ResumeLease),
		   proofs:    make(map[string]*HopOccupyProof),
		  proofByCase: make(map[string]*HopOccupyProof),
		   gens:        make(map[string]*BatchGeneration),
	  }
  }

// genKey 用户资产复合键。

   func genKey(userID, asset string) string {
	   return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }

// ── BatchResidualCase ─────────────────────────────────────────────────────────

  func (m *MemStore) SaveResidual(ctx context.Context, c *BatchResidualCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := *c
	  m.residuals[cp.ID] = &cp
	  return nil
  }

   func (m *MemStore) GetResidual(ctx context.Context, caseID string) (*BatchResidualCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.residuals[caseID]
	  if !ok {
		  return nil, ErrResidualNotFound
	   }
	  cp := *c
	  return &cp, nil
   }

  func (m *MemStore) FindOpenResidual(ctx context.Context, userID, asset string) (*BatchResidualCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, c := range m.residuals {
		  if c.UserID != userID || !strings.EqualFold(c.Asset, asset) {
			   continue
		  }
		   switch c.Status {
		   case ResidualOpen, ResidualProven, ResidualResuming:
			   cp := *c
			   return &cp, nil
		  }
	  }
	   return nil, nil
   }

  func (m *MemStore) ListResidualsByUser(ctx context.Context, userID string, limit int) ([]*BatchResidualCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]*BatchResidualCase, 0)
	  for _, c := range m.residuals {
		   if c.UserID != userID {
			   continue
		   }
		  cp := *c
		  out = append(out, &cp)
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
  }

   func (m *MemStore) ListResidualsByStatus(ctx context.Context, status ResidualStatus, limit int) ([]*BatchResidualCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]*BatchResidualCase, 0)
	  for _, c := range m.residuals {
		  if c.Status != status {
			   continue
		  }
		   cp := *c
		   out = append(out, &cp)
		   if limit > 0 && len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
  }

// ── ResumeLease ───────────────────────────────────────────────────────────────

   func (m *MemStore) SaveLease(ctx context.Context, l *ResumeLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *l
	   m.leases[cp.ID] = &cp
	  return nil
  }

  func (m *MemStore) GetLease(ctx context.Context, leaseID string) (*ResumeLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[leaseID]
	  if !ok {
		   return nil, ErrLeaseNotFound
	   }
	   cp := *l
	   return &cp, nil
   }

   func (m *MemStore) FindActiveLease(ctx context.Context, residualCaseID string) (*ResumeLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, l := range m.leases {
		  if l.ResidualCaseID != residualCaseID {
			   continue
		   }
		   if l.Status == LeaseOpen || l.Status == LeaseBound {
			  cp := *l
			  return &cp, nil
		   }
	  }
	   return nil, nil
   }

  func (m *MemStore) ListExpiredOpenLeases(ctx context.Context, before time.Time, limit int) ([]*ResumeLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]*ResumeLease, 0)
	   for _, l := range m.leases {
		  if l.Status == LeaseOpen && l.ExpiresAt.Before(before) {
			  cp := *l
			  out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	   }
	   return out, nil
   }

// ── HopOccupyProof ────────────────────────────────────────────────────────────

   func (m *MemStore) SaveProof(ctx context.Context, p *HopOccupyProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := *p
	   m.proofs[cp.ID] = &cp
	   m.proofByCase[cp.ResidualCaseID] = &cp
	   return nil
   }

  func (m *MemStore) GetProof(ctx context.Context, proofID string) (*HopOccupyProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		  return nil, ErrProofNotFound
	  }
	  cp := *p
	   return &cp, nil
   }

  func (m *MemStore) GetProofByCase(ctx context.Context, residualCaseID string) (*HopOccupyProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofByCase[residualCaseID]
	   if !ok {
		   return nil, ErrProofNotFound
	  }
	   cp := *p
	  return &cp, nil
   }

// ── BatchGeneration ───────────────────────────────────────────────────────────

   func (m *MemStore) GetGeneration(ctx context.Context, userID, asset string) (*BatchGeneration, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  k := genKey(userID, asset)
	   g, ok := m.gens[k]
	   if !ok {
		  g = &BatchGeneration{
			  ID:         uuid.New().String(),
			   UserID:     userID,
			  Asset:      strings.ToUpper(strings.TrimSpace(asset)),
			  Generation: 0,
			  Status: "idle",
			   UpdatedAt: time.Now().UTC(),
		   }
		  m.gens[k] = g
	  }
	  cp := *g
	   return &cp, nil
  }

   func (m *MemStore) BumpGeneration(ctx context.Context, userID, asset string, expected int64) (int64, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   k := genKey(userID, asset)
	   g, ok := m.gens[k]
	   if !ok {
		  if expected != 0 {
			   return 0, ErrStaleGeneration
		   }
		   g = &BatchGeneration{
			  ID: uuid.New().String(),
			   UserID: userID,
			  Asset:  strings.ToUpper(strings.TrimSpace(asset)),
			   Generation: 0,
			  Status:    "idle",
			   UpdatedAt: time.Now().UTC(),
		   }
		  m.gens[k] = g
	  }
	  if g.Status == "active" {
		  return 0, ErrConcurrentBatch
	   }
	  if g.Generation != expected {
		   return 0, ErrStaleGeneration
	   }
	  g.Generation = expected + 1
	  g.Status = "active"
	   g.UpdatedAt = time.Now().UTC()
	   return g.Generation, nil
   }

   func (m *MemStore) ReleaseGeneration(ctx context.Context, userID, asset string, generation int64) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  k := genKey(userID, asset)
	   g, ok := m.gens[k]
	   if !ok {
		   return nil
	  }
	  if g.Generation != generation {
		  return nil
	  }
	   g.Status = "idle"
	   g.UpdatedAt = time.Now().UTC()
	  return nil
  }
