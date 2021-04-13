// 功能：cv_quote_leases / cv_mark_gens / cv_quote_proofs SQL 存储。
// 作者：Knox
// 日期：2021.03.30

package cvquotelease

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type SQLStore struct {
	db *sql.DB
}

   func NewSQLStore(db *sql.DB) *SQLStore {
	   return &SQLStore{db: db}
   }

   func (s *SQLStore) SaveLease(ctx context.Context, l CvQuoteLease) (CvQuoteLease, error) {
	   if s == nil || s.db == nil {
		  return CvQuoteLease{}, ErrStoreUnavailable
	   }
	  body, err := json.Marshal(l)
	  if err != nil {
		  return CvQuoteLease{}, err
	   }
	   _, err = s.db.ExecContext(ctx, `
INSERT INTO cv_quote_leases (
  id, quote_id, user_id, from_asset, to_asset, amount, rate, book,
  mark_gen, fingerprint, book_print, state, ttl_ms, issued_at, expires_at,
  consumed_at, expired_at, revoked_at, used, order_id, proof_id, revoke_reason,
  consume_token, body, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), used=VALUES(used), order_id=VALUES(order_id),
  proof_id=VALUES(proof_id), consumed_at=VALUES(consumed_at),
  expired_at=VALUES(expired_at), revoked_at=VALUES(revoked_at),
  revoke_reason=VALUES(revoke_reason), consume_token=VALUES(consume_token),
  body=VALUES(body), updated_at=VALUES(updated_at)`,
		  l.ID, l.QuoteID, l.UserID, l.FromAsset, l.ToAsset, l.Amount, l.Rate, l.Book,
		   l.MarkGen, l.Fingerprint, l.BookPrint, string(l.State), l.TTL.Milliseconds(),
		   l.IssuedAt, l.ExpiresAt, nullTime(l.ConsumedAt), nullTime(l.ExpiredAt), nullTime(l.RevokedAt),
		   boolToInt(l.Used), l.OrderID, l.ProofID, l.RevokeReason, l.ConsumeToken, body, l.CreatedAt, l.UpdatedAt,
	   )
	   if err != nil {
		   return CvQuoteLease{}, err
	   }
	   return l, nil
  }

   func (s *SQLStore) GetLease(ctx context.Context, id string) (CvQuoteLease, error) {
	   return s.scanLease(ctx, `SELECT body FROM cv_quote_leases WHERE id=?`, strings.TrimSpace(id))
  }

  func (s *SQLStore) GetLeaseByQuote(ctx context.Context, quoteID string) (CvQuoteLease, error) {
	   return s.scanLease(ctx, `SELECT body FROM cv_quote_leases WHERE quote_id=? ORDER BY issued_at DESC LIMIT 1`, strings.TrimSpace(quoteID))
   }

   func (s *SQLStore) scanLease(ctx context.Context, q string, args ...any) (CvQuoteLease, error) {
	   if s == nil || s.db == nil {
		  return CvQuoteLease{}, ErrStoreUnavailable
	   }
	  var body []byte
	   if err := s.db.QueryRowContext(ctx, q, args...).Scan(&body); err != nil {
		  if err == sql.ErrNoRows {
			  return CvQuoteLease{}, ErrLeaseNotFound
		   }
		  return CvQuoteLease{}, err
	   }
	   var l CvQuoteLease
	  if err := json.Unmarshal(body, &l); err != nil || l.ID == "" {
		   return CvQuoteLease{}, ErrLeaseNotFound
	  }
	  return l, nil
  }

  func (s *SQLStore) ListIssuedByPair(ctx context.Context, pair string, limit int) ([]CvQuoteLease, error) {
	  parts := strings.Split(pair, "|")
	  if len(parts) != 2 {
		   return nil, ErrMarkPairRequired
	   }
	  if limit <= 0 {
		  limit = DefaultRevokeBatch
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT body FROM cv_quote_leases
 WHERE from_asset=? AND to_asset=? AND state='issued'
 ORDER BY issued_at ASC LIMIT ?`, parts[0], parts[1], limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanLeaseRows(rows)
   }

  func (s *SQLStore) ListExpiredIssued(ctx context.Context, before time.Time, limit int) ([]CvQuoteLease, error) {
	  if limit <= 0 {
		   limit = DefaultExpiryBatch
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT body FROM cv_quote_leases
 WHERE state='issued' AND expires_at<=?
 ORDER BY expires_at ASC LIMIT ?`, before, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanLeaseRows(rows)
   }

  func (s *SQLStore) ListIssued(ctx context.Context, limit int) ([]CvQuoteLease, error) {
	   return s.ListLeasesByState(ctx, LeaseIssued, limit)
  }

  func (s *SQLStore) ListLeasesByUser(ctx context.Context, userID string, limit int) ([]CvQuoteLease, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	   rows, err := s.db.QueryContext(ctx, `SELECT body FROM cv_quote_leases WHERE user_id=? ORDER BY issued_at DESC LIMIT ?`, userID, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanLeaseRows(rows)
  }

   func (s *SQLStore) ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]CvQuoteLease, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  rows, err := s.db.QueryContext(ctx, `SELECT body FROM cv_quote_leases WHERE state=? LIMIT ?`, string(state), limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanLeaseRows(rows)
   }

  func (s *SQLStore) CountByState(ctx context.Context, state LeaseState) (int, error) {
	  var n int
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cv_quote_leases WHERE state=?`, string(state)).Scan(&n)
	  return n, err
  }

   func (s *SQLStore) CASConsume(ctx context.Context, id, token string, now time.Time, orderID string) (CvQuoteLease, bool, error) {
	  tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	  if err != nil {
		  return CvQuoteLease{},
			   false, err
	  }
	  defer func() { _ = tx.Rollback() }()
	  var body []byte
	   if err := tx.QueryRowContext(ctx, `SELECT body FROM cv_quote_leases WHERE id=? FOR UPDATE`, id).Scan(&body); err != nil {
		   if err == sql.ErrNoRows {
			  return CvQuoteLease{}, false, ErrLeaseNotFound
		   }
		   return CvQuoteLease{}, false, err
	  }
	  var l CvQuoteLease
	   if json.Unmarshal(body, &l) != nil {
		   return CvQuoteLease{}, false, ErrLeaseNotFound
	   }
	   if l.State == LeaseConsumed && l.Used {
		  return l, false, nil
	  }
	  if l.State != LeaseIssued {
		  return l, false, ErrLeaseConsumed
	  }
	   ts := now
	   l.State = LeaseConsumed
	   l.Used = true
	  l.OrderID = orderID
	   l.ConsumeToken = token
	  l.ConsumedAt = &ts
	   l.UpdatedAt = now
	  nb, _ := json.Marshal(l)
	  if _, err := tx.ExecContext(ctx, `UPDATE cv_quote_leases SET state='consumed', used=1, order_id=?, consume_token=?, consumed_at=?, body=?, updated_at=? WHERE id=? AND state='issued'`,
		  orderID, token, now, nb, now, id); err != nil {
		   return CvQuoteLease{}, false, err
	   }
	  if _, err := tx.ExecContext(ctx, `UPDATE cv_quote_snaps SET used=1, order_id=? WHERE id=?`, orderID, l.QuoteID); err != nil {
		  // 快照表可能尚未迁移到独立 snaps；忽略列缺失由调用方 Mem 路径覆盖
		   _ = err
	   }
	   if err := tx.Commit(); err != nil {
		  return CvQuoteLease{}, false, err
	  }
	   return l, true, nil
   }

  func (s *SQLStore) SaveMark(ctx context.Context, m MarkGeneration) (MarkGeneration, error) {
	  body, err := json.Marshal(m)
	   if err != nil {
		   return MarkGeneration{}, err
	  }
	   _, err = s.db.ExecContext(ctx, ` 
INSERT INTO cv_mark_gens (pair_key, from_asset, to_asset, mark_gen, rate, book_print, source, updated_by, revoke_count, body, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE mark_gen=VALUES(mark_gen), rate=VALUES(rate), book_print=VALUES(book_print),
  source=VALUES(source), updated_by=VALUES(updated_by), revoke_count=VALUES(revoke_count), body=VALUES(body), updated_at=VALUES(updated_at)`,
		   m.PairKey, m.FromAsset, m.ToAsset, m.Gen, m.Rate, m.BookPrint, m.Source, m.UpdatedBy, m.RevokeCount, body, m.UpdatedAt)
	  return m, err
  }

  func (s *SQLStore) GetMark(ctx context.Context, pair string) (MarkGeneration, error) {
	   var body []byte
	  if err := s.db.QueryRowContext(ctx, `SELECT body FROM cv_mark_gens WHERE pair_key=?`, pair).Scan(&body); err != nil {
		   if err == sql.ErrNoRows {
			  return MarkGeneration{},
				  ErrMarkGenMissing
		   }
		   return MarkGeneration{}, err
	   }
	   var m MarkGeneration
	  if json.Unmarshal(body, &m) != nil {
		  return MarkGeneration{}, ErrMarkGenMissing
	   }
	   return m, nil
  }

   func (s *SQLStore) ListMarks(ctx context.Context) ([]MarkGeneration, error) {
	   rows, err := s.db.QueryContext(ctx, `SELECT body FROM cv_mark_gens`)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  out := make([]MarkGeneration, 0)
	   for rows.Next() {
		   var body []byte
		  if rows.Scan(&body) != nil {
			   continue
		  }
		   var m MarkGeneration
		  if json.Unmarshal(body, &m) == nil {
			  out = append(out, m)
		  }
	   }
	   return out, rows.Err()
  }

  func (s *SQLStore) CountMarks(ctx context.Context) (int, error) {
	  var n int
	   err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cv_mark_gens`).Scan(&n)
	   return n, err
   }

  func (s *SQLStore) SaveQuote(ctx context.Context, q QuoteSnapshot) (QuoteSnapshot, error) {
	  body, err := json.Marshal(q)
	  if err != nil {
		  return QuoteSnapshot{}, err
	   }
	  _, err = s.db.ExecContext(ctx, `
INSERT INTO cv_quote_snaps (id, lease_id, user_id, from_asset, to_asset, amount, rate, book, mark_gen, expires_at, used, order_id, body, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE used=VALUES(used), order_id=VALUES(order_id), body=VALUES(body)`,
		  q.ID,
		   q.LeaseID, q.UserID, q.FromAsset, q.ToAsset, q.Amount, q.Rate, q.Book, q.MarkGen, q.ExpiresAt, boolToInt(q.Used), q.OrderID, body, q.CreatedAt)
	  return q, err
  }

   func (s *SQLStore) GetQuote(ctx context.Context, id string) (QuoteSnapshot, error) {
	   var body []byte
	  if err := s.db.QueryRowContext(ctx, `SELECT body FROM cv_quote_snaps WHERE id=?`, id).Scan(&body); err != nil {
		   if err == sql.ErrNoRows {
			   return QuoteSnapshot{}, ErrQuoteNotFound
		   }
		  return QuoteSnapshot{}, err
	  }
	  var q QuoteSnapshot
	  if json.Unmarshal(body, &q) != nil {
		   return QuoteSnapshot{}, ErrQuoteNotFound
	   }
	  return q, nil
  }

   func (s *SQLStore) SaveProof(ctx context.Context, p CvQuoteDriftProof) (CvQuoteDriftProof, error) {
	  body, err := json.Marshal(p)
	   if err != nil {
		   return CvQuoteDriftProof{}, err
	   }
	   _, err = s.db.ExecContext(ctx, `
INSERT INTO cv_quote_proofs (
  id, lease_id, quote_id, user_id, from_asset, to_asset, kind,
  lease_mark_gen, live_mark_gen, lease_rate, live_rate, lease_book, live_book,
  lease_print, live_print, blocked, sealed, detail, body, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE detail=VALUES(detail), body=VALUES(body)`,
		   p.ID, p.LeaseID, p.QuoteID, p.UserID, p.FromAsset, p.ToAsset, string(p.Kind),
		   p.LeaseMarkGen, p.LiveMarkGen, p.LeaseRate, p.LiveRate, p.LeaseBook, p.LiveBook,
		   p.LeasePrint, p.LivePrint, boolToInt(p.Blocked), boolToInt(p.Sealed), p.Detail, body, p.CreatedAt)
	   return p, err
   }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (CvQuoteDriftProof, error) {
	   var body []byte
	  if err := s.db.QueryRowContext(ctx, `SELECT body FROM cv_quote_proofs WHERE id=?`, id).Scan(&body); err != nil {
		   if err == sql.ErrNoRows {
			   return CvQuoteDriftProof{}, ErrProofNotFound
		  }
		   return CvQuoteDriftProof{}, err
	   }
	  var p CvQuoteDriftProof
	  if json.Unmarshal(body, &p) != nil {
		   return CvQuoteDriftProof{}, ErrProofNotFound
	  }
	  return p, nil
   }

   func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]CvQuoteDriftProof, error) {
	   return s.listProofs(ctx, `SELECT body FROM cv_quote_proofs WHERE lease_id=? ORDER BY created_at DESC LIMIT ?`, leaseID, limit)
   }

  func (s *SQLStore) ListProofsByQuote(ctx context.Context, quoteID string, limit int) ([]CvQuoteDriftProof, error) {
	   return s.listProofs(ctx, `SELECT body FROM cv_quote_proofs WHERE quote_id=? ORDER BY created_at DESC LIMIT ?`, quoteID, limit)
   }

  func (s *SQLStore) ListProofsByUser(ctx context.Context, userID string, limit int) ([]CvQuoteDriftProof, error) {
	   return s.listProofs(ctx, `SELECT body FROM cv_quote_proofs WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
   }

   func (s *SQLStore) CountProofs(ctx context.Context) (int, error) {
	  var n int
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cv_quote_proofs`).Scan(&n)
	  return n, err
   }

   func (s *SQLStore) listProofs(ctx context.Context, q, key string, limit int) ([]CvQuoteDriftProof, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	   rows, err := s.db.QueryContext(ctx, q, key, limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]CvQuoteDriftProof, 0)
	  for rows.Next() {
		  var body []byte
		   if rows.Scan(&body) != nil {
			  continue
		   }
		  var p CvQuoteDriftProof
		   if json.Unmarshal(body, &p) == nil {
			  out = append(out, p)
		  }
	  }
	  return out, rows.Err()
   }

  func scanLeaseRows(rows *sql.Rows) ([]CvQuoteLease, error) {
	   out := make([]CvQuoteLease, 0)
	  for rows.Next() {
		   var body []byte
		  if rows.Scan(&body) != nil {
			  continue
		   }
		   var l CvQuoteLease
		  if json.Unmarshal(body, &l) == nil {
			   out = append(out, l)
		   }
	  }
	  return out, rows.Err()
   }

   func nullTime(t *time.Time) any {
	  if t == nil {
		   return nil
	   }
	  return *t
  }

  func boolToInt(b bool) int {
	  if b {
		   return 1
	  }
	  return 0
   }
