// @FileName: repo.go
// @Summary: 取消记录、漂移证明与兜底拒绝证明仓储。
// 重点：幂等键冲突返回稳定错误；证明只增。
// 风险点：覆盖已成功取消会破坏双入口一致性。
// @Author: Hale
// @Created: 2021.06.15

package fundmesh

import (
	"context"
	"sync"
	"time"
)

type CancelRepo interface {
	Put(ctx context.Context, rec *CancelRecord) error
	GetByWithdraw(ctx context.Context, withdrawID string) (*CancelRecord, error)
	GetByIdem(ctx context.Context, key string) (*CancelRecord, error)
}

type ProofRepo interface {
	PutDrift(p DriftProof)
	PutFallback(p FallbackProof)
	ListFallback() []FallbackProof
	ListDrift() []DriftProof
}

type MemCancelRepo struct {
	mu sync.RWMutex
	byWD   map[string]*CancelRecord
	byIdem map[string]*CancelRecord
}

   func NewMemCancelRepo() *MemCancelRepo {
	  return &MemCancelRepo{byWD: map[string]*CancelRecord{}, byIdem: map[string]*CancelRecord{}}
  }

  func (r *MemCancelRepo) Put(_ context.Context, rec *CancelRecord) error {
	  if r == nil || rec == nil || rec.WithdrawID == "" {
		  return ErrInvalidInput
	  }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   cp := *rec
	   if rec.IdempotencyKey != "" {
		   if old, ok := r.byIdem[rec.IdempotencyKey]; ok && old.WithdrawID != rec.WithdrawID {
			  return ErrIdemConflict
		   }
		   r.byIdem[rec.IdempotencyKey] = &cp
	   }
	  r.byWD[rec.WithdrawID] = &cp
	   return nil
   }

   func (r *MemCancelRepo) GetByWithdraw(_ context.Context, withdrawID string) (*CancelRecord, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  v, ok := r.byWD[withdrawID]
	   if !ok {
		   return nil, ErrWithdrawNotFound
	  }
	  cp := *v
	   return &cp, nil
   }

   func (r *MemCancelRepo) GetByIdem(_ context.Context, key string) (*CancelRecord, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   v, ok := r.byIdem[key]
	  if !ok {
		  return nil, ErrWithdrawNotFound
	   }
	  cp := *v
	   return &cp, nil
  }

type MemProofRepo struct {
	mu sync.Mutex
	fb []FallbackProof
	dr []DriftProof
}

   func NewMemProofRepo() *MemProofRepo { return &MemProofRepo{} }

  func (r *MemProofRepo) PutDrift(p DriftProof) {
	  if r == nil {
		   return
	  }
	  if p.At.IsZero() {
		  p.At = time.Now().UTC()
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  r.dr = append(r.dr, p)
   }

  func (r *MemProofRepo) PutFallback(p FallbackProof) {
	   if r == nil {
		   return
	   }
	   if p.At.IsZero() {
		   p.At = time.Now().UTC()
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  r.fb = append(r.fb, p)
  }

  func (r *MemProofRepo) ListFallback() []FallbackProof {
	  if r == nil {
		  return nil
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]FallbackProof,
		  len(r.fb))
	  copy(out, r.fb)
	   return out
  }

   func (r *MemProofRepo) ListDrift() []DriftProof {
	   if r == nil {
		  return nil
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]DriftProof, len(r.dr))
	  copy(out, r.dr)
	  return out
  }
