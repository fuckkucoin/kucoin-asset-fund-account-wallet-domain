package cfgdual

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// RollbackManager 回滚证明管理器
// ─────────────────────────────────────────────────────────────────────────────
// RollbackManager 协调回滚操作：更新发布状态、生成证明、清理注册表。
type RollbackManager struct {
	mu      sync.Mutex
	store   Store // 持久化接口（Store 定义在 store.go）
	cohorts *CohortRegistry
	seals   *SealRegistry
	engine  *Engine
	idgen   func() string
	clock func() time.Time
	// proofs key=releaseID；进程内缓存，防止重复回滚。
	proofs map[string]*RollbackProof
}

// NewRollbackManager 构造回滚证明管理器。

  func NewRollbackManager(
	   store Store,
	  cohorts *CohortRegistry,
	   seals *SealRegistry,
	  engine *Engine,
	  idgen func() string,
) *RollbackManager {
	return &RollbackManager{
		store: store,
		cohorts: cohorts,
		seals:   seals,
		engine:  engine,
		idgen: idgen,
		clock: func() time.Time { return time.Now().UTC() },
		proofs: map[string]*RollbackProof{},
	}
}

// WithClock 注入时钟（用）。

   func (m *RollbackManager) WithClock(c func() time.Time) *RollbackManager {
	  if c != nil {
		  m.clock = c
	   }
	   return m
  }

  // Rollback 执行回滚并生成证明。
  // 约束：
  // - release.State 必须为 StateCanary，否则返回 ErrInvalidState；
   // - 同一 releaseID 只能回滚一次，否则返回 ErrRollbackProofExists；
  // - 回滚后世代号自增（generation+1），阻止旧世代评估结论被复用。

   func (m *RollbackManager) Rollback(
	  ctx context.Context,
	  release *GrayRelease,
	   triggerSplitDiffID,
	   actor, reason string,
  ) (*RollbackProof, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   if release == nil {
		   return nil, fmt.Errorf("%w: release required", ErrConflict)
	  }
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		  actor = "system"
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  // 禁止重复回滚。
	  if _, exists := m.proofs[release.ID]; exists {
		   return nil, fmt.Errorf("%w: release_id=%s", ErrRollbackProofExists, release.ID)
	   }
	   // 状态检查：只有 canary 中才能回滚。
	   if release.State != StateCanary {
		  return nil, fmt.Errorf("%w: release %s is in state %s, rollback requires canary",
			  ErrInvalidState, release.ID, release.State)
	  }
	  now := m.clock()
	  genBefore := release.Generation
	  genAfter := genBefore + 1
	  // 构造证明记录。
	  proof := &RollbackProof{
		  ID:                m.idgen(),
		   GrayReleaseID:     release.ID,
		  PackName:          release.PackName,
		   RolledBackVersion: release.CanaryVersion,
		  RestoredVersion: release.BaselineVersion,
		  GenerationBefore:   genBefore,
		  GenerationAfter:    genAfter,
		  TriggerSplitDiffID: triggerSplitDiffID,
		  Actor:              actor,
		   Reason: reason,
		   CreatedAt: now,
	  }
	   // 更新发布状态。
	  release.State = StateRolledBack
	   release.Generation = genAfter
	   release.RolledBackAt = &now
	   release.UpdatedAt = now
	   // 持久化：发布状态 + 证明记录。
	   if m.store != nil {
		  if err := m.store.SaveRelease(ctx, release); err != nil {
			   // 持久化失败回滚内存状态。
			  release.State = StateCanary
			  release.Generation = genBefore
			   release.RolledBackAt = nil
			   release.UpdatedAt = release.CreatedAt
			  return nil, fmt.Errorf("rollback persist release: %w", err)
		  }
		   if err := m.store.SaveRollbackProof(ctx, proof); err != nil {
			  return nil, fmt.Errorf("rollback persist proof: %w", err)
		   }
	  }
	   // 缓存证明。
	  m.proofs[release.ID] = proof
	   // 从进程级注册表摘除（DualEval 自动降级 baseline-only）。
	   if m.cohorts != nil {
		  m.cohorts.Remove(release.ID)
	   }
	   if m.seals != nil {
		  m.seals.Remove(release.ID)
	   }
	  if m.engine != nil {
		   m.engine.UnregisterRelease(release.ID)
	  }
	   return proof, nil
  }

// GetProof 按 releaseID 查询回滚证明（进程内缓存先，否则查 store）。
   func (m *RollbackManager) GetProof(ctx context.Context, releaseID string) (*RollbackProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	  if p, ok := m.proofs[releaseID]; ok {
		  m.mu.Unlock()
		  return p, nil
	  }
	   m.mu.Unlock()
	   if m.store == nil {
		   return nil, ErrNotFound
	  }
	   p, err := m.store.GetRollbackProof(ctx, releaseID)
	   if err != nil {
		  return nil, err
	  }
	   m.mu.Lock()
	  m.proofs[releaseID] = p
	  m.mu.Unlock()
	   return p, nil
   }
