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

type SweepSchedule struct {
	ID string
	Region   string
	Weekdays string
	StartUTC int
	EndUTC int
	MaxPlans int
	Halt  bool
	Fence persist.Fence
	Updated time.Time
}

type SweepPlanRow struct {
	ID         string
	ScheduleID string
	Asset  string
	Amount string
	Reason string
	State  string
	Fence persist.Fence
	CreatedAt time.Time
}

type Sweep struct{ db *persist.SQLDB }

   func NewSweep(db *persist.SQLDB) *Sweep { return &Sweep{db: db} }

  func (r *Sweep) PutSchedule(ctx context.Context, s SweepSchedule) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   if s.ID == "" {
		  s.ID = uuid.NewString()
	   }
	   if s.Updated.IsZero() {
		   s.Updated = time.Now().UTC()
	   }
	   halt := 0
	  if s.Halt {
		  halt = 1
	  }
	  q := `INSERT INTO aa_sweep_schedules (id, region, weekdays, start_utc, end_utc, max_plans, halt, fence_epoch, fence_token, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE weekdays=VALUES(weekdays), start_utc=VALUES(start_utc), end_utc=VALUES(end_utc), max_plans=VALUES(max_plans), halt=VALUES(halt), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_sweep_schedules (id, region, weekdays, start_utc, end_utc, max_plans, halt, fence_epoch, fence_token, updated_at) 
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET weekdays=EXCLUDED.weekdays, start_utc=EXCLUDED.start_utc, end_utc=EXCLUDED.end_utc, max_plans=EXCLUDED.max_plans, halt=EXCLUDED.halt, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, s.ID, s.Region, s.Weekdays, s.StartUTC, s.EndUTC, s.MaxPlans, halt, s.Fence.Epoch, s.Fence.Token, s.Updated)
	  return err
   }

  func (r *Sweep) PutPlan(ctx context.Context, p SweepPlanRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  if p.ID == "" {
		   p.ID = uuid.NewString()
	  }
	  if p.CreatedAt.IsZero() {
		  p.CreatedAt = time.Now().UTC()
	   }
	   if p.State == "" {
		  p.State = "planned"
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `INSERT INTO aa_sweep_plans (id, schedule_id, asset, amount, reason, state, fence_epoch, fence_token, created_at)
VALUES (?,?,?,?,?,?,?,?,?)`,
		  p.ID,
		  p.ScheduleID, p.Asset, p.Amount, p.Reason, p.State, p.Fence.Epoch, p.Fence.Token, p.CreatedAt)
	  return err
  }

  func (r *Sweep) GetSchedule(ctx context.Context, region string) (SweepSchedule, error) {
	   if !sqlOn(r.db) {
		   return SweepSchedule{}, ErrUnavailable
	   }
	  var s SweepSchedule
	  var halt int
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, region, weekdays, start_utc, end_utc, max_plans, halt, fence_epoch, fence_token, updated_at FROM aa_sweep_schedules WHERE region=?`,
		  strings.TrimSpace(region),
	   ).Scan(&s.ID, &s.Region, &s.Weekdays, &s.StartUTC, &s.EndUTC, &s.MaxPlans, &halt, &s.Fence.Epoch, &s.Fence.Token, &s.Updated)
	   if errors.Is(err, sql.ErrNoRows) {
		  return SweepSchedule{}, ErrNotFound
	  }
	   s.Halt = halt != 0
	   return s, err
   }
