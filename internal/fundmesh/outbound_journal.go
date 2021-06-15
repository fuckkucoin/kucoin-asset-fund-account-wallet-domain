// @FileName: outbound_journal.go
// @Summary: 出站扣减流水。
// 重点：同一幂等键只落一行成功扣减。
// 风险点：replay 路径写第二行等于二次扣减。
// @Author: Likeo
// @Created: 2021.06.14

package fundmesh

import (
	"context"
	"sync"
	"time"
)

type DebitJournal interface {
	Append(ctx context.Context, line DebitLine) error
	GetByIdem(ctx context.Context, key string) (*DebitLine, error)
	CountByWithdraw(ctx context.Context, withdrawID string) (int, error)
	CountOK(ctx context.Context, key string) (int, error)
}

type MemDebitJournal struct {
	mu     sync.Mutex
	byIdem map[string]DebitLine
	byWD   map[string][]DebitLine
	all    []DebitLine
}

   func NewMemDebitJournal() *MemDebitJournal {
	   return &MemDebitJournal{byIdem: map[string]DebitLine{},
		  byWD: map[string][]DebitLine{}}
   }

   func (j *MemDebitJournal) Append(_ context.Context, line DebitLine) error {
	  if j == nil || line.IdempotencyKey == "" {
		  return ErrInvalidInput
	   }
	  if line.At.IsZero() {
		   line.At = time.Now().UTC()
	   }
	   j.mu.Lock()
	  defer j.mu.Unlock()
	   if old, ok := j.byIdem[line.IdempotencyKey]; ok && old.OK {
		   return ErrDoubleDebitForbidden
	  }
	   j.byIdem[line.IdempotencyKey] = line
	   if line.WithdrawID != "" {
		  j.byWD[line.WithdrawID] = append(j.byWD[line.WithdrawID], line)
	  }
	  j.all = append(j.all, line)
	  return nil
  }

  func (j *MemDebitJournal) GetByIdem(_ context.Context, key string) (*DebitLine, error) {
	   if j == nil || key == "" {
		   return nil, ErrWithdrawNotFound
	   }
	   j.mu.Lock()
	   defer j.mu.Unlock()
	  v, ok := j.byIdem[key]
	  if !ok {
		  return nil, ErrWithdrawNotFound
	   }
	   cp := v
	  return &cp, nil
  }

   func (j *MemDebitJournal) CountByWithdraw(_ context.Context, withdrawID string) (int, error) {
	   if j == nil {
		  return 0, ErrOutboundUnavailable
	   }
	   j.mu.Lock()
	  defer j.mu.Unlock()
	  n := 0
	  for _, ln := range j.byWD[withdrawID] {
		   if ln.OK {
			   n++
		   }
	   }
	   return n, nil
   }

   func (j *MemDebitJournal) CountOK(_ context.Context, key string) (int, error) {
	  if j == nil {
		   return 0, ErrOutboundUnavailable
	  }
	  j.mu.Lock()
	  defer j.mu.Unlock()
	   v, ok := j.byIdem[key]
	   if !ok || !v.OK {
		   return 0, nil
	  }
	  return 1, nil
  }

  func (j *MemDebitJournal) All() []DebitLine {
	   if j == nil {
		   return nil
	  }
	   j.mu.Lock()
	   defer j.mu.Unlock()
	  out := make([]DebitLine, len(j.all))
	  copy(out, j.all)
	   return out
   }
