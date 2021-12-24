package tradepwgen

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
	"time"
)

// ──────────────────────────────────────────────
// 存储接口
// ──────────────────────────────────────────────
// GenerationStore 密码世代持久化接口。

type GenerationStore interface {
	// Get 按 user_id 读取当前世代；不存在时返回 Generation=0。
	Get(userID string) (*PwGeneration, error)
	// BumpGeneration CAS 抬升世代。
	// expectedGen=0 表示首次写入（用户尚无世代记录）。
	// expectedGen!=当前 generation 时返回 ErrGenConflict。
	BumpGeneration(userID string, expectedGen, newGen int64, reason string, now time.Time) (*PwGeneration, error)
	// SetCoolBound 标记当前世代冷却已生效。
	SetCoolBound(userID string, gen int64, now time.Time) error
	// SetActiveLeaseID 更新活跃核验令牌 ID（可置空）。
	SetActiveLeaseID(userID, leaseID string, now time.Time) error
	// HasOpenDrift 检查用户是否存在未关闭漂移案件。
	HasOpenDrift(userID string) (bool,
		error)
}

// LeaseStore 核验令牌持久化接口。
type LeaseStore interface {
	// Insert 插入新令牌；ID 重复时返回错误。
	Insert(l *VerifyLease) error
	// Get 按 ID 查询；不存在时返回 ErrLeaseNotFound。
	Get(leaseID string) (*VerifyLease, error)
	// GetByToken 按令牌字符串查询（Token 字段内存匹配；SQL 实现存 hash）。
	GetByToken(token string) (*VerifyLease, error)
	// Update 全量更新令牌（仅允许状态/时间字段变更）。
	Update(l *VerifyLease) error
	// ListOpenByUser 列出指定用户全部 open 令牌（Worker 扫描入口）。
	ListOpenByUser(userID string, limit int) ([]*VerifyLease, error)
	// RevokeByUser 原子吊销 gen 之前（PwGen < newGen）该用户全部 open 令牌，返回吊销数量。
	RevokeByUser(userID string, newGen int64, now time.Time) (int, error)
	// CountOpen 统计用户 open 令牌数（用于互斥检查）。
	CountOpen(userID string) (int, error)
}

// DriftStore 漂移案件持久化接口。

type DriftStore interface {
	// Insert 插入新案件；若同 (user_id, lease_id) 已有 open 案件则返回已存在案件（幂等）。
	Insert(dc *PwDriftCase) (*PwDriftCase, error)
	// Get 按 ID 查询；不存在时返回 nil, nil。
	Get(caseID string) (*PwDriftCase, error)
	// Update 全量更新案件（仅允许状态/证明字段变更）。
	Update(dc *PwDriftCase) error
	// ListOpen 列出全部 open/proven 案件（Worker 巡检入口）。
	ListOpen(limit int) ([]*PwDriftCase, error)
	// ListOpenByUser 列出指定用户未关闭案件。
	ListOpenByUser(userID string, limit int) ([]*PwDriftCase, error)
	// HasOpen 检查指定用户是否存在 open/proven 案件。
	HasOpen(userID string) (bool, error)
}

// ──────────────────────────────────────────────
// 内存实现
// ──────────────────────────────────────────────
// MemGenerationStore 内存世代存储（专用）。
type MemGenerationStore struct {
	mu   sync.RWMutex
	gens map[string]*PwGeneration
}

