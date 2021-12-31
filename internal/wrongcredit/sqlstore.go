// 持久化层：SQL 读写与围栏语义。

// Author: Axel
// Date: 2021.12.30

package wrongcredit

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type caseSQL struct {
	db *sql.DB
}

func (s *Store) bindSQL(db *sql.DB) {
	if s != nil && db != nil {
		s.sql = &caseSQL{db: db}
	}
}

func (s *Store) hasSQL() bool {
	return s != nil && s.sql != nil && s.sql.db != nil
}

func (cs *caseSQL) saveCase(ctx context.Context, c WrongCreditCase) error {
	q := `INSERT INTO wrong_credit_case (
  id, idempotency_key, user_id, credit_ref, asset, credited_amt,
  confidence, risk_tier, score_note, score_version,
  clawed_amt, remain_amt, reversal_ref,
  phase, step, fence_epoch, fence_token,
  approved_by, approved_at, approved_by_2, approved_at_2,
  waived_by, waived_at, waived_note,
  opened_by, closed_by, note, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  confidence=VALUES(confidence), risk_tier=VALUES(risk_tier), score_note=VALUES(score_note),
  score_version=VALUES(score_version), clawed_amt=VALUES(clawed_amt), remain_amt=VALUES(remain_amt),
  reversal_ref=VALUES(reversal_ref), phase=VALUES(phase), step=VALUES(step),
  fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token),
  approved_by=VALUES(approved_by), approved_at=VALUES(approved_at),
  approved_by_2=VALUES(approved_by_2), approved_at_2=VALUES(approved_at_2),
  waived_by=VALUES(waived_by), waived_at=VALUES(waived_at), waived_note=VALUES(waived_note),
  closed_by=VALUES(closed_by), note=VALUES(note), updated_at=VALUES(updated_at)`
	_, err := cs.db.ExecContext(ctx, q,
		c.ID, c.IdempotencyKey, c.UserID, c.CreditRef, c.Asset, c.CreditedAmt,
		string(c.Confidence), string(c.RiskTier), c.ScoreNote, c.ScoreVersion,
		nullDec(c.ClawedAmt), nullDec(c.RemainAmt), nullStr(c.ReversalRef),
		string(c.Phase), string(c.Step), c.FenceEpoch, c.FenceToken,
		nullStr(c.ApprovedBy), nullTime(c.ApprovedAt), nullStr(c.ApprovedBy2), nullTime(c.ApprovedAt2),
		nullStr(c.WaivedBy), nullTime(c.WaivedAt), nullStr(c.WaivedNote),
		c.OpenedBy, nullStr(c.ClosedBy), c.Note, c.CreatedAt.UTC(), c.UpdatedAt.UTC(),
	)
	return err
}

func (cs *caseSQL) getCase(ctx context.Context, caseID string) (WrongCreditCase, error) {
	row := cs.db.QueryRowContext(ctx, `SELECT
  id, idempotency_key, user_id, credit_ref, asset, credited_amt,
  confidence, risk_tier, score_note, score_version,
  clawed_amt, remain_amt, reversal_ref,
  phase, step, fence_epoch, fence_token,
  approved_by, approved_at, approved_by_2, approved_at_2,
  waived_by, waived_at, waived_note,
  opened_by, closed_by, note, created_at, updated_at
 FROM wrong_credit_case WHERE id=?`, strings.TrimSpace(caseID))
	return scanCase(row)
}

