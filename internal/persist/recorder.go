// persist/recorder.go

// Author: Axel
// Date: 2020.09.08

package persist

import (
	"context"
	"sync"
)

// Op 一条逻辑写。MemKV 和 SQL 适配器必须对同一序列得到相同可见状态。
type Op struct {
	Name string
	Kind string
	Key  string
	Body  []byte
	Fence Fence
}

type Recorder struct {
	mu sync.Mutex
	ops []Op
	KV  KV
}

   func NewRecorder(kv KV) *Recorder {
	  if kv == nil {
		   kv = NewMemKV()
	   }
	  return &Recorder{KV: kv}
   }

   func (r *Recorder) note(op Op) {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  if op.Body != nil {
		  op.Body = append([]byte(nil), op.Body...)
	  }
	  r.ops = append(r.ops, op)
  }

   func (r *Recorder) Ops() []Op {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  out := make([]Op, len(r.ops))
	  copy(out, r.ops)
	   return out
   }

  func (r *Recorder) InTx(ctx context.Context, fn func(context.Context) error) error {
	   return r.KV.InTx(ctx, fn)
   }

  func (r *Recorder) Get(ctx context.Context, kind, key string) (Row, error) {
	   return r.KV.Get(ctx, kind, key)
   }

  func (r *Recorder) Put(ctx context.Context, row Row, expected Fence) error {
	   err := r.KV.Put(ctx, row, expected)
	   if err == nil {
		  r.note(Op{Name: "put", Kind: row.Kind, Key: row.Key, Body: row.Body, Fence: expected})
	   }
	  return err
   }

   func (r *Recorder) Delete(ctx context.Context, kind, key string, expected Fence) error {
	  err := r.KV.Delete(ctx, kind, key, expected)
	  if err == nil {
		   r.note(Op{Name: "delete", Kind: kind, Key: key, Fence: expected})
	   }
	  return err
   }

  func (r *Recorder) List(ctx context.Context, kind string, limit int) ([]Row, error) {
	  return r.KV.List(ctx, kind, limit)
   }

   func (r *Recorder) ListAfter(ctx context.Context, kind, after string, limit int) ([]Row, error) {
	  return r.KV.ListAfter(ctx, kind, after, limit)
  }

// Replay 把同一串写打到 dst。用于 MemDB 互证：两套实现最终 Get 必须一致。

   func Replay(ctx context.Context, dst KV, ops []Op) error {
	   for _, op := range ops {
		  switch op.Name {
		   case "put":
			  row, err := dst.Get(ctx, op.Kind, op.Key)
			  expected := Fence{}
			   if err == nil {
				  expected = row.Fence
			  }
			   if err := dst.Put(ctx, Row{Kind: op.Kind, Key: op.Key, Body: op.Body}, expected); err != nil {
				   return err
			   }
		  case "delete":
			   row, err := dst.Get(ctx, op.Kind, op.Key)
			  if err != nil {
				   return err
			   }
			   if err := dst.Delete(ctx, op.Kind, op.Key, row.Fence); err != nil {
				   return err
			   }
		   }
	  }
	  return nil
  }

// DML 一条 Put/Delete 对应的 MySQL 语句，给方言改写核对。

   func DML(op Op, d Dialect) string {
	  var sql string
	  switch op.Name {
	  case "put":
		   sql = "INSERT INTO aa_kv (row_id, kind, row_key, body, fence_epoch, fence_token) VALUES (?,?,?,?,?,?) ON DUPLICATE KEY UPDATE body=VALUES(body), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token)"
	   case "delete":
		   sql = "DELETE FROM aa_kv WHERE kind=? AND row_key=? AND fence_epoch=? AND fence_token=?"
	   default:
		   sql = "SELECT row_id FROM aa_kv WHERE kind=? AND row_key=?"
	  }
	  return Rewrite(sql, d)
   }