// NewMemGenerationStore 创建内存世代存储。

  func NewMemGenerationStore() *MemGenerationStore {
	   return &MemGenerationStore{gens: make(map[string]*PwGeneration)}
  }

   func (s *MemGenerationStore) Get(userID string) (*PwGeneration, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  g, ok := s.gens[userID]
	  if !ok {
		  return &PwGeneration{UserID: userID, Generation: 0, Status: GenStatusActive}, nil
	   }
	   cp := *g
	  return &cp, nil
  }

  func (s *MemGenerationStore) BumpGeneration(userID string, expectedGen, newGen int64, reason string, now time.Time) (*PwGeneration, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   g, ok := s.gens[userID]
	   if !ok {
		  if expectedGen != 0 {
			   return nil, ErrGenConflict
		   }
		  ng := &PwGeneration{
			  UserID:     userID,
			  Generation: newGen,
			  Status:    GenStatusActive,
			   UpdatedAt: now,
			  CreatedAt: now,
		  }
		  s.gens[userID] = ng
		  cp := *ng
		  return &cp, nil
	   }
	  if g.Generation != expectedGen {
		   return nil, ErrGenConflict
	  }
	  g.Generation = newGen
	   g.Status = GenStatusActive
	  g.CoolBound = false
	   g.ActiveLeaseID = ""
	   g.UpdatedAt = now
	   cp := *g
	   return &cp, nil
   }

   func (s *MemGenerationStore) SetCoolBound(userID string, gen int64, now time.Time) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  g, ok := s.gens[userID]
	   if !ok {
		   return ErrStore
	   }
	  if g.Generation != gen {
		   return ErrGenConflict
	   }
	  g.CoolBound = true
	  g.UpdatedAt = now
	   return nil
   }

   func (s *MemGenerationStore) SetActiveLeaseID(userID, leaseID string, now time.Time) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  g, ok := s.gens[userID]
	   if !ok {
		   return ErrStore
	  }
	   g.ActiveLeaseID = leaseID
	   g.UpdatedAt = now
	   return nil
  }

  func (s *MemGenerationStore) HasOpenDrift(userID string) (bool, error) {
	   // 内存世代存储自身不存漂移案件；由 Service 层跨 DriftStore 联查。
	  return false, nil
   }

// MemLeaseStore 内存核验令牌存储（专用）。
type MemLeaseStore struct {
	mu sync.RWMutex
	byID  map[string]*VerifyLease
	byTok map[string]*VerifyLease
}

