// 通用持久化辅助之分页辅助。

// Author: Axel
// Date: 2020.09.07

package persist

import "context"

// 按 key 向后翻。After 空从最小 key 起。
type Cursor struct {
	After string
	Limit int
}

type Page struct {
	Rows []Row
	Next string
	Done bool
}

// PageKind 走 KV.ListAfter 真游标，禁止先 List 截断再内存切片（大母子树会丢行）。
   func PageKind(ctx context.Context, kv KV, kind string, cur Cursor) (Page, error) {
	   if kv == nil {
		  return Page{Done: true}, ErrTxClosed
	  }
	  if cur.Limit <= 0 {
		   cur.Limit = 50
	  }
	  rows, err := kv.ListAfter(ctx, kind, cur.After, cur.Limit)
	  if err != nil {
		   return Page{}, err
	   }
	   p := Page{Rows: rows, Done: len(rows) < cur.Limit}
	   if !p.Done && len(rows) > 0 {
		   p.Next = rows[len(rows)-1].Key
	  }
	   return p, nil
  }
