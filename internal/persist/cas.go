// 功能：通用持久化辅助。
// 作者：TianpengLan
// 日期：2020.08.30

package persist

import (
	"bytes"
	"context"
	"strings"
)

// PutNew 首次写入。expected 必须是零围栏，否则当冲突。

  func PutNew(ctx context.Context, kv KV, row Row) error {
	  if kv == nil {
		   return ErrTxClosed
	  }
	   return kv.Put(ctx, row, Fence{})
   }

// Replace 读当前围栏再写。中间被别人 Bump 则 ErrStaleFence。
  func Replace(ctx context.Context, kv KV, kind, key string, body []byte) error {
	  if kv == nil {
		  return ErrTxClosed
	  }
	  return kv.InTx(ctx, func(ctx context.Context) error {
		   cur, err := kv.Get(ctx, kind, key)
		   if err != nil {
			  return err
		  }
		   cur.Body = append([]byte(nil), body...)
		   return kv.Put(ctx,
			   cur, cur.Fence)
	  })
  }

// GetOrInit 没有行就按 body 建。existed=true 表示读到已有行。

   func GetOrInit(ctx context.Context, kv KV, kind, key string, body []byte) (Row, bool, error) {
	  if kv == nil {
		   return Row{}, false, ErrTxClosed
	  }
	   kind = strings.TrimSpace(kind)
	   key = strings.TrimSpace(key)
	  var out Row
	   var existed bool
	  err := kv.InTx(ctx, func(ctx context.Context) error {
		  cur, err := kv.Get(ctx, kind, key)
		  if err == nil {
			  out = cur
			  existed = true
			   return nil
		  }
		   if err != ErrNotFound {
			   return err
		   }
		   row := Row{Kind: kind, Key: key, Body: append([]byte(nil), body...)}
		  if err := kv.Put(ctx, row, Fence{}); err != nil {
			   return err
		  }
		  got, gerr := kv.Get(ctx, kind, key)
		   if gerr != nil {
			  return gerr
		  }
		   out = got
		  return nil
	  })
	  return out, existed, err
  }

// CompareBody 只比内容，不比围栏。给幂等回放用。
  func CompareBody(a, b []byte) bool {
	   return bytes.Equal(a, b)
  }
