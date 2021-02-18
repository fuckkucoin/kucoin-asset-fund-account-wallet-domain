package availcas

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"github.com/shopspring/decimal"
)

// Store 持久化接口，便于用内存实现替换。
type Store interface {
	// ── 租约 ────────────────────────────────────────────────────────────────
	// SaveLease 写入新租约（INSERT）。
	SaveLease(ctx context.Context, lease *AvailLease) error
	// GetLease 按 ID 查询租约。
	GetLease(ctx context.Context, id string) (*AvailLease, error)
	// UpdateLeaseState 更新租约状态与 updated_at。
	UpdateLeaseState(ctx context.Context,
		id, state string, now time.Time) error
	// ListExpirableLeases 查询 state=open 且 expires_at ≤ now 的租约（用于回收 worker）。
	ListExpirableLeases(ctx context.Context,
		now time.Time, limit int) ([]*AvailLease, error)
	// MaxLeaseGen 查询指定 userID×asset×book 的最大 Gen（无记录返回 0）。
	MaxLeaseGen(ctx context.Context, userID, asset, book string) (int64, error)
	// ── 意图 ────────────────────────────────────────────────────────────────
	// SaveIntent 写入意图（INSERT）。
	SaveIntent(ctx context.Context, intent *DebitIntent) error
	// GetIntent 按 ID 查询意图。
	GetIntent(ctx context.Context, id string) (*DebitIntent, error)
	// FindIntentByIdem 按幂等键查找意图（not found 返回 nil, nil）。
	FindIntentByIdem(ctx context.Context,
		idemKey string) (*DebitIntent, error)
	// FindAcceptedIntentByLease 查找同租约已接受的意图（胜者）。
	FindAcceptedIntentByLease(ctx context.Context, leaseID string) (*DebitIntent, error)
	// SumInFlightIntents 查询 userID×asset×book 已接受但尚未完成的意图扣减总额。
	SumInFlightIntents(ctx context.Context, userID, asset, book string) (decimal.Decimal, error)
	// ── 竞态证明 ─────────────────────────────────────────────────────────────
	// SaveProof 写入竞态证明（INSERT）。
	SaveProof(ctx context.Context,
		proof *RaceProof) error
	// GetProof 按 ID 查询竞态证明。
	GetProof(ctx context.Context,
		id string) (*RaceProof, error)
	// ListProofs 列出用户×资产的竞态证明（最近 limit 条）。
	ListProofs(ctx context.Context, userID, asset string, limit int) ([]*RaceProof, error)
}

// SQLStore Store 的 MySQL 实现。

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 构造 SQLStore；db 不得为 nil。
  func NewSQLStore(db *sql.DB) *SQLStore {
	   return &SQLStore{db: db}
   }

// ────────────────────────────────────────────────────────────────────────────
// AvailLease CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveLease 写入新租约。

  func (s *SQLStore) SaveLease(ctx context.Context, l *AvailLease) error {
	   if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	   }
	  q := `INSERT INTO avail_leases
		(id, user_id, asset, book, gen,
		 balance, frozen, reserved, in_flight, available,
		 state, expires_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _,
		   err := s.db.ExecContext(ctx, q,
		  l.ID, l.UserID, l.Asset, l.Book, l.Gen,
		  l.Balance.String(), l.Frozen.String(), l.Reserved.String(),
		   l.InFlight.String(),
		   l.Available.String(),
		   string(l.State), l.ExpiresAt, l.CreatedAt, l.UpdatedAt,
	  )
	   if err != nil {
		   return fmt.Errorf("availcas save lease: %w", err)
	  }
	  return nil
   }

// GetLease 按 ID 查询租约。

  func (s *SQLStore) GetLease(ctx context.Context, id string) (*AvailLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	   q := `SELECT id, user_id, asset, book, gen,
	             balance, frozen, reserved, in_flight, available,
	             state, expires_at, created_at, updated_at
	      FROM avail_leases WHERE id = ? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, id)
	  return scanLease(row)
   }

