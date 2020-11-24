package swapthaw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────
// SQLPersist：SQL 持久化层（封装 sql.DB 句柄）
// ──────────────────────────────────────────────
// Queryer 最小化数据库操作接口，兼容 *sql.DB 和 *sql.Tx。

type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// SQLLeaseStore FreezeLease SQL 持久化实现。

type SQLLeaseStore struct {
	db      Queryer
	dialect string // "mysql" | "postgres"
}

// NewSQLLeaseStore 创建 FreezeLease SQL 存储。dialect 为 "mysql" 或 "postgres"。

  func NewSQLLeaseStore(db Queryer, dialect string) *SQLLeaseStore {
	   return &SQLLeaseStore{db: db, dialect: strings.ToLower(dialect)}
   }

  func (s *SQLLeaseStore) SaveLease(lease *FreezeLease) error {
	   expiresAt := sql.NullTime{}
	  if !lease.ExpiresAt.IsZero() {
		   expiresAt = sql.NullTime{Time: lease.ExpiresAt, Valid: true}
	  }
	   winnerLeg := sql.NullString{}
	  if lease.WinnerLeg != "" {
		   winnerLeg = sql.NullString{String: string(lease.WinnerLeg), Valid: true}
	  }
	   _,
		   err := s.db.ExecContext(context.Background(),
		   `INSERT INTO sw_freeze_leases 
		(id, order_id, user_id, asset, frozen_amount, generation, status, expires_at, winner_leg, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		   lease.ID, lease.OrderID, lease.UserID, lease.Asset, lease.FrozenAmount,
		   lease.Generation, string(lease.Status), expiresAt, winnerLeg,
		   lease.CreatedAt, lease.UpdatedAt,
	  )
	  return err
  }

  func (s *SQLLeaseStore) UpdateLease(lease *FreezeLease) error {
	  winnerLeg := sql.NullString{}
	   if lease.WinnerLeg != "" {
		  winnerLeg = sql.NullString{String: string(lease.WinnerLeg), Valid: true}
	   }
	  res, err := s.db.ExecContext(context.Background(),
		  `UPDATE sw_freeze_leases SET status=?, winner_leg=?, updated_at=? WHERE id=?`,
		   string(lease.Status), winnerLeg, lease.UpdatedAt, lease.ID,
	   )
	  if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrLeaseNotFound
	  }
	  return nil
  }

  func (s *SQLLeaseStore) GetLease(id string) (*FreezeLease, error) {
	   row := s.db.QueryRowContext(context.Background(),
		   `SELECT id, order_id, user_id, asset, frozen_amount, generation, status, expires_at, winner_leg, created_at, updated_at 
		FROM sw_freeze_leases WHERE id=?`, id)
	  return scanLease(row)
   }

  func (s *SQLLeaseStore) GetLeaseByOrder(orderID string) (*FreezeLease, error) {
	   row := s.db.QueryRowContext(context.Background(),
		  `SELECT id, order_id, user_id, asset, frozen_amount, generation, status, expires_at, winner_leg, created_at, updated_at
		FROM sw_freeze_leases WHERE order_id=? AND status='open' ORDER BY created_at DESC LIMIT 1`, orderID)
	   return scanLease(row)
  }

  func (s *SQLLeaseStore) ListExpiredOpen(limit int, before time.Time) ([]*FreezeLease, error) {
	   rows, err := s.db.QueryContext(context.Background(),
		  `SELECT id, order_id, user_id, asset, frozen_amount, generation, status, expires_at, winner_leg, created_at, updated_at
		FROM sw_freeze_leases WHERE status='open' AND expires_at IS NOT NULL AND expires_at < ? LIMIT ?`,
		  before, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanLeaseRows(rows)
   }

   func scanLease(row *sql.Row) (*FreezeLease, error) {
	   l := &FreezeLease{}
	   var expiresAt sql.NullTime
	   var winnerLeg sql.NullString
	   var status string
	   err := row.Scan(&l.ID,
		  &l.OrderID, &l.UserID, &l.Asset, &l.FrozenAmount,
		  &l.Generation, &status, &expiresAt, &winnerLeg, &l.CreatedAt, &l.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrLeaseNotFound
	   }
	  if err != nil {
		   return nil, err
	   }
	   l.Status = LeaseStatus(status)
	   if expiresAt.Valid {
		   l.ExpiresAt = expiresAt.Time
	  }
	   if winnerLeg.Valid {
		  l.WinnerLeg = BindLeg(winnerLeg.String)
	   }
	   return l, nil
   }

   func scanLeaseRows(rows *sql.Rows) ([]*FreezeLease, error) {
	   var out []*FreezeLease
	  for rows.Next() {
		  l := &FreezeLease{}
		   var expiresAt sql.NullTime
		  var winnerLeg sql.NullString
		  var status string
		  if err := rows.Scan(&l.ID, &l.OrderID, &l.UserID, &l.Asset, &l.FrozenAmount,
			   &l.Generation, &status, &expiresAt, &winnerLeg, &l.CreatedAt, &l.UpdatedAt); err != nil {
			  return nil, err
		  }
		  l.Status = LeaseStatus(status)
		  if expiresAt.Valid {
			   l.ExpiresAt = expiresAt.Time
		  }
		  if winnerLeg.Valid {
			   l.WinnerLeg = BindLeg(winnerLeg.String)
		   }
		  out = append(out, l)
	  }
	   return out, rows.Err()
   }

// SQLBindStore SettleBind SQL 持久化实现。
type SQLBindStore struct {
	db      Queryer
	dialect string
}

// NewSQLBindStore 创建 SettleBind SQL 存储。
  func NewSQLBindStore(db Queryer, dialect string) *SQLBindStore {
	  return &SQLBindStore{db: db, dialect: strings.ToLower(dialect)}
   }

  func (s *SQLBindStore) InsertBindOnce(bind *SettleBind) error {
	   var q string
	   if s.dialect == "postgres" {
		  q = `INSERT INTO sw_settle_binds (id, order_id, lease_id, leg, actor, bound_at)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (order_id) DO NOTHING`
	  } else {
		   q = `INSERT IGNORE INTO sw_settle_binds (id, order_id, lease_id, leg, actor, bound_at)
			VALUES (?,?,?,?,?,?)`
	  }
	  res, err := s.db.ExecContext(context.Background(), q,
		   bind.ID, bind.OrderID, bind.LeaseID, string(bind.Leg), bind.Actor, bind.BoundAt)
	  if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   // 已存在记录，需要检查是否冲突。
		   existing, err := s.GetBind(bind.OrderID)
		   if err != nil {
			   return err
		   }
		  if existing.Leg != bind.Leg {
			  return ErrSettleBindConflict
		  }
		  return nil // 幂等
	   }
	  return nil
   }

   func (s *SQLBindStore) GetBind(orderID string) (*SettleBind, error) {
	   row := s.db.QueryRowContext(context.Background(),
		  `SELECT id, order_id, lease_id, leg, actor, bound_at FROM sw_settle_binds WHERE order_id=?`, orderID)
	  b := &SettleBind{}
	   var actor sql.NullString
	   var leg string
	   err := row.Scan(&b.ID, &b.OrderID, &b.LeaseID, &leg, &actor, &b.BoundAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrSettleBindNotFound
	   }
	   if err != nil {
		   return nil, err
	  }
	   b.Leg = BindLeg(leg)
	   if actor.Valid {
		   b.Actor = actor.String
	   }
	   return b, nil
   }

// SQLThawCaseStore ThawCase SQL 持久化实现。
type SQLThawCaseStore struct {
	db      Queryer
	dialect string
}

// NewSQLThawCaseStore 创建 ThawCase SQL 存储。

   func NewSQLThawCaseStore(db Queryer, dialect string) *SQLThawCaseStore {
	  return &SQLThawCaseStore{db: db, dialect: strings.ToLower(dialect)}
   }

const thawCaseSelect = `SELECT id, order_id, user_id, lease_id, asset, kind, status,
	detected_amount, frozen_snapshot, available_snapshot, proof_payload, approver,
	detected_at, proven_at, closed_at, updated_at FROM sw_thaw_cases`

  func (s *SQLThawCaseStore) SaveCase(tc *ThawCase) error {
	  _, err := s.db.ExecContext(context.Background(),
		   `INSERT INTO sw_thaw_cases
		(id, order_id, user_id, lease_id, asset, kind, status,
		detected_amount, frozen_snapshot, available_snapshot, detected_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		  tc.ID, tc.OrderID, tc.UserID, tc.LeaseID, tc.Asset, string(tc.Kind),
		  string(tc.Status), tc.DetectedAmount, tc.FrozenSnapshot, tc.AvailableSnapshot,
		   tc.DetectedAt, tc.UpdatedAt,
	   )
	  return err
  }

  func (s *SQLThawCaseStore) UpdateCase(tc *ThawCase) error {
	  proofPayload := sql.NullString{}
	  if tc.ProofPayload != "" {
		   proofPayload = sql.NullString{String: tc.ProofPayload, Valid: true}
	  }
	  approver := sql.NullString{}
	  if tc.Approver != "" {
		   approver = sql.NullString{String: tc.Approver, Valid: true}
	  }
	  var provenAt,
		   closedAt sql.NullTime
	   if tc.ProvenAt != nil {
		   provenAt = sql.NullTime{Time: *tc.ProvenAt, Valid: true}
	   }
	   if tc.ClosedAt != nil {
		   closedAt = sql.NullTime{Time: *tc.ClosedAt, Valid: true}
	   }
	  res, err := s.db.ExecContext(context.Background(),
		   `UPDATE sw_thaw_cases SET status=?, proof_payload=?, approver=?, proven_at=?, closed_at=?, updated_at=? WHERE id=?`,
		  string(tc.Status),
		   proofPayload, approver, provenAt, closedAt, tc.UpdatedAt, tc.ID,
	   )
	  if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrThawCaseNotFound
	  }
	   return nil
  }

  func (s *SQLThawCaseStore) GetCase(id string) (*ThawCase, error) {
	  row := s.db.QueryRowContext(context.Background(), thawCaseSelect+` WHERE id=?`, id)
	  return scanThawCase(row)
   }

   func (s *SQLThawCaseStore) GetCaseByOrder(orderID string) (*ThawCase, error) {
	  row := s.db.QueryRowContext(context.Background(),
		  thawCaseSelect+` WHERE order_id=? ORDER BY detected_at DESC LIMIT 1`, orderID)
	  return scanThawCase(row)
   }

   func (s *SQLThawCaseStore) ListOpenByUser(userID string, limit int) ([]*ThawCase, error) {
	  rows, err := s.db.QueryContext(context.Background(),
		  thawCaseSelect+` WHERE user_id=? AND status IN ('open','proven') ORDER BY detected_at DESC LIMIT ?`,
		  userID, limit)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanThawCaseRows(rows)
  }

   func (s *SQLThawCaseStore) ListOpenCases(limit int) ([]*ThawCase, error) {
	  rows,
		   err := s.db.QueryContext(context.Background(),
		   thawCaseSelect+` WHERE status IN ('open','proven') ORDER BY detected_at ASC LIMIT ?`, limit)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  return scanThawCaseRows(rows)
   }

   func scanThawCase(row *sql.Row) (*ThawCase, error) {
	   tc := &ThawCase{}
	   var proofPayload, approver sql.NullString
	  var leaseID sql.NullString
	  var provenAt, closedAt sql.NullTime
	  var kind, status string
	  err := row.Scan(&tc.ID, &tc.OrderID, &tc.UserID, &leaseID, &tc.Asset, &kind, &status,
		   &tc.DetectedAmount,
		  &tc.FrozenSnapshot, &tc.AvailableSnapshot, &proofPayload, &approver,
		   &tc.DetectedAt, &provenAt, &closedAt, &tc.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrThawCaseNotFound
	   }
	   if err != nil {
		  return nil, err
	  }
	  tc.Kind = ThawCaseKind(kind)
	   tc.Status = ThawCaseStatus(status)
	  if leaseID.Valid {
		  tc.LeaseID = leaseID.String
	   }
	  if proofPayload.Valid {
		   tc.ProofPayload = proofPayload.String
	  }
	  if approver.Valid {
		  tc.Approver = approver.String
	  }
	  if provenAt.Valid {
		  tc.ProvenAt = &provenAt.Time
	   }
	  if closedAt.Valid {
		  tc.ClosedAt = &closedAt.Time
	  }
	  return tc, nil
   }

   func scanThawCaseRows(rows *sql.Rows) ([]*ThawCase, error) {
	  var out []*ThawCase
	   for rows.Next() {
		  tc := &ThawCase{}
		   var proofPayload,
			  approver sql.NullString
		  var leaseID sql.NullString
		   var provenAt, closedAt sql.NullTime
		  var kind, status string
		  if err := rows.Scan(&tc.ID,
			   &tc.OrderID, &tc.UserID, &leaseID, &tc.Asset, &kind, &status,
			  &tc.DetectedAmount, &tc.FrozenSnapshot, &tc.AvailableSnapshot, &proofPayload, &approver,
			  &tc.DetectedAt, &provenAt, &closedAt, &tc.UpdatedAt); err != nil {
			   return nil, err
		  }
		   tc.Kind = ThawCaseKind(kind)
		  tc.Status = ThawCaseStatus(status)
		   if leaseID.Valid {
			   tc.LeaseID = leaseID.String
		   }
		  if proofPayload.Valid {
			  tc.ProofPayload = proofPayload.String
		   }
		  if approver.Valid {
			   tc.Approver = approver.String
		   }
		  if provenAt.Valid {
			   tc.ProvenAt = &provenAt.Time
		   }
		  if closedAt.Valid {
			  tc.ClosedAt = &closedAt.Time
		   }
		  out = append(out, tc)
	  }
	   return out, rows.Err()
   }

