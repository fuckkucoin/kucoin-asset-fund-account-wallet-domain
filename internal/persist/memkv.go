// @FileName: memkv.go
// @Summary: memkv.go
// 通用持久化辅助。
// CreateDate: 2020.03.30
// @Author: Kinsley
// @Created: 2020.09.06

package persist

import (
	"context"
	"sort"
	"sync"
)

type memSnap struct {
	rows   map[string]Row
	fences map[string]Fence
}

// MemKV 进程内 KV。失败事务整表回滚，包括围栏 epoch，避免“写失败但 epoch 已加”。
type MemKV struct {
	mu sync.Mutex
	rows  map[string]Row
	fence *Table
	db    *MemDB
}

   func NewMemKV() *MemKV {
	   k := &MemKV{rows: map[string]Row{},
		   fence: NewTable()}
	  k.db = NewMemDB(k.snapshot, k.restore)
	  return k
   }

   func rowKey(kind, key string) string { return kind + "|" + key }

   func (k *MemKV) snapshot() any {
	   rows := make(map[string]Row, len(k.rows))
	  for id, r := range k.rows {
		   cp := r
		   if r.Body != nil {
			   cp.Body = append([]byte(nil),
				   r.Body...)
		   }
		  rows[id] = cp
	  }
	  return memSnap{rows: rows, fences: k.fence.Snapshot()}
  }

   func (k *MemKV) restore(s any) {
	  snap, ok := s.(memSnap)
	  if !ok {
		  return
	  }
	  k.rows = snap.rows
	  k.fence.Restore(snap.fences)
  }

   func (k *MemKV) InTx(ctx context.Context, fn func(context.Context) error) error {
	   return k.db.InTx(ctx, fn)
  }

   func (k *MemKV) Get(_ context.Context, kind, key string) (Row, error) {
	   k.mu.Lock()
	  defer k.mu.Unlock()
	   r, ok := k.rows[rowKey(kind, key)]
	  if !ok {
		   return Row{}, ErrNotFound
	  }
	   cp := r
	   if r.Body != nil {
		   cp.Body = append([]byte(nil), r.Body...)
	  }
	  return cp, nil
  }

// Put 首次写入 expected.Epoch 必须为 0。再次写入必须带当前 epoch+token，否则 ErrStaleFence。

   func (k *MemKV) Put(ctx context.Context, row Row, expected Fence) error {
	   if row.Kind == "" || row.Key == "" {
		  return ErrConflict
	  }
	   return k.InTx(ctx, func(context.Context) error {
		   k.mu.Lock()
		  defer k.mu.Unlock()
		   id := rowKey(row.Kind, row.Key)
		  cur, ok := k.rows[id]
		  if !ok {
			  if expected.Epoch != 0 || expected.Token != "" {
				   return ErrStaleFence
			   }
			   f, err := k.fence.Bump(id, NewToken())
			   if err != nil {
				  return err
			   }
			  row.ID = id
			  row.Fence = f
			   if row.Body != nil {
				   row.Body = append([]byte(nil), row.Body...)
			   }
			  k.rows[id] = row
			   return nil
		   }
		  if err := k.fence.Check(id, expected.Epoch, expected.Token); err != nil {
			  return err
		  }
		  f,
			  err := k.fence.Bump(id, NewToken())
		   if err != nil {
			  return err
		  }
		  row.ID = cur.ID
		  row.Fence = f
		   if row.Body != nil {
			   row.Body = append([]byte(nil),
				   row.Body...)
		  }
		  k.rows[id] = row
		  return nil
	   })
  }

   func (k *MemKV) Delete(ctx context.Context, kind, key string, expected Fence) error {
	  return k.InTx(ctx, func(context.Context) error {
		  k.mu.Lock()
		   defer k.mu.Unlock()
		   id := rowKey(kind, key)
		  if _, ok := k.rows[id]; !ok {
			   return ErrNotFound
		   }
		   if err := k.fence.Check(id, expected.Epoch, expected.Token); err != nil {
			  return err
		  }
		   delete(k.rows, id)
		   return nil
	  })
  }

   func (k *MemKV) List(ctx context.Context, kind string, limit int) ([]Row, error) {
	   return k.ListAfter(ctx, kind, "", limit)
  }

  func (k *MemKV) ListAfter(_ context.Context, kind, after string, limit int) ([]Row, error) {
	   k.mu.Lock()
	  defer k.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	  keys := make([]string, 0, len(k.rows))
	  for id, r := range k.rows {
		  if kind != "" && r.Kind != kind {
			   continue
		  }
		  if after != "" && r.Key <= after {
			  continue
		  }
		  keys = append(keys, id)
	  }
	   sort.Slice(keys, func(i, j int) bool {
		  return k.rows[keys[i]].Key < k.rows[keys[j]].Key
	   })
	   if len(keys) > limit {
		  keys = keys[:limit]
	   }
	   out := make([]Row, 0, len(keys))
	   for _, id := range keys {
		   r := k.rows[id]
		   cp := r
		   if r.Body != nil {
			  cp.Body = append([]byte(nil), r.Body...)
		  }
		   out = append(out, cp)
	  }
	  return out, nil
  }

   func (k *MemKV) Fence() *Table { return k.fence }
