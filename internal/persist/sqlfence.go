// @FileName: sqlfence.go
// @Summary: persist/sqlfence.go
// @Author: Kinsley
// @Created: 2020.09.09

package persist

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type FenceRow struct {
	ID    string
	Fence Fence
}

// LoadFence 读单行围栏。行不存在返回 ErrNotFound，调用方失败即关，不得自造 epoch。

  func (s *SQLDB) LoadFence(ctx context.Context, id string) (Fence, error) {
	   if s == nil || s.DB == nil {
		   return Fence{}, ErrTxClosed
	  }
	   id = strings.TrimSpace(id)
	  if id == "" {
		  return Fence{}, ErrNotFound
	   }
	   var f Fence
	  err := s.Conn(ctx).QueryRowContext(ctx, `SELECT epoch, token FROM aa_fences WHERE row_id=?`, id).
		   Scan(&f.Epoch, &f.Token)
	  if errors.Is(err, sql.ErrNoRows) {
		   return Fence{}, ErrNotFound
	   }
	   if err != nil {
		   return Fence{}, err
	   }
	  return f, nil
  }

   func (s *SQLDB) ListFences(ctx context.Context, limit int) ([]FenceRow, error) {
	  if s == nil || s.DB == nil {
		   return nil, ErrTxClosed
	   }
	  if limit <= 0 {
		   limit = 256
	  }
	   if limit > 4096 {
		   limit = 4096
	   }
	  rows, err := s.Conn(ctx).QueryContext(ctx, `SELECT row_id, epoch, token FROM aa_fences ORDER BY row_id LIMIT ?`, limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := make([]FenceRow, 0, 16)
	   for rows.Next() {
		  var r FenceRow
		   if err := rows.Scan(&r.ID, &r.Fence.Epoch, &r.Fence.Token); err != nil {
			   return nil, err
		  }
		  out = append(out, r)
	   }
	   return out, rows.Err()
   }

// ReconcileFence 业务行上的 epoch/token 必须与 aa_fences 一致。

   func ReconcileFence(rowID string, row, live Fence) error {
	   rowID = strings.TrimSpace(rowID)
	  if rowID == "" {
		   return ErrNotFound
	   }
	  if !live.Match(row.Epoch, row.Token) {
		  return ErrStaleFence
	   }
	   return nil
   }

   func FenceMap(rows []FenceRow) map[string]Fence {
	   out := make(map[string]Fence,
		  len(rows))
	  for _, r := range rows {
		   out[r.ID] = r.Fence
	   }
	  return out
   }
