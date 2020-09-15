package persist

import (
	"context"
	"strings"
	"sync"
	"time"
)

// RefuseMemRegister 档若 STORAGE=mem 或绑定进程内 Mem，失败即关；非允许 Mem。

   func RefuseMemRegister(production bool, storage string, memBound bool) error {
	   if !production {
		   return nil
	   }
	   st := strings.ToLower(strings.TrimSpace(storage))
	  if st == "mem" || memBound {
		  return ErrMemRegister
	  }
	  return nil
   }

// TableRegister 进程内表夹具，列一致 013x 租约：gate_key / holder / expires_at。
// 共享 rows 的两个实例进程重启后自 SQL 重载。
type TableRegister struct {
	mu    sync.Mutex
	rows  map[string]Lease
	clock func() time.Time
}

   func NewTableRegister() *TableRegister {
	  return &TableRegister{rows: map[string]Lease{}, clock: func() time.Time { return time.Now().UTC() }}
   }

   func OpenTableRegister(rows map[string]Lease) *TableRegister {
	   if rows == nil {
		  rows = map[string]Lease{}
	  }
	   return &TableRegister{rows: rows, clock: func() time.Time { return time.Now().UTC() }}
  }

  func (t *TableRegister) WithClock(c func() time.Time) *TableRegister {
	  if t != nil && c != nil {
		   t.clock = c
	   }
	   return t
  }

  func (t *TableRegister) now() time.Time {
	  if t == nil || t.clock == nil {
		  return time.Now().UTC()
	  }
	   return t.clock()
  }

   func (t *TableRegister) Register(_ context.Context, key, holder string, ttl time.Duration) (Lease, error) {
	   return t.put(key, holder, ttl)
  }

   func (t *TableRegister) put(key, holder string, ttl time.Duration) (Lease, error) {
	   if t == nil || t.rows == nil {
		  return Lease{}, ErrTxClosed
	  }
	   key = strings.TrimSpace(key)
	   holder = strings.TrimSpace(holder)
	   if key == "" || holder == "" {
		   return Lease{}, ErrConflict
	   }
	   if ttl <= 0 {
		   ttl = 8 * time.Second
	  }
	   t.mu.Lock()
	   defer t.mu.Unlock()
	  now := t.now()
	   cur, ok := t.rows[key]
	   if ok && now.Before(cur.ExpiresAt) && cur.Holder != holder {
		   return Lease{}, ErrConflict
	   }
	  ls := Lease{Name: key, Holder: holder, ExpiresAt: now.Add(ttl)}
	  t.rows[key] = ls
	   return ls, nil
   }

  func (t *TableRegister) Get(_ context.Context, key string) (Lease, error) {
	  if t == nil {
		   return Lease{}, ErrTxClosed
	  }
	   t.mu.Lock()
	  defer t.mu.Unlock()
	   cur, ok := t.rows[strings.TrimSpace(key)]
	  if !ok || !t.now().Before(cur.ExpiresAt) {
		   return Lease{}, ErrNotFound
	   }
	  return cur, nil
   }

   func (t *TableRegister) Renew(ctx context.Context, key, holder string, ttl time.Duration) (Lease, error) {
	   cur, err := t.Get(ctx, key)
	   if err != nil {
		  return Lease{}, err
	   }
	   if strings.TrimSpace(cur.Holder) != strings.TrimSpace(holder) {
		  return Lease{}, ErrConflict
	  }
	  return t.put(key, holder, ttl)
  }

  func (t *TableRegister) Release(_ context.Context, key, holder string) error {
	   if t == nil {
		  return ErrTxClosed
	  }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  k := strings.TrimSpace(key)
	   cur, ok := t.rows[k]
	   if !ok {
		   return ErrNotFound
	   }
	  if cur.Holder != strings.TrimSpace(holder) {
		   return ErrStaleFence
	   }
	   delete(t.rows, k)
	   return nil
  }

   func (t *TableRegister) ListActive(_ context.Context) []Lease {
	  if t == nil {
		  return nil
	   }
	   t.mu.Lock()
	   defer t.mu.Unlock()
	   now := t.now()
	  out := make([]Lease,
		  0, len(t.rows))
	   for _, ls := range t.rows {
		   if now.Before(ls.ExpiresAt) {
			   out = append(out, ls)
		  }
	  }
	   return out
  }

  func (t *TableRegister) Rows() map[string]Lease {
	   if t == nil {
		   return nil
	   }
	  return t.rows
  }
