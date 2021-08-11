// @FileName: sqlstore.go
// @Summary: 资金路径同片 hop 补偿闸——SQL 持久化后端。
// 环境使用此实现；使用 memStore。
// 所有 SQL 语句兼容 MySQL 8.0 与 PostgreSQL 14。
// @Author: Bronson
// @Created: 2021.07.20

package hopcompgate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// sqlStore SQL 持久化实现。
type sqlStore struct {
	db      *sql.DB
	dialect string // "mysql" | "postgres"
}

// NewSQLStore 创建 SQL 仓储。dialect 为 "mysql" 或 "postgres"。
   func NewSQLStore(db *sql.DB, dialect string) Store {
	  d := strings.ToLower(strings.TrimSpace(dialect))
	  if d != "postgres" {
		  d = "mysql"
	   }
	   return &sqlStore{db: db, dialect: d}
   }

// placeholder 返回 dialect 对应的占位符。
  func (s *sqlStore) ph(n int) string {
	  if s.dialect == "postgres" {
		  return fmt.Sprintf("$%d", n)
	   }
	  return "?"
  }

// phs 生成 n 个占位符。
  func (s *sqlStore) phs(n int) []string {
	   out := make([]string, n)
	   for i := range out {
		   out[i] = s.ph(i + 1)
	  }
	  return out
   }