// UpdateLeaseState 更新租约状态。
   func (s *SQLStore) UpdateLeaseState(ctx context.Context, id, state string, now time.Time) error {
	   if s == nil || s.db == nil {
		  return ErrStoreUnavailable
	  }
	  q := `UPDATE avail_leases SET state = ?, updated_at = ? WHERE id = ?`
	   res, err := s.db.ExecContext(ctx, q, state, now, id)
	   if err != nil {
		   return fmt.Errorf("availcas update lease state: %w", err)
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrLeaseNotFound
	  }
	   return nil
  }

// ListExpirableLeases 查询待回收租约。

   func (s *SQLStore) ListExpirableLeases(ctx context.Context, now time.Time, limit int) ([]*AvailLease, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	  q := `SELECT id, user_id, asset, book, gen,
	             balance, frozen, reserved, in_flight, available,
	             state, expires_at, created_at, updated_at
	      FROM avail_leases
	      WHERE state = 'open' AND expires_at <= ?
	      ORDER BY expires_at ASC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, now, limit)
	   if err != nil {
		   return nil, fmt.Errorf("availcas list expirable: %w", err)
	   }
	   defer rows.Close()
	   return scanLeases(rows)
  }

// MaxLeaseGen 返回指定用户×资产×账本位的最大 Gen（无记录返回 0）。
   func (s *SQLStore) MaxLeaseGen(ctx context.Context, userID, asset, book string) (int64, error) {
	  if s == nil || s.db == nil {
		  return 0, ErrStoreUnavailable
	  }
	   var maxGen sql.NullInt64
	  q := `SELECT MAX(gen) FROM avail_leases WHERE user_id = ? AND asset = ? AND book = ?`
	  if err := s.db.QueryRowContext(ctx, q, userID, asset, book).Scan(&maxGen); err != nil {
		   return 0, fmt.Errorf("availcas max gen: %w", err)
	  }
	   if !maxGen.Valid {
		  return 0, nil
	  }
	   return maxGen.Int64, nil
   }

// ────────────────────────────────────────────────────────────────────────────
// DebitIntent CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveIntent 写入意图（尝试幂等写入）。
   func (s *SQLStore) SaveIntent(ctx context.Context, d *DebitIntent) error {
	  if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	   }
	  decidedAt := (*time.Time)(nil)
	  if d.DecidedAt != nil {
		  t := *d.DecidedAt
		   decidedAt = &t
	   }
	  q := `INSERT INTO debit_intents 
		(id, idem_key, user_id, asset, book,
		 amount, kind, reason,
		 lease_id, lease_gen, expected_avail,
		 state, reject_code, reject_note, race_proof_id,
		 actor_id, request_ip,
		 proposed_at, decided_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _,
		  err := s.db.ExecContext(ctx, q,
		   d.ID, d.IdemKey, d.UserID, d.Asset, d.Book,
		   d.Amount.String(), d.Kind, d.Reason,
		  d.LeaseID, d.LeaseGen, d.ExpectedAvail.String(),
		   string(d.State), string(d.RejectCode), d.RejectNote, d.RaceProofID,
		  d.ActorID, d.RequestIP,
		  d.ProposedAt, decidedAt,
	  )
	   if err != nil {
		   return fmt.Errorf("availcas save intent: %w", err)
	  }
	  return nil
  }

// GetIntent 按 ID 查询意图。
   func (s *SQLStore) GetIntent(ctx context.Context, id string) (*DebitIntent, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	   q := `SELECT id, idem_key, user_id, asset, book,
	             amount, kind, reason,
	             lease_id, lease_gen, expected_avail,
	             state, reject_code, reject_note, race_proof_id,
	             actor_id, request_ip, proposed_at, decided_at
	      FROM debit_intents WHERE id = ? LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, id)
	   return scanIntent(row)
  }

// FindIntentByIdem 按幂等键查找。

  func (s *SQLStore) FindIntentByIdem(ctx context.Context, idemKey string) (*DebitIntent, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	  q := `SELECT id, idem_key, user_id, asset, book,
	             amount, kind, reason,
	             lease_id, lease_gen, expected_avail,
	             state, reject_code, reject_note, race_proof_id,
	             actor_id, request_ip, proposed_at, decided_at
	      FROM debit_intents WHERE idem_key = ? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, idemKey)
	  d, err := scanIntent(row)
	   if err == sql.ErrNoRows {
		  return nil, nil
	   }
	   return d, err
   }

