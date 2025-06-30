package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// Withdraw 出金登记 aa_withdraws。同一 idem 必须回放原单，禁止再冻一笔。

type Withdraw struct {
	db *persist.SQLDB
}

   func NewWithdraw(db *persist.SQLDB) *Withdraw { return &Withdraw{db: db} }

type WithdrawRow struct {
	ID string
	UserID string
	Asset string
	Amount string
	Chain  string
	Dest   string
	Idem   string
	State string
	Fence persist.Fence
	CreatedAt time.Time
}

   func (r *Withdraw) Open(ctx context.Context, userID, asset, amount, chain, dest, idem, state string) (WithdrawRow, error) {
	  if r == nil || r.db == nil {
		  return WithdrawRow{}, ErrUnavailable
	   }
	   if _, err := money.Parse(amount); err != nil {
		   return WithdrawRow{}, ErrAmount
	   }
	   userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   dest = strings.TrimSpace(dest)
	   idem = strings.TrimSpace(idem)
	   if state == "" {
		   state = "frozen"
	  }
	   if userID == "" || asset == "" || chain == "" || dest == "" || idem == "" {
		  return WithdrawRow{}, ErrConflict
	   }
	   var out WithdrawRow
	   err := r.db.InTx(ctx, func(ctx context.Context) error {
		  cur, err := r.getByIdem(ctx, idem)
		  if err == nil {
			  if cur.UserID != userID || cur.Asset != asset || cur.Amount != amount || cur.Chain != chain || cur.Dest != dest {
				  return ErrConflict
			   }
			  out = cur
			   return nil
		   }
		  if !errors.Is(err, ErrNotFound) {
			   return err
		   }
		  id := uuid.NewString()
		  f, err := r.db.BumpFence(ctx, "wd|"+id, persist.NewToken())
		   if err != nil {
			  return err
		   }
		  _, err = r.db.Conn(ctx).ExecContext(ctx,
			   `INSERT INTO aa_withdraws (id, user_id, asset, amount, chain, dest, idem, state, fence_epoch, fence_token, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP(3))`,
			   id, userID, asset, amount, chain, dest, idem, state, f.Epoch, f.Token)
		  if err != nil {
			   if isUnique(err) {
				   got, gerr := r.getByIdem(ctx, idem)
				  if gerr == nil {
					   out = got
					  return nil
				  }
				   return ErrConflict
			   }
			  return err
		   }
		   got, err := r.get(ctx, id)
		  if err != nil {
			  return err
		   }
		   out = got
		   return nil
	  })
	   return out, err
   }

   func (r *Withdraw) SetState(ctx context.Context, id, state string, epoch int64, token string) (WithdrawRow, error) {
	   if r == nil || r.db == nil {
		   return WithdrawRow{}, ErrUnavailable
	   }
	   id = strings.TrimSpace(id)
	  state = strings.TrimSpace(state)
	  if id == "" || state == "" {
		  return WithdrawRow{}, ErrConflict
	  }
	   var out WithdrawRow
	   err := r.db.InTx(ctx, func(ctx context.Context) error {
		   if err := r.db.CheckFence(ctx, "wd|"+id, epoch, token); err != nil {
			  if errors.Is(err, persist.ErrStaleFence) {
				   return ErrStale
			  }
			  return err
		  }
		  f, err := r.db.BumpFence(ctx, "wd|"+id, persist.NewToken())
		   if err != nil {
			   return err
		   }
		   res, err := r.db.Conn(ctx).ExecContext(ctx,
			   `UPDATE aa_withdraws SET state=?, fence_epoch=?, fence_token=? WHERE id=? AND fence_epoch=? AND fence_token=?`,
			  state, f.Epoch, f.Token, id, epoch, token)
		   if err != nil {
			  return err
		  }
		   n, _ := res.RowsAffected()
		  if n == 0 {
			  return ErrStale
		   }
		   got, err := r.get(ctx, id)
		   if err != nil {
			  return err
		   }
		  out = got
		   return nil
	   })
	   return out, err
  }

  func (r *Withdraw) Get(ctx context.Context, id string) (WithdrawRow, error) {
	  if r == nil || r.db == nil {
		   return WithdrawRow{}, ErrUnavailable
	   }
	  return r.get(ctx, strings.TrimSpace(id))
   }

   func (r *Withdraw) GetByIdem(ctx context.Context, idem string) (WithdrawRow, error) {
	  if r == nil || r.db == nil {
		  return WithdrawRow{}, ErrUnavailable
	  }
	   return r.getByIdem(ctx,
		  strings.TrimSpace(idem))
  }

  func (r *Withdraw) get(ctx context.Context, id string) (WithdrawRow, error) {
	  var row WithdrawRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, user_id, asset, amount, chain, dest, idem, state, fence_epoch, fence_token, created_at FROM aa_withdraws WHERE id=?`,
		   id,
	  ).Scan(&row.ID, &row.UserID, &row.Asset, &row.Amount, &row.Chain, &row.Dest, &row.Idem, &row.State, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return WithdrawRow{}, ErrNotFound
	  }
	   return row, err
  }

   func (r *Withdraw) getByIdem(ctx context.Context, idem string) (WithdrawRow, error) {
	   var row WithdrawRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, user_id, asset, amount, chain, dest, idem, state, fence_epoch, fence_token, created_at FROM aa_withdraws WHERE idem=?`,
		  idem,
	  ).Scan(&row.ID,
		   &row.UserID, &row.Asset, &row.Amount, &row.Chain, &row.Dest, &row.Idem, &row.State, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return WithdrawRow{}, ErrNotFound
	   }
	  return row, err
  }