// ──────────────────────────────────────────────────────────────────────────────
// HopCompCase
// ──────────────────────────────────────────────────────────────────────────────

   func (s *sqlStore) OpenCase(ctx context.Context, req OpenCaseReq) (*HopCompCase, error) {
	   idem := strings.TrimSpace(req.HopIdem)
	   if idem == "" || req.UserID == "" || req.Asset == "" || req.MoveID == "" {
		  return nil, ErrMoveIDMissing
	  }
	   if !req.SameShard {
		  return nil, ErrCrossShardIntent
	  }
	  // 幂等：先查
	  existing, err := s.GetCaseByIdem(ctx, idem)
	  if err == nil {
		   if existing.UserID != req.UserID || existing.Asset != strings.ToUpper(strings.TrimSpace(req.Asset)) {
			   return nil, ErrIdemConflict
		  }
		  return existing, nil
	  }
	  if !errors.Is(err, ErrCaseNotFound) {
		  return nil, err
	  }
	  now := time.Now().UTC()
	   id := uuid.NewString()
	  ph := s.phs(17)
	   q := fmt.Sprintf(`INSERT INTO fundpath_hop_comps
		(id, hop_idem, user_id, asset, amount, from_book, to_book, move_id,
		 bind_err, state, generation, same_shard, actor, region, retry_count, created_at, updated_at)
		VALUES (%s)`,
		   strings.Join(ph, ","))
	  _, execErr := s.db.ExecContext(ctx, q,
		   id, idem, req.UserID,
		  strings.ToUpper(strings.TrimSpace(req.Asset)),
		   req.Amount,
		   strings.ToLower(strings.TrimSpace(req.FromBook)),
		  strings.ToLower(strings.TrimSpace(req.ToBook)),
		  strings.TrimSpace(req.MoveID),
		  truncate(req.BindErr, 512),
		  string(CaseOpen),
		  int64(1),
		   true,
		   strings.TrimSpace(req.Actor),
		  strings.TrimSpace(req.Region),
		  0,
		  now, now,
	  )
	  if execErr != nil {
		   // 重复键：幂等读取
		  if isDuplicate(execErr) {
			   return s.GetCaseByIdem(ctx, idem)
		   }
		   return nil, execErr
	   }
	   return s.GetCase(ctx, id)
   }

  func (s *sqlStore) GetCase(ctx context.Context, id string) (*HopCompCase, error) {
	  q := fmt.Sprintf(`SELECT id, hop_idem, user_id, asset, amount, from_book, to_book,
		move_id, bind_err, state, generation, same_shard, actor, region,
		retry_count, forced_by, force_reason, created_at, updated_at, closed_at
		FROM fundpath_hop_comps WHERE id = %s`, s.ph(1))
	  row := s.db.QueryRowContext(ctx, q, strings.TrimSpace(id))
	  return scanCase(row)
   }

   func (s *sqlStore) GetCaseByIdem(ctx context.Context, hopIdem string) (*HopCompCase, error) {
	   q := fmt.Sprintf(`SELECT id, hop_idem, user_id, asset, amount, from_book, to_book,
		move_id, bind_err, state, generation, same_shard, actor, region,
		retry_count, forced_by, force_reason, created_at, updated_at, closed_at
		FROM fundpath_hop_comps WHERE hop_idem = %s LIMIT 1`, s.ph(1))
	  row := s.db.QueryRowContext(ctx, q, strings.TrimSpace(hopIdem))
	   return scanCase(row)
   }

  func scanCase(row *sql.Row) (*HopCompCase, error) {
	   var c HopCompCase
	   var closedAt sql.NullTime
	   var forcedBy, forceReason, actor, region sql.NullString
	   var sameShard int
	   err := row.Scan(
		   &c.ID, &c.HopIdem, &c.UserID, &c.Asset, &c.Amount,
		  &c.FromBook, &c.ToBook, &c.MoveID, &c.BindErr,
		   &c.State, &c.Generation, &sameShard,
		  &actor, &region,
		  &c.RetryCount, &forcedBy, &forceReason,
		  &c.CreatedAt, &c.UpdatedAt, &closedAt,
	   )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrCaseNotFound
		   }
		  return nil, err
	   }
	   c.SameShard = sameShard != 0
	  c.Actor = actor.String
	   c.Region = region.String
	  c.ForcedBy = forcedBy.String
	  c.ForceReason = forceReason.String
	  if closedAt.Valid {
		   c.ClosedAt = &closedAt.Time
	  }
	   return &c, nil
   }

   func (s *sqlStore) UpdateCaseState(ctx context.Context, id string, from, to CaseState, gen int64) (*HopCompCase, error) {
	   if !from.CanTransition(to) {
		   return nil, ErrInvalidTransition
	  }
	   now := time.Now().UTC()
	  var closedExpr string
	   if to == CaseClosed {
		  closedExpr = fmt.Sprintf(", closed_at = %s", s.ph(5))
	   }
	  q := fmt.Sprintf(`UPDATE fundpath_hop_comps SET state = %s, updated_at = %s%s
		WHERE id = %s AND state = %s AND generation = %s`,
		  s.ph(1), s.ph(2), closedExpr, s.ph(3), s.ph(4), s.ph(6))
	   var args []interface{}
	   args = append(args, string(to), now)
	   if to == CaseClosed {
		  args = append(args, now)
	   }
	  args = append(args, strings.TrimSpace(id), string(from), gen)
	   // Re-number placeholders for postgres
	   if s.dialect == "postgres" {
		   q = renumberPH(q)
	   }
	   res, err := s.db.ExecContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	  if n, _ := res.RowsAffected(); n == 0 {
		   return nil, ErrInvalidTransition
	  }
	   return s.GetCase(ctx, id)
  }

   func (s *sqlStore) BumpGeneration(ctx context.Context, id string) (*HopCompCase, error) {
	  now := time.Now().UTC()
	   q := fmt.Sprintf(`UPDATE fundpath_hop_comps SET generation = generation + 1, updated_at = %s WHERE id = %s`,
		  s.ph(1), s.ph(2))
	  if _, err := s.db.ExecContext(ctx, q, now, strings.TrimSpace(id)); err != nil {
		   return nil, err
	   }
	  return s.GetCase(ctx, id)
   }

  func (s *sqlStore) ListCases(ctx context.Context, q CaseQueryReq) ([]*HopCompCase, error) {
	  limit := q.Limit
	   if limit <= 0 {
		  limit = 50
	  }
	   offset := q.Offset
	   where := []string{"1=1"}
	  args := []interface{}{}
	  ph := 1
	  addArg := func(col string, v interface{}) {
		   where = append(where, fmt.Sprintf("%s = %s", col, s.ph(ph)))
		  args = append(args, v)
		   ph++
	  }
	   if q.UserID != "" {
		   addArg("user_id", q.UserID)
	  }
	  if q.State != "" {
		   addArg("state", string(q.State))
	  }
	  if q.HopIdem != "" {
		   addArg("hop_idem", q.HopIdem)
	  }
	  if !q.After.IsZero() {
		  where = append(where, fmt.Sprintf("created_at > %s", s.ph(ph)))
		  args = append(args, q.After)
		  ph++
	   }
	   if !q.Before.IsZero() {
		  where = append(where, fmt.Sprintf("created_at < %s", s.ph(ph)))
		   args = append(args, q.Before)
		  ph++
	   }
	   args = append(args, limit, offset)
	   query := fmt.Sprintf(`SELECT id, hop_idem, user_id, asset, amount, from_book, to_book,
		move_id, bind_err, state, generation, same_shard, actor, region,
		retry_count, forced_by, force_reason, created_at, updated_at, closed_at
		FROM fundpath_hop_comps WHERE %s ORDER BY created_at DESC LIMIT %s OFFSET %s`,
		  strings.Join(where, " AND "), s.ph(ph), s.ph(ph+1))
	  rows, err := s.db.QueryContext(ctx, query, args...)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []*HopCompCase
	  for rows.Next() {
		  var c HopCompCase
		   var closedAt sql.NullTime
		  var forcedBy, forceReason, actor, region sql.NullString
		   var sameShard int
		   if err := rows.Scan(
			   &c.ID, &c.HopIdem, &c.UserID, &c.Asset, &c.Amount,
			   &c.FromBook, &c.ToBook, &c.MoveID, &c.BindErr,
			  &c.State, &c.Generation, &sameShard,
			   &actor, &region,
			  &c.RetryCount, &forcedBy, &forceReason,
			   &c.CreatedAt,
			  &c.UpdatedAt, &closedAt,
		  ); err != nil {
			  return nil, err
		   }
		   c.SameShard = sameShard != 0
		   c.Actor = actor.String
		   c.Region = region.String
		  c.ForcedBy = forcedBy.String
		   c.ForceReason = forceReason.String
		   if closedAt.Valid {
			  c.ClosedAt = &closedAt.Time
		   }
		  out = append(out, &c)
	  }
	   return out, rows.Err()
   }

  func (s *sqlStore) SetForceCompensate(ctx context.Context, id, actor, approver, reason string) (*HopCompCase, error) {
	   if actor == approver {
		  return nil, ErrForceApproverSameAsActor
	  }
	  forced := actor + "/" + approver
	  now := time.Now().UTC()
	   q := fmt.Sprintf(`UPDATE fundpath_hop_comps SET forced_by = %s, force_reason = %s, updated_at = %s WHERE id = %s`,
		  s.ph(1), s.ph(2), s.ph(3), s.ph(4))
	  if _, err := s.db.ExecContext(ctx, q, forced, truncate(reason, 256), now, strings.TrimSpace(id)); err != nil {
		  return nil, err
	  }
	   return s.GetCase(ctx, id)
   }

   func (s *sqlStore) MarkRetry(ctx context.Context, id string) (*HopCompCase, error) {
	   now := time.Now().UTC()
	   q := fmt.Sprintf(`UPDATE fundpath_hop_comps SET retry_count = retry_count + 1, updated_at = %s WHERE id = %s`,
		  s.ph(1), s.ph(2))
	   if _, err := s.db.ExecContext(ctx, q, now, strings.TrimSpace(id)); err != nil {
		  return nil, err
	   }
	   return s.GetCase(ctx, id)
  }