// FindAcceptedIntentByLease 查找同租约已接受的意图（CAS 胜者）。
   func (s *SQLStore) FindAcceptedIntentByLease(ctx context.Context, leaseID string) (*DebitIntent, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  q := `SELECT id, idem_key, user_id, asset, book,
	             amount, kind, reason,
	             lease_id, lease_gen, expected_avail,
	             state, reject_code, reject_note, race_proof_id,
	             actor_id, request_ip, proposed_at, decided_at
	      FROM debit_intents
	      WHERE lease_id = ? AND state = 'accepted'
	      ORDER BY proposed_at ASC LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, leaseID)
	   d, err := scanIntent(row)
	   if err == sql.ErrNoRows {
		  return nil, nil
	   }
	   return d, err
  }

// SumInFlightIntents 查询已接受但尚未完成的意图扣减总额。
   func (s *SQLStore) SumInFlightIntents(ctx context.Context, userID, asset, book string) (decimal.Decimal, error) {
	  if s == nil || s.db == nil {
		  return decimal.Zero, ErrStoreUnavailable
	  }
	   var sumStr sql.NullString
	   q := `SELECT SUM(amount) FROM debit_intents 
	      WHERE user_id = ? AND asset = ? AND book = ? AND state = 'accepted'`
	   if err := s.db.QueryRowContext(ctx, q, userID, asset, book).Scan(&sumStr); err != nil {
		  return decimal.Zero, fmt.Errorf("availcas sum inflight: %w", err)
	   }
	  if !sumStr.Valid || sumStr.String == "" || sumStr.String == "0" {
		  return decimal.Zero, nil
	  }
	   v, err := decimal.NewFromString(sumStr.String)
	   if err != nil {
		   return decimal.Zero, fmt.Errorf("availcas sum inflight parse: %w", err)
	   }
	  return v, nil
   }

// ────────────────────────────────────────────────────────────────────────────
// RaceProof CRUD
// ────────────────────────────────────────────────────────────────────────────
// SaveProof 写入竞态证明。

   func (s *SQLStore) SaveProof(ctx context.Context, p *RaceProof) error {
	   if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	  }
	  q := `INSERT INTO race_proofs 
		(id, user_id, asset, book, lease_id,
		 winner_intent_id, loser_intent_id,
		 winner_amount, loser_amount, avail_at_decision,
		 window_start, window_end, reject_code, note, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		   p.ID, p.UserID, p.Asset, p.Book, p.LeaseID,
		  p.WinnerIntentID, p.LoserIntentID,
		   p.WinnerAmount.String(), p.LoserAmount.String(), p.AvailAtDecision.String(),
		   p.WindowStart, p.WindowEnd, string(p.RejectCode), p.Note, p.CreatedAt,
	   )
	   if err != nil {
		  return fmt.Errorf("availcas save proof: %w", err)
	   }
	  return nil
   }

