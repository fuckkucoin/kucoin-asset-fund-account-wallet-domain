package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// SubVelocityHit 母子划转频次命中行。

type SubVelocityHit struct {
	ID     string
	Owner  string
	Amount string
	At     time.Time
}

// ListVelocityHits 列出 owner（空=全量）自 since 起的命中。
   func (r *SubAcct) ListVelocityHits(ctx context.Context, owner string, since time.Time) ([]SubVelocityHit, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  if since.IsZero() {
		   since = time.Now().UTC().Add(-24 * time.Hour)
	  }
	  owner = strings.TrimSpace(owner)
	   var rows *sql.Rows
	  var err error
	  if owner == "" {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, owner_id, amount, hit_at FROM aa_subacct_velocity_hits WHERE hit_at>=? ORDER BY hit_at ASC`, since)
	  } else {
		  rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, owner_id, amount, hit_at FROM aa_subacct_velocity_hits WHERE owner_id=? AND hit_at>=? ORDER BY hit_at ASC`,
			   owner, since)
	   }
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]SubVelocityHit, 0)
	   for rows.Next() {
		   var h SubVelocityHit
		   if err := rows.Scan(&h.ID, &h.Owner, &h.Amount, &h.At); err != nil {
			   return nil, err
		   }
		  out = append(out, h)
	  }
	  return out, rows.Err()
   }

// ReserveVelocityCAS 跨实例频次占用——事务内锁 owner 命中后校验并插入。
// 按 hourly/daily/daily_sum 闸；24h 窗外行不计入。
// 超额返回 ErrConflict；多实例必须以本 CAS 为准。
   func (r *SubAcct) ReserveVelocityCAS(ctx context.Context, owner, amount string, hourly, daily int, dailySum string, now time.Time) (SubVelocityHit, error) {
	  if !sqlOn(r.db) {
		  return SubVelocityHit{}, ErrUnavailable
	  }
	   owner = strings.TrimSpace(owner)
	   amount = strings.TrimSpace(amount)
	   if owner == "" || amount == "" {
		  return SubVelocityHit{}, ErrNotFound
	  }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	   amt, err := money.Parse(amount)
	  if err != nil {
		  return SubVelocityHit{}, err
	  }
	  var out SubVelocityHit
	  err = r.db.InTx(ctx, func(txCtx context.Context) error {
		  // 锁闸行（无则插入空闸）串行化同 owner。
		  if err := r.lockVelocityGate(txCtx, owner, now); err != nil {
			  return err
		  }
		   since := now.Add(-24 * time.Hour)
		  rows, err := r.db.Conn(txCtx).QueryContext(txCtx,
			   `SELECT id, owner_id, amount, hit_at FROM aa_subacct_velocity_hits WHERE owner_id=? AND hit_at>=? ORDER BY hit_at ASC FOR UPDATE`,
			  owner, since)
		   if err != nil {
			  return err
		   }
		   hour, day := 0, 0
		  sum, _ := money.ParseBook("0")
		   for rows.Next() {
			  var h SubVelocityHit
			   if err := rows.Scan(&h.ID, &h.Owner, &h.Amount, &h.At); err != nil {
				   rows.Close()
				  return err
			  }
			  day++
			  if now.Sub(h.At) <= time.Hour {
				   hour++
			   }
			   got,
				  e := money.Parse(h.Amount)
			   if e == nil {
				  sum, _ = sum.Add(got)
			  }
		   }
		  if err := rows.Err(); err != nil {
			   rows.Close()
			   return err
		   }
		  rows.Close()
		   if hourly > 0 && hour >= hourly {
			   return ErrConflict
		  }
		   if daily > 0 && day >= daily {
			  return ErrConflict
		   }
		   if strings.TrimSpace(dailySum) != "" {
			  capAmt, e := money.Parse(dailySum)
			   if e != nil {
				  return ErrConflict
			   }
			  next, e := sum.Add(amt)
			   if e != nil || next.Cmp(capAmt) > 0 {
				  return ErrConflict
			   }
		   }
		  out = SubVelocityHit{ID: uuid.NewString(), Owner: owner, Amount: amount, At: now}
		   _, err = r.db.Conn(txCtx).ExecContext(txCtx,
			   `INSERT INTO aa_subacct_velocity_hits (id, owner_id, amount, hit_at) VALUES (?,?,?,?)`,
			   out.ID, out.Owner, out.Amount, out.At)
		  return err
	   })
	  return out, err
  }

  func (r *SubAcct) lockVelocityGate(ctx context.Context, owner string, now time.Time) error {
	   q := r.db.Conn(ctx)
	   var id string
	   err := q.QueryRowContext(ctx,
		   `SELECT owner_id FROM aa_subacct_velocity_gate WHERE owner_id=? FOR UPDATE`, owner,
	  ).Scan(&id)
	   if errors.Is(err, sql.ErrNoRows) {
		  ins := `INSERT INTO aa_subacct_velocity_gate (owner_id, updated_at) VALUES (?,?)`
		   if _, e2 := q.ExecContext(ctx, ins, owner, now); e2 != nil {
			  // 并发插入后重锁
			   return q.QueryRowContext(ctx,
				   `SELECT owner_id FROM aa_subacct_velocity_gate WHERE owner_id=? FOR UPDATE`, owner,
			  ).Scan(&id)
		   }
		   return q.QueryRowContext(ctx,
			  `SELECT owner_id FROM aa_subacct_velocity_gate WHERE owner_id=? FOR UPDATE`, owner,
		  ).Scan(&id)
	  }
	  return err
  }

// ReleaseVelocityLast 删除 owner 最近一条命中（失败回滚占用）。
   func (r *SubAcct) ReleaseVelocityLast(ctx context.Context, owner string) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   owner = strings.TrimSpace(owner)
	  if owner == "" {
		   return nil
	   }
	   return r.db.InTx(ctx, func(txCtx context.Context) error {
		  if err := r.lockVelocityGate(txCtx, owner, time.Now().UTC()); err != nil {
			  return err
		   }
		  var id string
		  err := r.db.Conn(txCtx).QueryRowContext(txCtx,
			   `SELECT id FROM aa_subacct_velocity_hits WHERE owner_id=? ORDER BY hit_at DESC, id DESC LIMIT 1 FOR UPDATE`,
			  owner,
		   ).Scan(&id)
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil
		   }
		  if err != nil {
			  return err
		  }
		   _, err = r.db.Conn(txCtx).ExecContext(txCtx, `DELETE FROM aa_subacct_velocity_hits WHERE id=?`, id)
		  return err
	   })
  }
