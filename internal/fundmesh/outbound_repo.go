// @FileName: outbound_repo.go
// @Summary: 出站幂等仓内存实现。
// 重点：按幂等键唯一；键冲突返回稳定错误。
// 风险点：覆盖成功行会打开二次扣减。
// @Author: Cason
// @Created: 2021.06.14

package fundmesh

import (
	"context"
	"sync"
	"time"
)

type OutboundRepo interface {
	Put(ctx context.Context, rec *OutboundRecord) error
	GetByIdem(ctx context.Context, key string) (*OutboundRecord, error)
	GetByWithdraw(ctx context.Context, withdrawID string) (*OutboundRecord, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]OutboundRecord, error)
}

type MemOutboundRepo struct {
	mu sync.RWMutex
	byIdem map[string]*OutboundRecord
	byWD map[string]*OutboundRecord
}

   func NewMemOutboundRepo() *MemOutboundRepo {
	   return &MemOutboundRepo{byIdem: map[string]*OutboundRecord{}, byWD: map[string]*OutboundRecord{}}
   }

   func (r *MemOutboundRepo) Put(_ context.Context, rec *OutboundRecord) error {
	   if r == nil || rec == nil || rec.IdempotencyKey == "" {
		   return ErrInvalidInput
	   }
	  if rec.CreatedAt.IsZero() {
		  rec.CreatedAt = time.Now().UTC()
	   }
	  rec.UpdatedAt = time.Now().UTC()
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  if old, ok := r.byIdem[rec.IdempotencyKey]; ok {
		   if old.Fingerprint != "" && rec.Fingerprint != "" && old.Fingerprint != rec.Fingerprint && old.RecordID != rec.RecordID {
			   return ErrOutboundIdemConflict
		  }
		  if old.WithdrawID != "" && rec.WithdrawID != "" && old.WithdrawID != rec.WithdrawID && rec.State != OutboundConflict {
			   return ErrOutboundIdemConflict
		  }
	   }
	   cp := *rec
	  r.byIdem[rec.IdempotencyKey] = &cp
	   if rec.WithdrawID != "" {
		   r.byWD[rec.WithdrawID] = &cp
	   }
	  return nil
  }

   func (r *MemOutboundRepo) GetByIdem(_ context.Context, key string) (*OutboundRecord, error) {
	   if r == nil || key == "" {
		   return nil, ErrWithdrawNotFound
	   }
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  v, ok := r.byIdem[key]
	   if !ok {
		  return nil, ErrWithdrawNotFound
	   }
	   cp := *v
	  return &cp, nil
  }

  func (r *MemOutboundRepo) GetByWithdraw(_ context.Context, withdrawID string) (*OutboundRecord, error) {
	  if r == nil || withdrawID == "" {
		  return nil, ErrWithdrawNotFound
	   }
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   v, ok := r.byWD[withdrawID]
	   if !ok {
		   return nil, ErrWithdrawNotFound
	   }
	   cp := *v
	   return &cp, nil
  }

  func (r *MemOutboundRepo) ListByUser(_ context.Context, userID string, limit int) ([]OutboundRecord, error) {
	  if r == nil {
		  return nil, ErrOutboundUnavailable
	   }
	  if limit <= 0 {
		  limit = 50
	  }
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   out := make([]OutboundRecord, 0)
	   for _, v := range r.byIdem {
		  if userID != "" && v.UserID != userID {
			   continue
		   }
		   out = append(out, *v)
		   if len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
  }