// GetProof 按 ID 查询竞态证明。
  func (s *SQLStore) GetProof(ctx context.Context, id string) (*RaceProof, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	   q := `SELECT id, user_id, asset, book, lease_id,
	             winner_intent_id, loser_intent_id,
	             winner_amount, loser_amount, avail_at_decision,
	             window_start, window_end, reject_code, note, created_at
	      FROM race_proofs WHERE id = ? LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, id)
	   return scanProof(row)
   }

// ListProofs 列出用户×资产的竞态证明。
  func (s *SQLStore) ListProofs(ctx context.Context, userID, asset string, limit int) ([]*RaceProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  q := `SELECT id, user_id, asset, book, lease_id, 
	             winner_intent_id, loser_intent_id,
	             winner_amount, loser_amount, avail_at_decision,
	             window_start, window_end, reject_code, note, created_at
	      FROM race_proofs
	      WHERE user_id = ? AND asset = ?
	      ORDER BY created_at DESC LIMIT ?`
	  rows,
		  err := s.db.QueryContext(ctx, q, userID, asset, limit)
	  if err != nil {
		   return nil, fmt.Errorf("availcas list proofs: %w", err)
	   }
	   defer rows.Close()
	  var out []*RaceProof
	  for rows.Next() {
		  p, err := scanProof(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, p)
	  }
	   return out, rows.Err()
  }

// ────────────────────────────────────────────────────────────────────────────
// scan 辅助
// ────────────────────────────────────────────────────────────────────────────

type scannable interface {
	Scan(dest ...any) error
}

  func scanLease(r scannable) (*AvailLease, error) {
	   var l AvailLease
	   var (
		   bal, frozen, reserved, inFlight, avail string
		   state string
	   )
	   err := r.Scan(
		   &l.ID, &l.UserID, &l.Asset, &l.Book, &l.Gen,
		  &bal, &frozen, &reserved, &inFlight, &avail,
		   &state,
		   &l.ExpiresAt, &l.CreatedAt, &l.UpdatedAt,
	  )
	   if err != nil {
		   if err == sql.ErrNoRows {
			   return nil, ErrLeaseNotFound
		  }
		   return nil, fmt.Errorf("availcas scan lease: %w", err)
	  }
	   l.State = LeaseState(state)
	   l.Balance = mustDecimal(bal)
	  l.Frozen = mustDecimal(frozen)
	   l.Reserved = mustDecimal(reserved)
	  l.InFlight = mustDecimal(inFlight)
	  l.Available = mustDecimal(avail)
	   return &l, nil
  }

   func scanLeases(rows *sql.Rows) ([]*AvailLease, error) {
	  var out []*AvailLease
	   for rows.Next() {
		   l, err := scanLease(rows)
		  if err != nil {
			   return nil, err
		   }
		   out = append(out, l)
	  }
	  return out, rows.Err()
  }

  func scanIntent(r scannable) (*DebitIntent, error) {
	  var d DebitIntent
	   var (
		   amount, expectedAvail string
		  state, rejectCode     string
		  decidedAt sql.NullTime
	  )
	   err := r.Scan(
		   &d.ID, &d.IdemKey, &d.UserID, &d.Asset, &d.Book,
		  &amount, &d.Kind, &d.Reason,
		   &d.LeaseID, &d.LeaseGen, &expectedAvail,
		  &state, &rejectCode, &d.RejectNote, &d.RaceProofID,
		   &d.ActorID,
		   &d.RequestIP, &d.ProposedAt, &decidedAt,
	  )
	   if err != nil {
		  if err == sql.ErrNoRows {
			  return nil, ErrIntentNotFound
		  }
		   return nil, fmt.Errorf("availcas scan intent: %w", err)
	  }
	  d.State = IntentState(state)
	   d.RejectCode = RejectCode(rejectCode)
	  d.Amount = mustDecimal(amount)
	  d.ExpectedAvail = mustDecimal(expectedAvail)
	  if decidedAt.Valid {
		  t := decidedAt.Time
		   d.DecidedAt = &t
	   }
	   return &d, nil
   }

  func scanProof(r scannable) (*RaceProof, error) {
	  var p RaceProof
	   var (
		   winnerAmt, loserAmt, availAtDecision string
		  rejectCode string
	   )
	   err := r.Scan(
		  &p.ID, &p.UserID, &p.Asset, &p.Book, &p.LeaseID,
		   &p.WinnerIntentID, &p.LoserIntentID,
		   &winnerAmt, &loserAmt, &availAtDecision,
		   &p.WindowStart, &p.WindowEnd, &rejectCode, &p.Note, &p.CreatedAt,
	   )
	   if err != nil {
		  if err == sql.ErrNoRows {
			   return nil, ErrProofNotFound
		  }
		  return nil, fmt.Errorf("availcas scan proof: %w", err)
	  }
	  p.RejectCode = RejectCode(rejectCode)
	   p.WinnerAmount = mustDecimal(winnerAmt)
	   p.LoserAmount = mustDecimal(loserAmt)
	   p.AvailAtDecision = mustDecimal(availAtDecision)
	   return &p, nil
  }

// mustDecimal 解析十进制字符串；空串或非法值返回 0。
  func mustDecimal(s string) decimal.Decimal {
	  if s == "" {
		   return decimal.Zero
	  }
	  d, err := decimal.NewFromString(s)
	   if err != nil {
		   return decimal.Zero
	  }
	  return d
   }
