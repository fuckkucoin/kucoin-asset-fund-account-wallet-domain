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

// SignJob 签章单 aa_sign_jobs。同一 movement 未结单复用，禁止并行再开一张去签另一笔。

type SignJob struct {
	db *persist.SQLDB
}

   func NewSignJob(db *persist.SQLDB) *SignJob { return &SignJob{db: db} }

type SignRow struct {
	ID         string
	MovementID string
	Chain string
	Dest   string
	Amount string
	Canonical string
	State   string
	Adapter string
	Fence persist.Fence
	CreatedAt time.Time
	UpdatedAt time.Time
}

  func (r *SignJob) Open(ctx context.Context, movementID, chain, dest, amount, canonical, adapter string) (SignRow, error) {
	  if r == nil || r.db == nil {
		  return SignRow{}, ErrUnavailable
	   }
	   if _, err := money.Parse(amount); err != nil {
		   return SignRow{}, ErrAmount
	   }
	   movementID = strings.TrimSpace(movementID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   dest = strings.TrimSpace(dest)
	   canonical = strings.TrimSpace(canonical)
	   if movementID == "" || chain == "" || dest == "" || canonical == "" {
		  return SignRow{}, ErrConflict
	  }
	  var out SignRow
	  err := r.db.InTx(ctx, func(ctx context.Context) error {
		  cur, err := r.getOpen(ctx, movementID)
		  if err == nil {
			   if cur.Canonical != canonical {
				  return ErrConflict
			  }
			  out = cur
			   return nil
		   }
		   if !errors.Is(err, ErrNotFound) {
			  return err
		  }
		   id := uuid.NewString()
		   f, err := r.db.BumpFence(ctx, "sign|"+id, persist.NewToken())
		  if err != nil {
			  return err
		   }
		   _, err = r.db.Conn(ctx).ExecContext(ctx,
			   `INSERT INTO aa_sign_jobs (id, movement_id, chain, dest, amount, canonical, state, adapter, fence_epoch, fence_token, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3))`,
			   id, movementID, chain, dest, amount, canonical, "queued", adapter, f.Epoch, f.Token)
		  if err != nil {
			  if isUnique(err) {
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

   func (r *SignJob) SetState(ctx context.Context, id, state string, epoch int64, token string) (SignRow, error) {
	   if r == nil || r.db == nil {
		  return SignRow{}, ErrUnavailable
	   }
	  id = strings.TrimSpace(id)
	   state = strings.TrimSpace(state)
	   if id == "" || state == "" {
		  return SignRow{}, ErrConflict
	   }
	   var out SignRow
	   err := r.db.InTx(ctx, func(ctx context.Context) error {
		   if err := r.db.CheckFence(ctx, "sign|"+id, epoch, token); err != nil {
			   if errors.Is(err, persist.ErrStaleFence) {
				   return ErrStale
			   }
			  return err
		   }
		   f, err := r.db.BumpFence(ctx, "sign|"+id, persist.NewToken())
		   if err != nil {
			   return err
		  }
		  res,
			  err := r.db.Conn(ctx).ExecContext(ctx,
			   `UPDATE aa_sign_jobs SET state=?, fence_epoch=?, fence_token=?, updated_at=CURRENT_TIMESTAMP(3) WHERE id=? AND fence_epoch=? AND fence_token=?`,
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

   func (r *SignJob) Get(ctx context.Context, id string) (SignRow, error) {
	  if r == nil || r.db == nil {
		   return SignRow{}, ErrUnavailable
	  }
	  return r.get(ctx,
		   strings.TrimSpace(id))
   }

  func (r *SignJob) get(ctx context.Context, id string) (SignRow, error) {
	  var row SignRow
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, movement_id, chain, dest, amount, canonical, state, adapter, fence_epoch, fence_token, created_at, updated_at FROM aa_sign_jobs WHERE id=?`,
		  id,
	  ).Scan(&row.ID, &row.MovementID, &row.Chain, &row.Dest, &row.Amount, &row.Canonical, &row.State, &row.Adapter, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt, &row.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return SignRow{}, ErrNotFound
	   }
	   return row, err
  }

   func (r *SignJob) getOpen(ctx context.Context, movementID string) (SignRow, error) {
	  var row SignRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, movement_id, chain, dest, amount, canonical, state, adapter, fence_epoch, fence_token, created_at, updated_at FROM aa_sign_jobs WHERE movement_id=? AND state IN ('queued','approved')`,
		   movementID,
	  ).Scan(&row.ID,
		  &row.MovementID, &row.Chain, &row.Dest, &row.Amount, &row.Canonical, &row.State, &row.Adapter, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt, &row.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return SignRow{}, ErrNotFound
	   }
	   return row, err
  }