// NewMemLeaseStore 创建内存核验令牌存储。

   func NewMemLeaseStore() *MemLeaseStore {
	   return &MemLeaseStore{
		   byID:  make(map[string]*VerifyLease),
		   byTok: make(map[string]*VerifyLease),
	   }
  }

  func (s *MemLeaseStore) Insert(l *VerifyLease) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.byID[l.ID]; ok {
		  return ErrLeaseConsumed
	   }
	  cp := *l
	  s.byID[l.ID] = &cp
	   s.byTok[l.Token] = &cp
	   return nil
   }

   func (s *MemLeaseStore) Get(leaseID string) (*VerifyLease, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   l, ok := s.byID[leaseID]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  cp := *l
	   return &cp, nil
   }

  func (s *MemLeaseStore) GetByToken(token string) (*VerifyLease, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  l, ok := s.byTok[token]
	  if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  cp := *l
	   return &cp, nil
  }

   func (s *MemLeaseStore) Update(l *VerifyLease) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  old, ok := s.byID[l.ID]
	   if !ok {
		  return ErrLeaseNotFound
	  }
	   cp := *l
	  s.byID[l.ID] = &cp
	  // 同步 token 索引（token 不会变更，但需更新指针）。
	   if old.Token != "" {
		  s.byTok[old.Token] = &cp
	   }
	   return nil
   }

  func (s *MemLeaseStore) ListOpenByUser(userID string, limit int) ([]*VerifyLease, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []*VerifyLease
	  for _, l := range s.byID {
		   if l.UserID == userID && l.Status == LeaseStatusOpen {
			  cp := *l
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	   }
	   return out, nil
   }

  func (s *MemLeaseStore) RevokeByUser(userID string, newGen int64, now time.Time) (int, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   count := 0
	   for _, l := range s.byID {
		   if l.UserID == userID && l.Status == LeaseStatusOpen && l.PwGen < newGen {
			  l.Status = LeaseStatusRevoked
			   l.RevokedAt = &now
			  l.UpdatedAt = now
			  // 同步 token 索引。
			  if l.Token != "" {
				  s.byTok[l.Token] = l
			   }
			   count++
		   }
	  }
	   return count, nil
  }

   func (s *MemLeaseStore) CountOpen(userID string) (int, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   count := 0
	  for _, l := range s.byID {
		  if l.UserID == userID && l.Status == LeaseStatusOpen {
			   count++
		  }
	  }
	  return count, nil
   }

// MemDriftStore 内存漂移案件存储（专用）。
type MemDriftStore struct {
	mu sync.RWMutex
	byID map[string]*PwDriftCase
	// openByUser 维护用户 open/proven 案件 ID 集合（加速 HasOpen 查询）。
	openByUser map[string]map[string]struct{}
}

// NewMemDriftStore 创建内存漂移案件存储。
  func NewMemDriftStore() *MemDriftStore {
	  return &MemDriftStore{
		  byID: make(map[string]*PwDriftCase),
		   openByUser: make(map[string]map[string]struct{}),
	   }
  }

   func (s *MemDriftStore) Insert(dc *PwDriftCase) (*PwDriftCase, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  // 幂等：同 (user_id, lease_id) 已有 open 案件，返回已存在案件。
	  for _, existing := range s.byID {
		   if existing.UserID == dc.UserID && existing.LeaseID == dc.LeaseID && existing.Status == DriftStatusOpen {
			   cp := *existing
			   return &cp, nil
		  }
	   }
	   cp := *dc
	   s.byID[dc.ID] = &cp
	  if s.openByUser[dc.UserID] == nil {
		   s.openByUser[dc.UserID] = make(map[string]struct{})
	  }
	   s.openByUser[dc.UserID][dc.ID] = struct{}{}
	   return &cp, nil
  }

  func (s *MemDriftStore) Get(caseID string) (*PwDriftCase, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  dc, ok := s.byID[caseID]
	   if !ok {
		  return nil, nil
	   }
	  cp := *dc
	  return &cp, nil
   }

  func (s *MemDriftStore) Update(dc *PwDriftCase) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   _, ok := s.byID[dc.ID]
	  if !ok {
		  return ErrStore
	   }
	   cp := *dc
	  s.byID[dc.ID] = &cp
	  // 维护 openByUser 集合。
	   if dc.Status == DriftStatusClosed {
		   if set, ok2 := s.openByUser[dc.UserID]; ok2 {
			  delete(set, dc.ID)
		  }
	  }
	   return nil
  }

  func (s *MemDriftStore) ListOpen(limit int) ([]*PwDriftCase, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []*PwDriftCase
	  for _, dc := range s.byID {
		   if dc.Status == DriftStatusOpen || dc.Status == DriftStatusProven {
			  cp := *dc
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	  }
	  return out, nil
  }

  func (s *MemDriftStore) ListOpenByUser(userID string, limit int) ([]*PwDriftCase, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []*PwDriftCase
	   for _, dc := range s.byID {
		  if dc.UserID == userID && (dc.Status == DriftStatusOpen || dc.Status == DriftStatusProven) {
			  cp := *dc
			   out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				  break
			  }
		   }
	   }
	  return out, nil
   }

   func (s *MemDriftStore) HasOpen(userID string) (bool, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  set, ok := s.openByUser[userID]
	  if !ok {
		   return false, nil
	  }
	  return len(set) > 0, nil
  }

// ──────────────────────────────────────────────
// 工具函数
// ──────────────────────────────────────────────
// newToken 生成 32 字节随机令牌（hex 编码，64 字符）。

  func newToken() (string, error) {
	  var b [32]byte
	  if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		  return "", err
	   }
	  return hex.EncodeToString(b[:]), nil
  }

// newID 生成 UUID-like 的 ID（16 字节随机值，hex 编码）。
  func newID() (string, error) {
	  var b [16]byte
	  if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		   return "", err
	   }
	   return hex.EncodeToString(b[:]), nil
  }
