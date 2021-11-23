package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type ApprovalRow struct {
	ID          string
	Kind        string
	MovementID  string
	Chain       string
	Dest        string
	Amount      string
	RequestedBy string
	ApprovedBy string
	State string
	Note  string
	Fence     persist.Fence
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Approvals struct{ db *persist.SQLDB }

   func NewApprovals(db *persist.SQLDB) *Approvals { return &Approvals{db: db} }

  func (r *Approvals) Put(ctx context.Context, a ApprovalRow) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   amt := a.Amount
	  if strings.TrimSpace(amt) == "" {
		   amt = "0"
	   }
	  q := `INSERT INTO aa_approvals (id, kind, movement_id, chain, dest, amount, requested_by, approved_by, state, note, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind), movement_id=VALUES(movement_id), chain=VALUES(chain), dest=VALUES(dest), amount=VALUES(amount), requested_by=VALUES(requested_by), approved_by=VALUES(approved_by), state=VALUES(state), note=VALUES(note), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_approvals (id, kind, movement_id, chain, dest, amount, requested_by, approved_by, state, note, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind, movement_id=EXCLUDED.movement_id, chain=EXCLUDED.chain, dest=EXCLUDED.dest, amount=EXCLUDED.amount, requested_by=EXCLUDED.requested_by, approved_by=EXCLUDED.approved_by, state=EXCLUDED.state, note=EXCLUDED.note, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, a.ID, a.Kind, a.MovementID, a.Chain, a.Dest, amt, a.RequestedBy, a.ApprovedBy, a.State, a.Note, a.Fence.Epoch, a.Fence.Token, a.CreatedAt, a.UpdatedAt)
	  return err
   }

   func (r *Approvals) Get(ctx context.Context, id string) (ApprovalRow, error) {
	  if !sqlOn(r.db) {
		  return ApprovalRow{}, ErrUnavailable
	  }
	   var a ApprovalRow
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, kind, movement_id, chain, dest, amount, requested_by, approved_by, state, note, fence_epoch, fence_token, created_at, updated_at FROM aa_approvals WHERE id=?`,
		   strings.TrimSpace(id),
	   ).Scan(&a.ID, &a.Kind, &a.MovementID, &a.Chain, &a.Dest, &a.Amount, &a.RequestedBy, &a.ApprovedBy, &a.State, &a.Note, &a.Fence.Epoch, &a.Fence.Token, &a.CreatedAt, &a.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return ApprovalRow{}, ErrNotFound
	   }
	   return a, err
   }
