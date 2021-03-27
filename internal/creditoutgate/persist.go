package creditoutgate

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// LeaseWriter 投递租约写口。

type LeaseWriter interface {
	UpsertLease(ctx context.Context, l CreditDeliverLease) error
}

// LeaseReader 投递租约读口。

type LeaseReader interface {
	LeaseByKey(ctx context.Context, creditKey string) (CreditDeliverLease, bool, error)
	ActiveLeases(ctx context.Context, boardKey string) ([]CreditDeliverLease, error)
}

// CaseWriter 事故证明写口（只追加）。

type CaseWriter interface {
	InsertCase(ctx context.Context, c CreditFenceCase) error
}

// CaseReader 事故证明读口。
type CaseReader interface {
	CasesByKey(ctx context.Context, creditKey string) ([]CreditFenceCase, error)
	CasesByBoard(ctx context.Context, boardKey string) ([]CreditFenceCase, error)
	UnexportedCases(ctx context.Context, limit int) ([]CreditFenceCase, error)
}

// ProofWriter 投递证明写口。
type ProofWriter interface {
	InsertProof(ctx context.Context, p CreditProof) error
	MarkExported(ctx context.Context, proofID string, at time.Time) error
}

// ProofReader 投递证明读口。
type ProofReader interface {
	ProofByKey(ctx context.Context, creditKey string) (CreditProof, bool, error)
	UnexportedProofs(ctx context.Context, limit int) ([]CreditProof, error)
}

// GenWriter 板卡世代写口。
type GenWriter interface {
	UpsertGen(ctx context.Context,
		bg BoardGeneration) error
}

// GenReaderStore 板卡世代读口（与内存 GenReader 接口独立，支持跨进程查询）。

type GenReaderStore interface {
	GenByKey(ctx context.Context, boardKey string) (BoardGeneration, bool, error)
	AllGens(ctx context.Context) ([]BoardGeneration,
		error)
}

// ─────────────────────────────────────────────────────────────
// 内存实现（与单机模式）
// ─────────────────────────────────────────────────────────────
// MemRepo 内存仓储，实现全部读写接口（用途）。

type MemRepo struct {
	mu     sync.RWMutex
	leases map[string]CreditDeliverLease
	cases  map[string]CreditFenceCase
	proofs map[string]CreditProof
	gens map[string]BoardGeneration
}

// NewMemRepo 创建内存仓储。

   func NewMemRepo() *MemRepo {
	  return &MemRepo{
		   leases: map[string]CreditDeliverLease{},
		   cases:  map[string]CreditFenceCase{},
		   proofs: map[string]CreditProof{},
		   gens: map[string]BoardGeneration{},
	   }
   }

// UpsertLease 插入或更新租约。
  func (r *MemRepo) UpsertLease(_ context.Context, l CreditDeliverLease) error {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  r.leases[l.CreditKey] = l
	  return nil
  }

// LeaseByKey 按 creditKey 读取租约。

  func (r *MemRepo) LeaseByKey(_ context.Context, creditKey string) (CreditDeliverLease, bool, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  l, ok := r.leases[creditKey]
	   return l, ok, nil
   }

// ActiveLeases 返回指定板卡的活跃租约列表。
   func (r *MemRepo) ActiveLeases(_ context.Context, boardKey string) ([]CreditDeliverLease, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  var out []CreditDeliverLease
	   for _, l := range r.leases {
		   if l.BoardKey == boardKey && (l.State == LeaseEnqueued || l.State == LeaseDelivering) {
			   out = append(out, l)
		   }
	  }
	   return out, nil
  }

// InsertCase 追加事故证明（同 ID 幂等，不覆盖）。

   func (r *MemRepo) InsertCase(_ context.Context, c CreditFenceCase) error {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  if _, exists := r.cases[c.ID]; !exists {
		   r.cases[c.ID] = c
	   }
	  return nil
  }

// CasesByKey 按 creditKey 读取事故证明。
  func (r *MemRepo) CasesByKey(_ context.Context, creditKey string) ([]CreditFenceCase, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  var out []CreditFenceCase
	  for _, c := range r.cases {
		   if c.CreditKey == creditKey {
			   out = append(out, c)
		  }
	   }
	  return out, nil
  }

// CasesByBoard 按 boardKey 读取事故证明。
   func (r *MemRepo) CasesByBoard(_ context.Context, boardKey string) ([]CreditFenceCase, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  var out []CreditFenceCase
	   for _, c := range r.cases {
		   if c.BoardKey == boardKey {
			   out = append(out, c)
		  }
	  }
	   return out, nil
  }

// UnexportedCases 返回未导出的事故证明。

   func (r *MemRepo) UnexportedCases(_ context.Context, limit int) ([]CreditFenceCase, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  var out []CreditFenceCase
	  for _, c := range r.cases {
		   if c.ExportedAt == nil {
			  out = append(out, c)
			  if len(out) >= limit {
				  break
			  }
		  }
	  }
	   return out, nil
   }

// InsertProof 插入投递证明（同 ID 幂等）。

   func (r *MemRepo) InsertProof(_ context.Context, p CreditProof) error {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   if _, exists := r.proofs[p.CreditKey]; !exists {
		   r.proofs[p.CreditKey] = p
	   }
	   return nil
  }

// MarkExported 标记证明已导出。

  func (r *MemRepo) MarkExported(_ context.Context, proofID string, at time.Time) error {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  for k, p := range r.proofs {
		  if p.ID == proofID {
			   p.ExportedAt = &at
			  r.proofs[k] = p
			   return nil
		  }
	  }
	   return nil
  }

// ProofByKey 按 creditKey 读取投递证明。

  func (r *MemRepo) ProofByKey(_ context.Context, creditKey string) (CreditProof, bool, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  p, ok := r.proofs[creditKey]
	   return p, ok, nil
   }

// UnexportedProofs 返回未导出的投递证明。

  func (r *MemRepo) UnexportedProofs(_ context.Context, limit int) ([]CreditProof, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  var out []CreditProof
	   for _, p := range r.proofs {
		  if p.ExportedAt == nil {
			   out = append(out, p)
			   if len(out) >= limit {
				  break
			   }
		  }
	  }
	  return out, nil
  }

// UpsertGen 插入或更新板卡世代。
  func (r *MemRepo) UpsertGen(_ context.Context,
	   bg BoardGeneration) error {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   r.gens[bg.BoardKey] = bg
	  return nil
   }

// GenByKey 按 boardKey 读取板卡世代。

   func (r *MemRepo) GenByKey(_ context.Context, boardKey string) (BoardGeneration, bool, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   bg, ok := r.gens[boardKey]
	   return bg, ok, nil
  }

// AllGens 返回全部板卡世代。

  func (r *MemRepo) AllGens(_ context.Context) ([]BoardGeneration, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  out := make([]BoardGeneration, 0, len(r.gens))
	  for _, bg := range r.gens {
		  out = append(out, bg)
	   }
	  return out, nil
   }

// ─────────────────────────────────────────────────────────────
// 序列化辅助（用于持久化层 JSON 编码）
// ─────────────────────────────────────────────────────────────
// MarshalLease 将 CreditDeliverLease 序列化为 JSON。
   func MarshalLease(l CreditDeliverLease) ([]byte, error) {
	   return json.Marshal(l)
  }

// UnmarshalLease 将 JSON 反序列化为 CreditDeliverLease。
  func UnmarshalLease(data []byte) (CreditDeliverLease, error) {
	   var l CreditDeliverLease
	   err := json.Unmarshal(data, &l)
	   return l, err
  }
