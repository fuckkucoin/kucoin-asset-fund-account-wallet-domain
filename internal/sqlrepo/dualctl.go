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

type DualRequest struct {
	ID   string
	Kind string
	Subject string
	OpenedBy string
	Need     int
	State string
	Note      string
	ExpiresAt time.Time
	Fence persist.Fence
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DualVote struct {
	ID string
	RequestID string
	Actor     string
	Allow bool
	At time.Time
}

type DualCtl struct{ db *persist.SQLDB }

  func NewDualCtl(db *persist.SQLDB) *DualCtl { return &DualCtl{db: db} }

  func (r *DualCtl) PutRequest(ctx context.Context, req DualRequest) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   q := `INSERT INTO aa_dualctl_requests (id, kind, subject, opened_by, need, state, note, expires_at, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind), subject=VALUES(subject), opened_by=VALUES(opened_by), need=VALUES(need), state=VALUES(state), note=VALUES(note), expires_at=VALUES(expires_at), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_dualctl_requests (id, kind, subject, opened_by, need, state, note, expires_at, fence_epoch, fence_token, created_at, updated_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind, subject=EXCLUDED.subject, opened_by=EXCLUDED.opened_by, need=EXCLUDED.need, state=EXCLUDED.state, note=EXCLUDED.note, expires_at=EXCLUDED.expires_at, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, req.ID, req.Kind, req.Subject, req.OpenedBy, req.Need, req.State, req.Note, req.ExpiresAt, req.Fence.Epoch, req.Fence.Token, req.CreatedAt, req.UpdatedAt)
	  return err
   }

   func (r *DualCtl) PutVote(ctx context.Context, v DualVote) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  if v.ID == "" {
		  v.ID = uuid.NewString()
	   }
	  allow := 0
	  if v.Allow {
		   allow = 1
	   }
	   q := `INSERT INTO aa_dualctl_votes (id, request_id, actor, allow, at) VALUES (?,?,?,?,?)  
ON DUPLICATE KEY UPDATE allow=VALUES(allow), at=VALUES(at)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_dualctl_votes (id, request_id, actor, allow, at) VALUES (?,?,?,?,?)
ON CONFLICT (request_id, actor) DO UPDATE SET allow=EXCLUDED.allow, at=EXCLUDED.at`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, v.ID, v.RequestID, v.Actor, allow, v.At)
	  return err
   }

  func (r *DualCtl) GetRequest(ctx context.Context, id string) (DualRequest, error) {
	  if !sqlOn(r.db) {
		   return DualRequest{}, ErrUnavailable
	   }
	  var req DualRequest
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, kind, subject, opened_by, need, state, note, expires_at, fence_epoch, fence_token, created_at, updated_at FROM aa_dualctl_requests WHERE id=?`,
		  strings.TrimSpace(id),
	   ).Scan(&req.ID, &req.Kind, &req.Subject, &req.OpenedBy, &req.Need, &req.State, &req.Note, &req.ExpiresAt, &req.Fence.Epoch, &req.Fence.Token, &req.CreatedAt, &req.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return DualRequest{},
			  ErrNotFound
	   }
	   return req, err
   }

  func (r *DualCtl) ListVotes(ctx context.Context, requestID string) ([]DualVote, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT id, request_id, actor, allow, at FROM aa_dualctl_votes WHERE request_id=? ORDER BY at`,
		  strings.TrimSpace(requestID))
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  out := make([]DualVote, 0)
	   for rows.Next() {
		   var v DualVote
		  var allow int
		  if err := rows.Scan(&v.ID, &v.RequestID, &v.Actor, &allow, &v.At); err != nil {
			   return nil, err
		  }
		  v.Allow = allow != 0
		  out = append(out, v)
	  }
	   return out, rows.Err()
  }