// ──────────────────────────────────────────────────────────────────────────────
// CompensateLease
// ──────────────────────────────────────────────────────────────────────────────

   func (s *sqlStore) AcquireLease(ctx context.Context, req AcquireLeaseReq, now time.Time) (*CompensateLease, error) {
	   c,
		  err := s.GetCase(ctx, req.CaseID)
	  if err != nil {
		   return nil, err
	  }
	  if c.State == CaseClosed {
		  return nil, ErrCaseAlreadyClosed
	   }
	  if c.State == CaseBlocked {
		   return nil, ErrCaseBlocked
	  }
	   ttl := req.TTL
	  if ttl <= 0 {
		  ttl = DefaultLeaseTTL
	  }
	  if ttl > MaxLeaseTTL {
		   ttl = MaxLeaseTTL
	  }
	   id := uuid.NewString()
	   token := uuid.NewString()
	  exp := now.Add(time.Duration(ttl) * time.Second)
	   ph := s.phs(6)
	   q := fmt.Sprintf(`INSERT INTO fundpath_comp_leases
		(id, case_id, generation, token, state, expires_at, created_at)
		VALUES (%s)`, strings.Join(ph, ","))
	   if _, err := s.db.ExecContext(ctx, q, id, c.ID, c.Generation, token, string(LeaseOpen), exp, now); err != nil {
		   return nil, err
	   }
	   return s.GetLease(ctx, id)
   }

   func (s *sqlStore) GetLease(ctx context.Context, id string) (*CompensateLease, error) {
	   q := fmt.Sprintf(`SELECT id, case_id, generation, token, state, expires_at,
		bound_at, done_at, expired_at, created_at
		FROM fundpath_comp_leases WHERE id = %s`, s.ph(1))
	   row := s.db.QueryRowContext(ctx, q, strings.TrimSpace(id))
	   return scanLease(row)
   }

  func scanLease(row *sql.Row) (*CompensateLease, error) {
	  var l CompensateLease
	   var boundAt, doneAt, expiredAt sql.NullTime
	  err := row.Scan(
		   &l.ID, &l.CaseID, &l.Generation, &l.Token, &l.State,
		   &l.ExpiresAt, &boundAt, &doneAt, &expiredAt, &l.CreatedAt,
	   )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrLeaseNotFound
		   }
		   return nil, err
	   }
	  if boundAt.Valid {
		   l.BoundAt = &boundAt.Time
	   }
	  if doneAt.Valid {
		  l.DoneAt = &doneAt.Time
	   }
	   if expiredAt.Valid {
		  l.ExpiredAt = &expiredAt.Time
	   }
	   return &l, nil
  }

  func (s *sqlStore) BindLease(ctx context.Context, id, token string) (*CompensateLease, error) {
	  now := time.Now().UTC()
	  q := fmt.Sprintf(`UPDATE fundpath_comp_leases SET state = %s, bound_at = %s
		WHERE id = %s AND state = %s AND token = %s`,
		  s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5))
	  res, err := s.db.ExecContext(ctx, q, string(LeaseBound), now, strings.TrimSpace(id), string(LeaseOpen), token)
	  if err != nil {
		  return nil, err
	   }
	   if n, _ := res.RowsAffected(); n == 0 {
		  return nil, ErrLeaseExpired
	   }
	   return s.GetLease(ctx, id)
   }

  func (s *sqlStore) CloseLease(ctx context.Context, id string) (*CompensateLease, error) {
	   now := time.Now().UTC()
	   q := fmt.Sprintf(`UPDATE fundpath_comp_leases SET state = %s, done_at = %s
		WHERE id = %s AND state = %s`,
		  s.ph(1), s.ph(2), s.ph(3), s.ph(4))
	  res, err := s.db.ExecContext(ctx, q, string(LeaseDone), now, strings.TrimSpace(id), string(LeaseBound))
	  if err != nil {
		   return nil, err
	   }
	  if n, _ := res.RowsAffected(); n == 0 {
		  return nil, ErrLeaseExpired
	   }
	   return s.GetLease(ctx, id)
  }

   func (s *sqlStore) ExpireLease(ctx context.Context, id string, now time.Time) (*CompensateLease, error) {
	  q := fmt.Sprintf(`UPDATE fundpath_comp_leases SET state = %s, expired_at = %s
		WHERE id = %s AND state NOT IN (%s, %s)`,
		  s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5))
	   if _, err := s.db.ExecContext(ctx, q, string(LeaseExpired), now, strings.TrimSpace(id),
		  string(LeaseDone), string(LeaseExpired)); err != nil {
		  return nil, err
	   }
	   return s.GetLease(ctx, id)
  }

  func (s *sqlStore) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]*CompensateLease, error) {
	  if limit <= 0 {
		   limit = 100
	  }
	   q := fmt.Sprintf(`SELECT id, case_id, generation, token, state, expires_at, 
		bound_at, done_at, expired_at, created_at
		FROM fundpath_comp_leases
		WHERE expires_at < %s AND state NOT IN (%s, %s)
		ORDER BY expires_at ASC LIMIT %s`,
		  s.ph(1), s.ph(2), s.ph(3), s.ph(4))
	  rows, err := s.db.QueryContext(ctx, q, now, string(LeaseDone), string(LeaseExpired), limit)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   var out []*CompensateLease
	  for rows.Next() {
		   var l CompensateLease
		  var boundAt,
			  doneAt, expiredAt sql.NullTime
		  if err := rows.Scan(
			   &l.ID, &l.CaseID, &l.Generation, &l.Token, &l.State,
			  &l.ExpiresAt, &boundAt, &doneAt, &expiredAt, &l.CreatedAt,
		  ); err != nil {
			   return nil, err
		  }
		   if boundAt.Valid {
			  l.BoundAt = &boundAt.Time
		   }
		  if doneAt.Valid {
			  l.DoneAt = &doneAt.Time
		   }
		   if expiredAt.Valid {
			   l.ExpiredAt = &expiredAt.Time
		   }
		  out = append(out, &l)
	   }
	   return out, rows.Err()
  }

