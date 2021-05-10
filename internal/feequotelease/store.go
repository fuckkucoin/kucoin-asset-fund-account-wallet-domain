package feequotelease

import (
	"context"
	"sync"
	"time"
)

// Store 租约存储接口。

type Store interface {
	// SaveLease 新建或更新租约（幂等：按 ID 覆盖）。
	SaveLease(ctx context.Context, l FeeQuoteLease) error
	// GetLease 按 ID 查询租约。
	GetLease(ctx context.Context, id string) (FeeQuoteLease, error)
	// GetLeaseByIdempotencyKey 按幂等键查询已存在租约。
	GetLeaseByIdempotencyKey(ctx context.Context, key string) (FeeQuoteLease, bool, error)
	// ListActiveLeasesByKindAsset 列出指定 kind/asset 的活跃（quoted）租约，用于吊销。
	ListActiveLeasesByKindAsset(ctx context.Context, kind, asset string) ([]FeeQuoteLease, error)
	// ListExpiredLeases 列出已超 ExpiresAt 但仍为 quoted 状态的租约（Worker 调用）。
	ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]FeeQuoteLease, error)
	// SaveGen 新建 ScheduleGeneration 记录。
	SaveGen(ctx context.Context, g ScheduleGeneration) error
	// GetCurrentGen 查询指定 kind/asset 的当前最高 gen。
	GetCurrentGen(ctx context.Context, kind, asset string) (ScheduleGeneration, error)
	// SaveProof 追加漂移事故证明（只追加）。
	SaveProof(ctx context.Context, p FeeQuoteDriftProof) error
	// ListProofs 列出未导出证明（Worker/Ops 调用）。
	ListProofs(ctx context.Context,
		limit int) ([]FeeQuoteDriftProof, error)
	// MarkProofExported 标记证明已导出。
	MarkProofExported(ctx context.Context, id string, exportedAt time.Time) error
}

// ─────────────────────────────────────────────────────────────────────────────
// InMemStore（用）
// ─────────────────────────────────────────────────────────────────────────────
// InMemStore 基于内存的 Store 实现，线程安全，用于单元。

type InMemStore struct {
	mu     sync.Mutex
	leases map[string]FeeQuoteLease
	idem map[string]string // idempotency_key → lease_id
	gens   map[string]ScheduleGeneration
	proofs map[string]FeeQuoteDriftProof
}

// NewInMemStore 创建内存存储实例。

   func NewInMemStore() *InMemStore {
	  return &InMemStore{
		  leases: map[string]FeeQuoteLease{},
		   idem:   map[string]string{},
		  gens: map[string]ScheduleGeneration{},
		   proofs: map[string]FeeQuoteDriftProof{},
	   }
   }

   func (s *InMemStore) SaveLease(_ context.Context, l FeeQuoteLease) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  s.leases[l.ID] = l
	   if l.IdempotencyKey != "" {
		  s.idem[l.IdempotencyKey] = l.ID
	  }
	  return nil
  }

  func (s *InMemStore) GetLease(_ context.Context, id string) (FeeQuoteLease, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   l, ok := s.leases[id]
	   if !ok {
		  return FeeQuoteLease{}, ErrLeaseNotFound
	  }
	  return l, nil
  }

   func (s *InMemStore) GetLeaseByIdempotencyKey(_ context.Context, key string) (FeeQuoteLease, bool, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   id, ok := s.idem[key]
	   if !ok {
		   return FeeQuoteLease{}, false, nil
	  }
	  l, exists := s.leases[id]
	   if !exists {
		  return FeeQuoteLease{}, false, nil
	   }
	  return l, true, nil
  }

   func (s *InMemStore) ListActiveLeasesByKindAsset(_ context.Context, kind, asset string) ([]FeeQuoteLease, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []FeeQuoteLease
	  for _, l := range s.leases {
		  if l.Kind == kind && l.Asset == asset && l.State == LeaseStateQuoted {
			  out = append(out, l)
		   }
	  }
	   return out, nil
  }

   func (s *InMemStore) ListExpiredLeases(_ context.Context, now time.Time, limit int) ([]FeeQuoteLease, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []FeeQuoteLease
	   for _, l := range s.leases {
		   if l.State == LeaseStateQuoted && now.After(l.ExpiresAt) {
			  out = append(out, l)
			   if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	  }
	   return out, nil
  }

   func (s *InMemStore) SaveGen(_ context.Context, g ScheduleGeneration) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   key := g.Kind + "|" + g.Asset
	  existing, ok := s.gens[key]
	   if !ok || g.Gen >= existing.Gen {
		   s.gens[key] = g
	  }
	  return nil
   }

   func (s *InMemStore) GetCurrentGen(_ context.Context, kind, asset string) (ScheduleGeneration, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  g, ok := s.gens[kind+"|"+asset]
	  if !ok {
		   // wildcard fallback
		  g, ok = s.gens[kind+"|*"]
		   if !ok {
			  return ScheduleGeneration{Kind: kind, Asset: asset, Gen: 0}, ErrGenNotFound
		   }
	   }
	  return g, nil
   }

  func (s *InMemStore) SaveProof(_ context.Context, p FeeQuoteDriftProof) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   s.proofs[p.ID] = p
	  return nil
  }

  func (s *InMemStore) ListProofs(_ context.Context, limit int) ([]FeeQuoteDriftProof, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  var out []FeeQuoteDriftProof
	   for _, p := range s.proofs {
		  if p.ExportedAt == nil {
			   out = append(out, p)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
  }

  func (s *InMemStore) MarkProofExported(_ context.Context, id string, exportedAt time.Time) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   p, ok := s.proofs[id]
	  if !ok {
		  return ErrLeaseNotFound
	  }
	  p.ExportedAt = &exportedAt
	  s.proofs[id] = p
	   return nil
   }
