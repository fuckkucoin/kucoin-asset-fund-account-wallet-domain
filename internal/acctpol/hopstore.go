package acctpol

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store acctpol 完整持久化接口。
type HopStore interface {
	// HopPolicyVersion CRUD
	InsertVersion(ctx context.Context,
		v *HopPolicyVersion) error
	GetVersionByID(ctx context.Context, id string) (*HopPolicyVersion, error)
	GetVersionByGen(ctx context.Context, gen int64) (*HopPolicyVersion, error)
	GetActiveGlobalVersion(ctx context.Context, asset string) (*HopPolicyVersion, error)
	GetPinnedVersionByUser(ctx context.Context, userID, asset string) (*HopPolicyVersion, error)
	UpdateVersion(ctx context.Context,
		v *HopPolicyVersion) error
	ListVersionsByState(ctx context.Context, state PublishState, limit int) ([]*HopPolicyVersion, error)
	// Gen 世代序列
	NextGen(ctx context.Context) (int64, error)
	CurrentMaxGen(ctx context.Context) (int64, error)
	// HopDualReadLog 只追加
	AppendDualReadLog(ctx context.Context,
		l *HopDualReadLog) error
	ListDualReadLogs(ctx context.Context, bizKind HopBizKind, since time.Time, limit int) ([]*HopDualReadLog, error)
	CountDivergeLogs(ctx context.Context, bizKind HopBizKind, since time.Time) (int, error)
}

// ─── HopMemStore 内存实现 ─────────────────────────────────────────────────────
// HopMemStore 线程安全内存存储（供使用）。
type HopMemStore struct {
	mu sync.RWMutex
	versions map[string]*HopPolicyVersion // id → version
	versionGen map[int64]string // gen → id
	dualLogs   []*HopDualReadLog
	genCounter atomic.Int64
}

// NewHopMemStore 创建内存存储实例。
  func NewHopMemStore() *HopMemStore {
	   ms := &HopMemStore{
		   versions:   map[string]*HopPolicyVersion{},
		  versionGen: map[int64]string{},
		   dualLogs:   []*HopDualReadLog{},
	   }
	  ms.genCounter.Store(0)
	   return ms
  }

// InsertVersion 插入新策略代次（主键冲突返回 ErrDuplicate）。

  func (m *HopMemStore) InsertVersion(_ context.Context, v *HopPolicyVersion) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.versions[v.ID]; ok {
		   return fmt.Errorf("%w: version id=%s", ErrDuplicate, v.ID)
	   }
	   cp := *v
	  m.versions[v.ID] = &cp
	  m.versionGen[v.Gen] = v.ID
	  return nil
   }

// GetVersionByID 按 ID 查询策略代次。
  func (m *HopMemStore) GetVersionByID(_ context.Context, id string) (*HopPolicyVersion, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   v, ok := m.versions[id]
	  if !ok {
		   return nil, fmt.Errorf("%w: id=%s", ErrNotFound, id)
	  }
	  cp := *v
	  return &cp, nil
   }

// GetVersionByGen 按世代号查询策略代次。
   func (m *HopMemStore) GetVersionByGen(_ context.Context, gen int64) (*HopPolicyVersion, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.versionGen[gen]
	   if !ok {
		   return nil, fmt.Errorf("%w: gen=%d", ErrNotFound, gen)
	   }
	  v := m.versions[id]
	   cp := *v
	   return &cp, nil
  }

// GetActiveGlobalVersion 获取当前生效的全局代次（state=active，userID 为空）。
   func (m *HopMemStore) GetActiveGlobalVersion(_ context.Context, asset string) (*HopPolicyVersion, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var best *HopPolicyVersion
	   for _, v := range m.versions {
		   if v.UserID != "" {
			   continue
		  }
		   if v.State != PublishStateActive {
			   continue
		   }
		  if asset != "" && v.Asset != "" && v.Asset != asset {
			   continue
		   }
		  if best == nil || v.Gen > best.Gen {
			   best = v
		   }
	  }
	   if best == nil {
		  return nil, fmt.Errorf("%w: no active global version for asset=%s", ErrNotFound, asset)
	  }
	   cp := *best
	  return &cp, nil
  }

// GetPinnedVersionByUser 获取指定用户的钉版（state=pinned）。
  func (m *HopMemStore) GetPinnedVersionByUser(_ context.Context, userID, asset string) (*HopPolicyVersion, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var best *HopPolicyVersion
	   for _, v := range m.versions {
		  if v.UserID != userID {
			   continue
		   }
		  if v.State != PublishStatePinned {
			   continue
		  }
		  if asset != "" && v.Asset != "" && v.Asset != asset {
			  continue
		   }
		  if best == nil || v.Gen > best.Gen {
			   best = v
		   }
	  }
	  if best == nil {
		  return nil, fmt.Errorf("%w: no pinned version for user=%s asset=%s", ErrNotFound, userID, asset)
	   }
	  cp := *best
	   return &cp, nil
  }

// UpdateVersion 更新策略代次（必须存在）。

  func (m *HopMemStore) UpdateVersion(_ context.Context, v *HopPolicyVersion) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.versions[v.ID]; !ok {
		   return fmt.Errorf("%w: id=%s", ErrNotFound, v.ID)
	   }
	  cp := *v
	   m.versions[v.ID] = &cp
	   m.versionGen[v.Gen] = v.ID
	   return nil
   }

// ListVersionsByState 按状态列出策略代次（按 Gen 降序）。
   func (m *HopMemStore) ListVersionsByState(_ context.Context, state PublishState, limit int) ([]*HopPolicyVersion, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*HopPolicyVersion
	   for _, v := range m.versions {
		   if v.State == state {
			   cp := *v
			  out = append(out, &cp)
		  }
	  }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].Gen > out[j].Gen })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

// NextGen 原子递增并返回新世代号。

   func (m *HopMemStore) NextGen(_ context.Context) (int64, error) {
	   return m.genCounter.Add(1), nil
  }

// CurrentMaxGen 返回当前最大世代号。

  func (m *HopMemStore) CurrentMaxGen(_ context.Context) (int64, error) {
	  return m.genCounter.Load(), nil
   }

// AppendDualReadLog 追加双读观测日志。

  func (m *HopMemStore) AppendDualReadLog(_ context.Context, l *HopDualReadLog) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := *l
	  m.dualLogs = append(m.dualLogs, &cp)
	   return nil
  }

// ListDualReadLogs 按 bizKind 和时间范围列出双读日志。

   func (m *HopMemStore) ListDualReadLogs(_ context.Context, bizKind HopBizKind, since time.Time, limit int) ([]*HopDualReadLog, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*HopDualReadLog
	   for _, l := range m.dualLogs {
		   if l.BizKind != bizKind && bizKind != "" {
			   continue
		  }
		  if !l.ObservedAt.Before(since) || since.IsZero() {
			  cp := *l
			   out = append(out, &cp)
		   }
	   }
	  // 按观测时间降序
	  sort.Slice(out, func(i, j int) bool { return out[i].ObservedAt.After(out[j].ObservedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }

// CountDivergeLogs 统计指定时间窗口内的分歧日志数量。
  func (m *HopMemStore) CountDivergeLogs(_ context.Context, bizKind HopBizKind, since time.Time) (int, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   n := 0
	   for _, l := range m.dualLogs {
		   if l.Result != DualReadDiverge {
			  continue
		  }
		  if bizKind != "" && l.BizKind != bizKind {
			  continue
		   }
		   if !since.IsZero() && l.ObservedAt.Before(since) {
			  continue
		  }
		  n++
	  }
	   return n, nil
  }