// ──────────────────────────────────────────────────────────────────────────────
// DoubleMoveProof
// ──────────────────────────────────────────────────────────────────────────────

   func (s *sqlStore) WriteProof(ctx context.Context, p *DoubleMoveProof) error {
	  if p == nil {
		   return nil
	   }
	   if p.ID == "" {
		  p.ID = uuid.NewString()
	  }
	   ph := s.phs(11)
	  q := fmt.Sprintf(`INSERT INTO fundpath_double_move_proofs 
		(id, case_id, kind, hop_idem, user_id, asset, amount,
		 move_id, compensate_move_id, second_move_id, evidence, detected_at)
		VALUES (%s)
		ON CONFLICT DO NOTHING`,
		  strings.Join(ph, ","))
	  if s.dialect == "mysql" {
		  q = strings.Replace(q, "ON CONFLICT DO NOTHING",
			  "ON DUPLICATE KEY UPDATE id=id", 1)
	  }
	   _, err := s.db.ExecContext(ctx, q,
		  p.ID, p.CaseID, string(p.Kind),
		  p.HopIdem, p.UserID, p.Asset, p.Amount,
		  p.MoveID,
		   nullStr(p.CompensateMoveID),
		   nullStr(p.SecondMoveID),
		   p.Evidence, p.DetectedAt,
	   )
	  return err
   }

   func (s *sqlStore) GetProof(ctx context.Context, id string) (*DoubleMoveProof, error) {
	   q := fmt.Sprintf(`SELECT id, case_id, kind, hop_idem, user_id, asset, amount,
		move_id, compensate_move_id, second_move_id, evidence, detected_at, exported_at
		FROM fundpath_double_move_proofs WHERE id = %s`, s.ph(1))
	  row := s.db.QueryRowContext(ctx,
		  q, strings.TrimSpace(id))
	   return scanProof(row)
  }

  func scanProof(row *sql.Row) (*DoubleMoveProof, error) {
	  var p DoubleMoveProof
	  var compMoveID,
		   secondMoveID sql.NullString
	  var exportedAt sql.NullTime
	   err := row.Scan(
		   &p.ID, &p.CaseID, &p.Kind,
		   &p.HopIdem, &p.UserID, &p.Asset, &p.Amount,
		  &p.MoveID, &compMoveID, &secondMoveID,
		  &p.Evidence, &p.DetectedAt, &exportedAt,
	  )
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrCaseNotFound
		  }
		  return nil, err
	   }
	   p.CompensateMoveID = compMoveID.String
	   p.SecondMoveID = secondMoveID.String
	  if exportedAt.Valid {
		  p.ExportedAt = &exportedAt.Time
	   }
	   return &p, nil
  }

   func (s *sqlStore) ListProofs(ctx context.Context, q ProofExportReq) ([]*DoubleMoveProof, error) {
	   limit := q.Limit
	   if limit <= 0 {
		  limit = 100
	  }
	   where := []string{"1=1"}
	  args := []interface{}{}
	  ph := 1
	   if q.CaseID != "" {
		   where = append(where, fmt.Sprintf("case_id = %s", s.ph(ph)))
		   args = append(args, q.CaseID)
		   ph++
	  }
	  if q.Kind != "" {
		  where = append(where, fmt.Sprintf("kind = %s", s.ph(ph)))
		   args = append(args, string(q.Kind))
		  ph++
	  }
	   if !q.After.IsZero() {
		  where = append(where, fmt.Sprintf("detected_at > %s", s.ph(ph)))
		  args = append(args, q.After)
		   ph++
	  }
	   args = append(args, limit)
	   query := fmt.Sprintf(`SELECT id, case_id, kind, hop_idem, user_id, asset, amount,
		move_id, compensate_move_id, second_move_id, evidence, detected_at, exported_at
		FROM fundpath_double_move_proofs WHERE %s ORDER BY detected_at DESC LIMIT %s`,
		   strings.Join(where, " AND "), s.ph(ph))
	   rows, err := s.db.QueryContext(ctx, query, args...)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []*DoubleMoveProof
	   for rows.Next() {
		   var p DoubleMoveProof
		  var compMoveID, secondMoveID sql.NullString
		  var exportedAt sql.NullTime
		  if err := rows.Scan(
			   &p.ID, &p.CaseID, &p.Kind,
			  &p.HopIdem, &p.UserID, &p.Asset, &p.Amount,
			   &p.MoveID, &compMoveID, &secondMoveID,
			  &p.Evidence, &p.DetectedAt, &exportedAt,
		  ); err != nil {
			   return nil, err
		  }
		   p.CompensateMoveID = compMoveID.String
		  p.SecondMoveID = secondMoveID.String
		   if exportedAt.Valid {
			  p.ExportedAt = &exportedAt.Time
		  }
		  out = append(out, &p)
	  }
	  return out, rows.Err()
   }

  func (s *sqlStore) MarkProofExported(ctx context.Context, id string, at time.Time) error {
	   q := fmt.Sprintf(`UPDATE fundpath_double_move_proofs SET exported_at = %s WHERE id = %s`,
		  s.ph(1), s.ph(2))
	   _, err := s.db.ExecContext(ctx, q, at, strings.TrimSpace(id))
	  return err
  }

// ──────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────────────────────────────────────

  func isDuplicate(err error) bool {
	  if err == nil {
		   return false
	   }
	  msg := strings.ToLower(err.Error())
	   return strings.Contains(msg, "duplicate") ||
		   strings.Contains(msg, "unique") ||
		  strings.Contains(msg, "1062")
  }

  func nullStr(s string) sql.NullString {
	  return sql.NullString{String: s, Valid: s != ""}
   }

// renumberPH 将 MySQL 风格的 ? 占位符重编号为 Postgres 的 $N。
// 仅在 UpdateCaseState 的动态 SQL 中使用。
   func renumberPH(q string) string {
	   n := 0
	   var b strings.Builder
	   for _, ch := range q {
		   if ch == '?' {
			   n++
			   b.WriteString(fmt.Sprintf("$%d", n))
		  } else {
			   b.WriteRune(ch)
		   }
	  }
	  return b.String()
  }
