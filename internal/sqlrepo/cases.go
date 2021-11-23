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

type CaseRow struct {
	ID         string
	MovementID string
	UserID string
	Kind   string
	Reason string
	State  string
	Assignee  string
	DecidedBy string
	PolicyVer int
	Fence     persist.Fence
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CaseAuditRow struct {
	ID     string
	CaseID string
	Actor  string
	Event  string
	From   string
	To string
	Note string
	At   time.Time
}

type Cases struct{ db *persist.SQLDB }

   func NewCases(db *persist.SQLDB) *Cases { return &Cases{db: db} }

   func (r *Cases) Put(ctx context.Context, c CaseRow) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  q := `INSERT INTO aa_cases (id, movement_id, user_id, kind, reason, state, assignee, decided_by, policy_ver, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE movement_id=VALUES(movement_id), user_id=VALUES(user_id), kind=VALUES(kind), reason=VALUES(reason), state=VALUES(state), assignee=VALUES(assignee), decided_by=VALUES(decided_by), policy_ver=VALUES(policy_ver), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_cases (id, movement_id, user_id, kind, reason, state, assignee, decided_by, policy_ver, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET movement_id=EXCLUDED.movement_id, user_id=EXCLUDED.user_id, kind=EXCLUDED.kind, reason=EXCLUDED.reason, state=EXCLUDED.state, assignee=EXCLUDED.assignee, decided_by=EXCLUDED.decided_by, policy_ver=EXCLUDED.policy_ver, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, c.ID, c.MovementID, c.UserID, c.Kind, c.Reason, c.State, c.Assignee, c.DecidedBy, c.PolicyVer, c.Fence.Epoch, c.Fence.Token, c.CreatedAt, c.UpdatedAt)
	   return err
   }

   func (r *Cases) PutAudit(ctx context.Context, a CaseAuditRow) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   if a.ID == "" {
		   a.ID = uuid.NewString()
	  }
	  if a.At.IsZero() {
		   a.At = time.Now().UTC()
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `INSERT INTO aa_case_audit (id, case_id, actor, event, from_state, to_state, note, at) VALUES (?,?,?,?,?,?,?,?)`,
		  a.ID, a.CaseID, a.Actor, a.Event, a.From, a.To, a.Note, a.At)
	  return err
  }

   func (r *Cases) GetByMovement(ctx context.Context, movementID string) (CaseRow, error) {
	   if !sqlOn(r.db) {
		  return CaseRow{}, ErrUnavailable
	   }
	  var c CaseRow
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, movement_id, user_id, kind, reason, state, assignee, decided_by, policy_ver, fence_epoch, fence_token, created_at, updated_at FROM aa_cases WHERE movement_id=? ORDER BY updated_at DESC LIMIT 1`,
		   strings.TrimSpace(movementID),
	   ).Scan(&c.ID, &c.MovementID, &c.UserID, &c.Kind, &c.Reason, &c.State, &c.Assignee, &c.DecidedBy, &c.PolicyVer, &c.Fence.Epoch, &c.Fence.Token, &c.CreatedAt, &c.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return CaseRow{}, ErrNotFound
	  }
	   return c, err
  }

  func (r *Cases) ListAudit(ctx context.Context, caseID string) ([]CaseAuditRow, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT id, case_id, actor, event, from_state, to_state, note, at FROM aa_case_audit WHERE case_id=? ORDER BY at`,
		  strings.TrimSpace(caseID))
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := make([]CaseAuditRow, 0)
	   for rows.Next() {
		   var a CaseAuditRow
		  if err := rows.Scan(&a.ID, &a.CaseID, &a.Actor, &a.Event, &a.From, &a.To, &a.Note, &a.At); err != nil {
			  return nil, err
		   }
		   out = append(out, a)
	   }
	   return out, rows.Err()
   }
