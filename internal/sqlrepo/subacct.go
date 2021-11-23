package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

type SubParent struct {
	ID string
	UserID string
	Label string
	Status    string
	Reason    string
	Fence     persist.Fence
	OpenedAt  time.Time
	UpdatedAt time.Time
}

type SubChild struct {
	ID       string
	ParentID string
	UserID   string
	Label    string
	Status string
	Reason string
	Privileges string
	Fence      persist.Fence
	OpenedAt   time.Time
	UpdatedAt time.Time
}

type SubGrant struct {
	ID string
	ChildID  string
	Actor    string
	Reviewer string
	Add      string
	Remove   string
	Assets   string
	DailyCap string
	Fence   persist.Fence
	Created time.Time
}

type SubHop struct {
	ID        string
	Idem      string
	FromOwner string
	ToOwner  string
	FromUser string
	ToUser   string
	FromBook string
	ToBook string
	Asset  string
	Amount     string
	Kind       string
	TransferID string
	CreatedAt time.Time
}

type SubAcct struct{ db *persist.SQLDB }

   func NewSubAcct(db *persist.SQLDB) *SubAcct { return &SubAcct{db: db} }

   func (r *SubAcct) PutParent(ctx context.Context, p SubParent) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	  q := `INSERT INTO aa_subacct_parents (id, user_id, label, status, reason, fence_epoch, fence_token, opened_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), label=VALUES(label), status=VALUES(status), reason=VALUES(reason), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_subacct_parents (id, user_id, label, status, reason, fence_epoch, fence_token, opened_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET user_id=EXCLUDED.user_id, label=EXCLUDED.label, status=EXCLUDED.status, reason=EXCLUDED.reason, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, p.ID, p.UserID, p.Label, p.Status, p.Reason, p.Fence.Epoch, p.Fence.Token, p.OpenedAt, p.UpdatedAt)
	  return err
   }

  func (r *SubAcct) PutChild(ctx context.Context, c SubChild) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   q := `INSERT INTO aa_subacct_children (id, parent_id, user_id, label, status, reason, privileges, fence_epoch, fence_token, opened_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE parent_id=VALUES(parent_id), user_id=VALUES(user_id), label=VALUES(label), status=VALUES(status), reason=VALUES(reason), privileges=VALUES(privileges), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_subacct_children (id, parent_id, user_id, label, status, reason, privileges, fence_epoch, fence_token, opened_at, updated_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET parent_id=EXCLUDED.parent_id, user_id=EXCLUDED.user_id, label=EXCLUDED.label, status=EXCLUDED.status, reason=EXCLUDED.reason, privileges=EXCLUDED.privileges, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, c.ID, c.ParentID, c.UserID, c.Label, c.Status, c.Reason, c.Privileges, c.Fence.Epoch, c.Fence.Token, c.OpenedAt, c.UpdatedAt)
	  return err
   }

   func (r *SubAcct) PutGrant(ctx context.Context, g SubGrant) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   if g.ID == "" {
		   g.ID = uuid.NewString()
	   }
	  if g.Created.IsZero() {
		  g.Created = time.Now().UTC()
	   }
	   cap := g.DailyCap
	   if strings.TrimSpace(cap) == "" {
		   cap = "0"
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO aa_subacct_grants (id, child_id, actor, reviewer, add_privs, remove_privs, assets, daily_cap, fence_epoch, fence_token, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		   g.ID, g.ChildID, g.Actor, g.Reviewer, g.Add, g.Remove, g.Assets, cap, g.Fence.Epoch, g.Fence.Token, g.Created)
	  return err
  }

   func (r *SubAcct) PutHop(ctx context.Context, h SubHop) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  _,
		  err := r.db.Conn(ctx).ExecContext(ctx,
		   `INSERT INTO aa_subacct_hops (id, idem, from_owner, to_owner, from_user, to_user, from_book, to_book, asset, amount, kind, transfer_id, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  h.ID, h.Idem, h.FromOwner, h.ToOwner, h.FromUser, h.ToUser, h.FromBook, h.ToBook, h.Asset, h.Amount, h.Kind, h.TransferID, h.CreatedAt)
	   if err != nil && isUnique(err) {
		   return ErrConflict
	  }
	   return err
  }

   func (r *SubAcct) GetParent(ctx context.Context, id string) (SubParent, error) {
	  if !sqlOn(r.db) {
		  return SubParent{}, ErrUnavailable
	   }
	  var p SubParent
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, user_id, label, status, reason, fence_epoch, fence_token, opened_at, updated_at FROM aa_subacct_parents WHERE id=?`,
		   strings.TrimSpace(id),
	  ).Scan(&p.ID, &p.UserID, &p.Label, &p.Status, &p.Reason, &p.Fence.Epoch, &p.Fence.Token, &p.OpenedAt, &p.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return SubParent{}, ErrNotFound
	  }
	  return p, err
  }

  func (r *SubAcct) GetChild(ctx context.Context, id string) (SubChild, error) {
	   if !sqlOn(r.db) {
		  return SubChild{}, ErrUnavailable
	   }
	   var c SubChild
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, parent_id, user_id, label, status, reason, privileges, fence_epoch, fence_token, opened_at, updated_at FROM aa_subacct_children WHERE id=?`,
		  strings.TrimSpace(id),
	   ).Scan(&c.ID, &c.ParentID, &c.UserID, &c.Label, &c.Status, &c.Reason, &c.Privileges, &c.Fence.Epoch, &c.Fence.Token, &c.OpenedAt, &c.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return SubChild{}, ErrNotFound
	   }
	   return c, err
   }
