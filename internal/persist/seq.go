// 功能：persist 模块 seq。
// 通用持久化辅助。

// 作者：TianpengLan
// 日期：2020.09.09

package persist

import (
	"context"
	"strconv"
	"strings"
)

const kindSeq = "seq"

// Counter 命名序号。每次 Next 用当前围栏 CAS +1，失败整段回滚。

type Counter struct {
	kv KV
}

   func NewCounter(kv KV) *Counter {
	   if kv == nil {
		   kv = NewMemKV()
	  }
	  return &Counter{kv: kv}
  }

   func (c *Counter) KV() KV { return c.kv }

   func seqName(name string) string {
	  return strings.TrimSpace(name)
  }

// Next 取下一个正整数。name 空拒。首次写入 epoch=0。
  func (c *Counter) Next(ctx context.Context, name string) (int64, error) {
	  if c == nil || c.kv == nil {
		  return 0, ErrTxClosed
	   }
	  name = seqName(name)
	  if name == "" {
		  return 0, ErrConflict
	  }
	  var out int64
	  err := c.kv.InTx(ctx, func(ctx context.Context) error {
		   row, err := c.kv.Get(ctx, kindSeq, name)
		   if err != nil {
			   if err != ErrNotFound {
				  return err
			   }
			  out = 1
			   return c.kv.Put(ctx, Row{Kind: kindSeq, Key: name, Body: []byte("1")}, Fence{})
		  }
		  n, perr := strconv.ParseInt(strings.TrimSpace(string(row.Body)), 10, 64)
		   if perr != nil || n < 0 {
			   return ErrConflict
		   }
		   out = n + 1
		   return c.kv.Put(ctx, Row{Kind: kindSeq, Key: name, Body: []byte(strconv.FormatInt(out, 10))}, row.Fence)
	   })
	   return out, err
   }

  func (c *Counter) Current(ctx context.Context, name string) (int64, error) {
	  if c == nil || c.kv == nil {
		   return 0, ErrTxClosed
	   }
	  row, err := c.kv.Get(ctx, kindSeq, seqName(name))
	  if err != nil {
		   return 0, err
	   }
	   n, perr := strconv.ParseInt(strings.TrimSpace(string(row.Body)), 10, 64)
	   if perr != nil {
		  return 0, ErrConflict
	  }
	  return n, nil
  }