// SQLGenerationStore FreezeGeneration SQL 持久化实现。
type SQLGenerationStore struct {
	db      Queryer
	dialect string
	mu      sync.Mutex
}

// NewSQLGenerationStore 创建 FreezeGeneration SQL 存储。

   func NewSQLGenerationStore(db Queryer, dialect string) *SQLGenerationStore {
	  return &SQLGenerationStore{db: db, dialect: strings.ToLower(dialect)}
   }

  func (s *SQLGenerationStore) GetGeneration(orderID string) (*FreezeGeneration, error) {
	   row := s.db.QueryRowContext(context.Background(),
		  `SELECT order_id, user_id, generation, active_lease_id, updated_at
		FROM sw_freeze_generations WHERE order_id=?`, orderID)
	   g := &FreezeGeneration{}
	  var activeLeaseID sql.NullString
	   err := row.Scan(&g.OrderID, &g.UserID, &g.Generation, &activeLeaseID, &g.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return &FreezeGeneration{OrderID: orderID, Generation: 0}, nil
	   }
	   if err != nil {
		  return nil, err
	   }
	   if activeLeaseID.Valid {
		   g.ActiveLeaseID = activeLeaseID.String
	   }
	  return g, nil
   }

  func (s *SQLGenerationStore) CASGeneration(orderID string, expected, next int64, userID, activeLeaseID string) error {
	  var res sql.Result
	  var err error
	  if expected == 0 {
		  // 首次插入，使用 INSERT IGNORE / ON CONFLICT DO NOTHING。
		   if s.dialect == "postgres" {
			   res, err = s.db.ExecContext(context.Background(),
				  `INSERT INTO sw_freeze_generations (order_id, user_id, generation, active_lease_id, updated_at)
				VALUES ($1,$2,$3,$4,$5) ON CONFLICT (order_id) DO NOTHING`,
				   orderID, userID, next, activeLeaseID, time.Now().UTC())
		   } else {
			  res, err = s.db.ExecContext(context.Background(),
				  `INSERT IGNORE INTO sw_freeze_generations (order_id, user_id, generation, active_lease_id, updated_at)
				VALUES (?,?,?,?,?)`,
				   orderID, userID, next, activeLeaseID, time.Now().UTC())
		   }
		   if err != nil {
			   return err
		  }
		   n, _ := res.RowsAffected()
		  if n == 0 {
			  return fmt.Errorf("%w: first-time insert conflict for order %s", ErrGenerationCAS, orderID)
		  }
		   return nil
	   }
	   res, err = s.db.ExecContext(context.Background(),
		   `UPDATE sw_freeze_generations SET generation=?, active_lease_id=?, updated_at=?
		WHERE order_id=? AND generation=?`,
		  next, activeLeaseID, time.Now().UTC(), orderID, expected)
	  if err != nil {
		   return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return fmt.Errorf("%w: expected=%d for order %s", ErrGenerationCAS, expected, orderID)
	  }
	   return nil
   }

   func (s *SQLGenerationStore) ClearActiveLease(orderID string) error {
	   _, err := s.db.ExecContext(context.Background(),
		  `UPDATE sw_freeze_generations SET active_lease_id=NULL, updated_at=? WHERE order_id=?`,
		   time.Now().UTC(), orderID)
	   return err
   }

   func (s *SQLGenerationStore) ListOpenThawCasesByUser(userID string) ([]string, error) {
	   rows, err := s.db.QueryContext(context.Background(),
		  `SELECT id FROM sw_thaw_cases WHERE user_id=? AND status IN ('open','proven')`, userID)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []string
	  for rows.Next() {
		   var id string
		   if err := rows.Scan(&id); err != nil {
			   return nil, err
		  }
		  out = append(out, id)
	  }
	   return out, rows.Err()
   }