func (cs *caseSQL) listCases(ctx context.Context, q ListReq) ([]WrongCreditCase, error) {
	where := `1=1`
	args := []any{}
	if uid := strings.TrimSpace(q.UserID); uid != "" {
		where += ` AND user_id=?`
		args = append(args, uid)
	}
	if ph := strings.TrimSpace(q.Phase); ph != "" {
		where += ` AND phase=?`
		args = append(args, ph)
	}
	if asset := strings.TrimSpace(q.Asset); asset != "" {
		where += ` AND asset=?`
		args = append(args, asset)
	}
	if ref := strings.TrimSpace(q.CreditRef); ref != "" {
		where += ` AND credit_ref=?`
		args = append(args, ref)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	qsql := `SELECT
  id, idempotency_key, user_id, credit_ref, asset, credited_amt,
  confidence, risk_tier, score_note, score_version,
  clawed_amt, remain_amt, reversal_ref,
  phase, step, fence_epoch, fence_token,
  approved_by, approved_at, approved_by_2, approved_at_2,
  waived_by, waived_at, waived_note,
  opened_by, closed_by, note, created_at, updated_at
 FROM wrong_credit_case WHERE ` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := cs.db.QueryContext(ctx, qsql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]WrongCreditCase, 0, limit)
	for rows.Next() {
		c, err := scanCaseRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (cs *caseSQL) listActiveCases(ctx context.Context) ([]WrongCreditCase, error) {
	rows, err := cs.db.QueryContext(ctx, `SELECT
  id, idempotency_key, user_id, credit_ref, asset, credited_amt,
  confidence, risk_tier, score_note, score_version,
  clawed_amt, remain_amt, reversal_ref,
  phase, step, fence_epoch, fence_token,
  approved_by, approved_at, approved_by_2, approved_at_2,
  waived_by, waived_at, waived_note,
  opened_by, closed_by, note, created_at, updated_at
 FROM wrong_credit_case
 WHERE step NOT IN ('close','waived','aborted')
 ORDER BY updated_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]WrongCreditCase, 0)
	for rows.Next() {
		c, err := scanCaseRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (cs *caseSQL) saveEvidence(ctx context.Context, e EvidenceEnvelope) error {
	_, err := cs.db.ExecContext(ctx, `INSERT INTO wrong_credit_evidence
(id, case_id, source, sha256, kind, note, payload, actor, score, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  source=VALUES(source), sha256=VALUES(sha256), kind=VALUES(kind), note=VALUES(note),
  payload=VALUES(payload), actor=VALUES(actor), score=VALUES(score)`,
		e.ID, e.CaseID, string(e.Source), e.SHA256, e.Kind, e.Note, nullStr(e.Payload),
		e.Actor, e.Score, e.CreatedAt.UTC(),
	)
	return err
}

func (cs *caseSQL) listEvidence(ctx context.Context, caseID string) ([]EvidenceEnvelope, error) {
	rows, err := cs.db.QueryContext(ctx, `SELECT id, case_id, source, sha256, kind, note, payload, actor, score, created_at
 FROM wrong_credit_evidence WHERE case_id=? ORDER BY created_at ASC`, strings.TrimSpace(caseID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EvidenceEnvelope, 0)
	for rows.Next() {
		var e EvidenceEnvelope
		var src string
		var payload sql.NullString
		if err := rows.Scan(&e.ID, &e.CaseID, &src, &e.SHA256, &e.Kind, &e.Note, &payload, &e.Actor, &e.Score, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Source = EvidenceSource(src)
		if payload.Valid {
			e.Payload = payload.String
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (cs *caseSQL) saveTimeline(ctx context.Context, ev TimelineEvent) error {
	_, err := cs.db.ExecContext(ctx, `INSERT INTO wrong_credit_timeline
(case_id, seq, kind, actor, from_phase, to_phase, from_step, to_step, note, at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  kind=VALUES(kind), actor=VALUES(actor), from_phase=VALUES(from_phase), to_phase=VALUES(to_phase),
  from_step=VALUES(from_step), to_step=VALUES(to_step), note=VALUES(note), at=VALUES(at)`,
		ev.CaseID, ev.Seq, ev.Kind, ev.Actor, string(ev.FromPhase), string(ev.ToPhase),
		string(ev.FromStep), string(ev.ToStep), ev.Note, ev.At.UTC(),
	)
	return err
}

func (cs *caseSQL) listTimeline(ctx context.Context, caseID string) ([]TimelineEvent, error) {
	rows, err := cs.db.QueryContext(ctx, `SELECT case_id, seq, kind, actor, from_phase, to_phase, from_step, to_step, note, at
 FROM wrong_credit_timeline WHERE case_id=? ORDER BY seq ASC`, strings.TrimSpace(caseID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TimelineEvent, 0)
	for rows.Next() {
		var ev TimelineEvent
		var fp, tp, fs, ts string
		if err := rows.Scan(&ev.CaseID, &ev.Seq, &ev.Kind, &ev.Actor, &fp, &tp, &fs, &ts, &ev.Note, &ev.At); err != nil {
			return nil, err
		}
		ev.FromPhase = Phase(fp)
		ev.ToPhase = Phase(tp)
		ev.FromStep = Step(fs)
		ev.ToStep = Step(ts)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (cs *caseSQL) saveMutex(ctx context.Context, c creditClaim) error {
	_, err := cs.db.ExecContext(ctx, `INSERT INTO wrong_credit_mutex (credit_ref, kind, owner, claimed_at)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind), owner=VALUES(owner), claimed_at=VALUES(claimed_at)`,
		c.CreditRef, string(c.Kind), c.Owner, c.ClaimedAt.UTC(),
	)
	return err
}

func (cs *caseSQL) deleteMutex(ctx context.Context, creditRef string) error {
	_, err := cs.db.ExecContext(ctx, `DELETE FROM wrong_credit_mutex WHERE credit_ref=?`, strings.TrimSpace(creditRef))
	return err
}

func (cs *caseSQL) loadMutex(ctx context.Context) ([]creditClaim, error) {
	rows, err := cs.db.QueryContext(ctx, `SELECT credit_ref, kind, owner, claimed_at FROM wrong_credit_mutex`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]creditClaim, 0)
	for rows.Next() {
		var c creditClaim
		var kind string
		if err := rows.Scan(&c.CreditRef, &kind, &c.Owner, &c.ClaimedAt); err != nil {
			return nil, err
		}
		c.Kind = ClaimKind(kind)
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanCase(row *sql.Row) (WrongCreditCase, error) {
	var c WrongCreditCase
	var conf, tier, phase, step string
	var claw, remain, rev sql.NullString
	var apBy, ap2, waivedBy, closedBy sql.NullString
	var apAt, ap2At, waivedAt sql.NullTime
	err := row.Scan(
		&c.ID, &c.IdempotencyKey, &c.UserID, &c.CreditRef, &c.Asset, &c.CreditedAmt,
		&conf, &tier, &c.ScoreNote, &c.ScoreVersion,
		&claw, &remain, &rev,
		&phase, &step, &c.FenceEpoch, &c.FenceToken,
		&apBy, &apAt, &ap2, &ap2At,
		&waivedBy, &waivedAt, &c.WaivedNote,
		&c.OpenedBy, &closedBy, &c.Note, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return WrongCreditCase{}, err
	}
	c.Confidence = Confidence(conf)
	c.RiskTier = RiskTier(tier)
	c.Phase = Phase(phase)
	c.Step = Step(step)
	if claw.Valid {
		c.ClawedAmt = claw.String
	}
	if remain.Valid {
		c.RemainAmt = remain.String
	}
	if rev.Valid {
		c.ReversalRef = rev.String
	}
	if apBy.Valid {
		c.ApprovedBy = apBy.String
	}
	if apAt.Valid {
		t := apAt.Time
		c.ApprovedAt = &t
	}
	if ap2.Valid {
		c.ApprovedBy2 = ap2.String
	}
	if ap2At.Valid {
		t := ap2At.Time
		c.ApprovedAt2 = &t
	}
	if waivedBy.Valid {
		c.WaivedBy = waivedBy.String
	}
	if waivedAt.Valid {
		t := waivedAt.Time
		c.WaivedAt = &t
	}
	if closedBy.Valid {
		c.ClosedBy = closedBy.String
	}
	return c, nil
}

func scanCaseRows(rows *sql.Rows) (WrongCreditCase, error) {
	var c WrongCreditCase
	var conf, tier, phase, step string
	var claw, remain, rev sql.NullString
	var apBy, ap2, waivedBy, closedBy sql.NullString
	var apAt, ap2At, waivedAt sql.NullTime
	err := rows.Scan(
		&c.ID, &c.IdempotencyKey, &c.UserID, &c.CreditRef, &c.Asset, &c.CreditedAmt,
		&conf, &tier, &c.ScoreNote, &c.ScoreVersion,
		&claw, &remain, &rev,
		&phase, &step, &c.FenceEpoch, &c.FenceToken,
		&apBy, &apAt, &ap2, &ap2At,
		&waivedBy, &waivedAt, &c.WaivedNote,
		&c.OpenedBy, &closedBy, &c.Note, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return WrongCreditCase{}, err
	}
	c.Confidence = Confidence(conf)
	c.RiskTier = RiskTier(tier)
	c.Phase = Phase(phase)
	c.Step = Step(step)
	if claw.Valid {
		c.ClawedAmt = claw.String
	}
	if remain.Valid {
		c.RemainAmt = remain.String
	}
	if rev.Valid {
		c.ReversalRef = rev.String
	}
	if apBy.Valid {
		c.ApprovedBy = apBy.String
	}
	if apAt.Valid {
		t := apAt.Time
		c.ApprovedAt = &t
	}
	if ap2.Valid {
		c.ApprovedBy2 = ap2.String
	}
	if ap2At.Valid {
		t := ap2At.Time
		c.ApprovedAt2 = &t
	}
	if waivedBy.Valid {
		c.WaivedBy = waivedBy.String
	}
	if waivedAt.Valid {
		t := waivedAt.Time
		c.WaivedAt = &t
	}
	if closedBy.Valid {
		c.ClosedBy = closedBy.String
	}
	return c, nil
}

func nullStr(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullDec(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{}
	}
	if _, err := decimal.NewFromString(s); err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}
