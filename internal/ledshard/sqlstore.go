package ledshard

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore 把内存目录镜像到 led_shard_* 。协议判断仍在 Engine，避免两套状态机。

type SQLStore struct {
	db *persist.SQLDB
}

   func (s *SQLStore) SaveAll(m *Mem) error {
	   if s == nil || s.db == nil || m == nil {
		  return nil
	   }
	  ctx := context.Background()
	   return s.db.InTx(ctx, func(ctx context.Context) error {
		  q := s.db.Conn(ctx)
		   for _, table := range snapshotTables {
			   if _, err := q.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				   return err
			   }
		   }
		  for _, sh := range m.shards {
			  if _, err := q.ExecContext(ctx, ` 
INSERT INTO led_shard_shards (id, kind, status, weight, pin_subject, region, note, created_ms, updated_ms)
VALUES (?,?,?,?,?,?,?,?,?)`,
				  sh.ID, string(sh.Kind), string(sh.Status), sh.Weight, sh.PinSubject, sh.Region, sh.Note, ms(sh.CreatedAt), ms(sh.UpdatedAt)); err != nil {
				  return err
			  }
		   }
		  for _, r := range m.routes {
			   ring, err := json.Marshal(r.UserShards)
			  if err != nil {
				  return err
			   }
			   pins, err := json.Marshal(r.Pins)
			  if err != nil {
				   return err
			  }
			   aff, err := json.Marshal(r.Affinity)
			   if err != nil {
				   return err
			   }
			  if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_routes (version, status, modulus, user_shards, pins, affinity, fingerprint, note, created_ms, published_ms, activated_ms, retired_ms)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				  r.Version, string(r.Status), r.Modulus, string(ring), string(pins), string(aff), r.Fingerprint, r.Note,
				  ms(r.CreatedAt), ms(r.PublishedAt), ms(r.ActivatedAt), ms(r.RetiredAt)); err != nil {
				   return err
			   }
		   }
		  for _, it := range m.intents {
			  if _, err := q.ExecContext(ctx, ` 
INSERT INTO led_shard_intents (
 id, idem, biz_hash, route_version, route_fp, src_user, src_book, src_subject, src_shard,
 dst_user, dst_book, dst_subject, dst_shard, asset, amount, state, halted, manual, deadline_ms,
 freeze_posted, voucher_id, credit_posted, credit_count, reverse_count, reason, desk_id, created_ms, updated_ms)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				  it.ID, it.Idem, it.BizHash, it.RouteVersion, it.RouteFP, it.SrcUser, it.SrcBook, it.SrcSubject, it.SrcShard,
				  it.DstUser,
				   it.DstBook, it.DstSubject, it.DstShard, it.Asset, it.Amount, string(it.State), b2i(it.Halted), b2i(it.Manual), ms(it.Deadline),
				  b2i(it.FreezePosted), it.VoucherID, b2i(it.CreditPosted), it.CreditCount, it.ReverseCount, it.Reason, it.DeskID, ms(it.CreatedAt), ms(it.UpdatedAt)); err != nil {
				   return err
			  }
		  }
		   for intentID, steps := range m.steps {
			   for _, st := range steps {
				   if st.IntentID == "" {
					  st.IntentID = intentID
				   }
				   if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_steps (id, intent_id, seq, kind, state, shard_id, ledger_from, ledger_to, detail, at_ms)
VALUES (?,?,?,?,?,?,?,?,?,?)`,
					   st.ID, st.IntentID, st.Seq, string(st.Kind), st.State, st.ShardID, st.LedgerFrom, st.LedgerTo, st.Detail, ms(st.At)); err != nil {
					  return err
				   }
			   }
		  }
		   for _, v := range m.vouchers {
			   if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_vouchers (
 id, intent_id, idem, asset, amount, src_shard, dst_shard, bridge_shard, state, route_version, credit_count, created_ms, consumed_ms, voided_ms)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				  v.ID, v.IntentID, v.Idem, v.Asset, v.Amount, v.SrcShard, v.DstShard, v.BridgeShard, string(v.State), v.RouteVersion, v.CreditCount,
				   ms(v.CreatedAt), ms(v.ConsumedAt), ms(v.VoidedAt)); err != nil {
				   return err
			   }
		  }
		  for _, b := range m.bals {
			   if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_balances (shard_id, subject, asset, available, frozen) VALUES (?,?,?,?,?)`,
				  b.ShardID, b.Subject, b.Asset, nzZero(b.Available), nzZero(b.Frozen)); err != nil {
				  return err
			   }
		   }
		   for _, posts := range m.posts {
			  for _, p := range posts {
				  if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_postings (
 id, intent_id, voucher_id, seq, kind, shard_id, subject, asset, debit_avail, credit_avail, debit_frozen, credit_frozen, memo, at_ms)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
					   p.ID, p.IntentID, p.VoucherID, p.Seq, string(p.Kind), p.ShardID, p.Subject, p.Asset,
					   nz(p.DebitAvail), nz(p.CreditAvail), nz(p.DebitFrozen), nz(p.CreditFrozen), p.Memo, ms(p.At)); err != nil {
					  return err
				  }
			   }
		   }
		  for _, idem := range m.idems {
			   if _, err := q.ExecContext(ctx, `INSERT INTO led_shard_idem (idem, biz_hash, intent_id) VALUES (?,?,?)`,
				   idem.Key, idem.BizHash, idem.Intent); err != nil {
				   return err
			   }
		  }
		   for _, a := range m.alerts {
			  if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_alerts (id, kind, intent_id, detail, acked, at_ms) VALUES (?,?,?,?,?,?)`,
				   a.ID, a.Kind, a.IntentID, a.Detail, b2i(a.Acked), ms(a.At)); err != nil {
				   return err
			   }
		  }
		  for _, d := range m.desks {
			   if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_desk (id, intent_id, state, opened_by, approved_by, executed_by, reason, created_ms, updated_ms)
VALUES (?,?,?,?,?,?,?,?,?)`,
				   d.ID, d.IntentID, string(d.State), d.OpenedBy, d.ApprovedBy, d.ExecutedBy, d.Reason, ms(d.CreatedAt), ms(d.UpdatedAt)); err != nil {
				  return err
			   }
		  }
		   for _, h := range m.hints {
			   if _, err := q.ExecContext(ctx, `
INSERT INTO led_shard_hints (id, kind, intent_id, voucher_id, src_shard, dst_shard, asset, amount, at_ms)
VALUES (?,?,?,?,?,?,?,?,?)`,
				  h.ID, h.Kind, h.IntentID, h.VoucherID, h.SrcShard, h.DstShard, h.Asset, h.Amount, ms(h.At)); err != nil {
				  return err
			   }
		  }
		  return nil
	  })
   }

// LoadInto 在库里已有生效路由时覆盖内存目录。空库保留调用方刚建的引导数据。
   func (s *SQLStore) LoadInto(m *Mem) error {
	  if s == nil || s.db == nil || m == nil {
		  return nil
	   }
	  ctx := context.Background()
	  q := s.db.Conn(ctx)
	  var n int
	  if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM led_shard_routes`).Scan(&n); err != nil {
		   return err
	  }
	   if n == 0 {
		  return nil
	  }
	  next := newMem()
	  rows, err := q.QueryContext(ctx, `SELECT id, kind, status, weight, pin_subject, region, note, created_ms, updated_ms FROM led_shard_shards`)
	  if err != nil {
		  return err
	   }
	  for rows.Next() {
		   var sh Shard
		   var kind, status string
		  var created, updated int64
		  if err := rows.Scan(&sh.ID, &kind, &status, &sh.Weight, &sh.PinSubject, &sh.Region, &sh.Note, &created, &updated); err != nil {
			  rows.Close()
			  return err
		  }
		   sh.Kind = ShardKind(kind)
		  sh.Status = ShardStatus(status)
		  sh.CreatedAt = fromMS(created)
		  sh.UpdatedAt = fromMS(updated)
		  next.putShard(sh)
	  }
	   rows.Close()
	   if err := rows.Err(); err != nil {
		   return err
	   }
	   rrows, err := q.QueryContext(ctx, `SELECT version, status, modulus, user_shards, pins, affinity, fingerprint, note, created_ms, published_ms, activated_ms, retired_ms FROM led_shard_routes`)
	   if err != nil {
		   return err
	  }
	   for rrows.Next() {
		   var r RouteVersion
		   var status, ring, pins, aff string
		   var created, published, activated, retired int64
		   if err := rrows.Scan(&r.Version, &status, &r.Modulus, &ring, &pins, &aff, &r.Fingerprint, &r.Note, &created, &published, &activated, &retired); err != nil {
			   rrows.Close()
			  return err
		  }
		   r.Status = RouteStatus(status)
		  if err := json.Unmarshal([]byte(ring), &r.UserShards); err != nil {
			   rrows.Close()
			   return err
		   }
		  if err := json.Unmarshal([]byte(pins), &r.Pins); err != nil {
			   rrows.Close()
			   return err
		  }
		  if aff != "" && aff != "null" {
			   if err := json.Unmarshal([]byte(aff), &r.Affinity); err != nil {
				   rrows.Close()
				   return err
			  }
		   }
		   if r.Affinity == nil {
			   r.Affinity = map[string]string{}
		  }
		   r.CreatedAt = fromMS(created)
		  r.PublishedAt = fromMS(published)
		   r.ActivatedAt = fromMS(activated)
		   r.RetiredAt = fromMS(retired)
		  next.putRoute(r)
	   }
	  rrows.Close()
	  if err := rrows.Err(); err != nil {
		   return err
	  }
	  if err := s.loadIntents(ctx, q, next); err != nil {
		  return err
	   }
	  if err := s.loadChildren(ctx, q, next); err != nil {
		  return err
	  }
	  *m = *next
	   return nil
   }

   func (s *SQLStore) loadIntents(ctx context.Context, q persist.Queryer, m *Mem) error {
	   rows, err := q.QueryContext(ctx, `SELECT id, idem, biz_hash, route_version, route_fp, src_user, src_book, src_subject, src_shard,
 dst_user, dst_book, dst_subject, dst_shard, asset, amount, state, halted, manual, deadline_ms,
 freeze_posted, voucher_id, credit_posted, credit_count, reverse_count, reason, desk_id, created_ms, updated_ms
 FROM led_shard_intents`)
	  if err != nil {
		  return err
	   }
	  defer rows.Close()
	  for rows.Next() {
		  var it Intent
		  var amount sqlNum
		  var state string
		  var halted, manual, freeze, credit int
		   var deadline, created, updated int64
		  if err := rows.Scan(&it.ID,
			  &it.Idem, &it.BizHash, &it.RouteVersion, &it.RouteFP, &it.SrcUser, &it.SrcBook, &it.SrcSubject, &it.SrcShard,
			  &it.DstUser, &it.DstBook, &it.DstSubject, &it.DstShard, &it.Asset, &amount, &state, &halted, &manual, &deadline,
			  &freeze, &it.VoucherID, &credit, &it.CreditCount, &it.ReverseCount, &it.Reason, &it.DeskID, &created, &updated); err != nil {
			  return err
		   }
		   it.Amount = amount.String()
		  it.State = IntentState(state)
		  it.Halted = halted == 1
		  it.Manual = manual == 1
		   it.FreezePosted = freeze == 1
		   it.CreditPosted = credit == 1
		  it.Deadline = fromMS(deadline)
		  it.CreatedAt = fromMS(created)
		  it.UpdatedAt = fromMS(updated)
		   m.saveIntent(it)
		   m.idems[it.Idem] = IdemRec{Key: it.Idem, BizHash: it.BizHash, Intent: it.ID}
	   }
	   return rows.Err()
  }

   func (s *SQLStore) loadChildren(ctx context.Context, q persist.Queryer, m *Mem) error {
	   steps, err := q.QueryContext(ctx, `SELECT id, intent_id, seq, kind, state, shard_id, ledger_from, ledger_to, detail, at_ms FROM led_shard_steps ORDER BY intent_id, seq`)
	  if err != nil {
		   return err
	   }
	   for steps.Next() {
		  var st Step
		   var kind string
		  var at int64
		   if err := steps.Scan(&st.ID, &st.IntentID, &st.Seq, &kind, &st.State, &st.ShardID, &st.LedgerFrom, &st.LedgerTo, &st.Detail, &at); err != nil {
			  steps.Close()
			  return err
		   }
		  st.Kind = StepKind(kind)
		   st.At = fromMS(at)
		   m.addStep(st)
	  }
	   steps.Close()
	  if err := steps.Err(); err != nil {
		  return err
	   }
	  vs, err := q.QueryContext(ctx, `SELECT id, intent_id, idem, asset, amount, src_shard, dst_shard, bridge_shard, state, route_version, credit_count, created_ms, consumed_ms, voided_ms FROM led_shard_vouchers`)
	  if err != nil {
		   return err
	  }
	  for vs.Next() {
		   var v BridgeVoucher
		  var amount sqlNum
		  var state string
		  var created, consumed, voided int64
		  if err := vs.Scan(&v.ID, &v.IntentID, &v.Idem, &v.Asset, &amount, &v.SrcShard, &v.DstShard, &v.BridgeShard, &state, &v.RouteVersion, &v.CreditCount, &created, &consumed, &voided); err != nil {
			  vs.Close()
			  return err
		  }
		  v.Amount = amount.String()
		  v.State = VoucherState(state)
		  v.CreatedAt = fromMS(created)
		   v.ConsumedAt = fromMS(consumed)
		   v.VoidedAt = fromMS(voided)
		  m.putVoucher(v)
	  }
	   vs.Close()
	  if err := vs.Err(); err != nil {
		  return err
	   }
	   bs, err := q.QueryContext(ctx, `SELECT shard_id, subject, asset, available, frozen FROM led_shard_balances`)
	   if err != nil {
		   return err
	  }
	   for bs.Next() {
		  var b Balance
		  var avail, frozen sqlNum
		   if err := bs.Scan(&b.ShardID, &b.Subject, &b.Asset, &avail, &frozen); err != nil {
			   bs.Close()
			  return err
		  }
		  b.Available = avail.String()
		  b.Frozen = frozen.String()
		  m.saveBalance(b)
	   }
	  bs.Close()
	   if err := bs.Err(); err != nil {
		  return err
	  }
	  ps, err := q.QueryContext(ctx, `SELECT id, intent_id, voucher_id, seq, kind, shard_id, subject, asset, debit_avail, credit_avail, debit_frozen, credit_frozen, memo, at_ms FROM led_shard_postings ORDER BY intent_id, seq`)
	   if err != nil {
		   return err
	  }
	   for ps.Next() {
		  var p Posting
		   var kind string
		   var da, ca, df, cf sqlNum
		   var at int64
		   if err := ps.Scan(&p.ID, &p.IntentID, &p.VoucherID, &p.Seq, &kind, &p.ShardID, &p.Subject, &p.Asset, &da, &ca, &df, &cf, &p.Memo, &at); err != nil {
			   ps.Close()
			  return err
		   }
		  p.Kind = StepKind(kind)
		   p.DebitAvail = da.String()
		  p.CreditAvail = ca.String()
		  p.DebitFrozen = df.String()
		   p.CreditFrozen = cf.String()
		  p.At = fromMS(at)
		   m.posts[p.IntentID] = append(m.posts[p.IntentID],
			   p)
		  if p.Seq > m.postSeq[p.IntentID] {
			  m.postSeq[p.IntentID] = p.Seq
		  }
	   }
	   ps.Close()
	   if err := ps.Err(); err != nil {
		  return err
	  }
	   as, err := q.QueryContext(ctx, `SELECT id, kind, intent_id, detail, acked, at_ms FROM led_shard_alerts`)
	   if err != nil {
		   return err
	  }
	  for as.Next() {
		  var a Alert
		   var acked int
		   var at int64
		   if err := as.Scan(&a.ID, &a.Kind, &a.IntentID, &a.Detail, &acked, &at); err != nil {
			   as.Close()
			   return err
		   }
		  a.Acked = acked == 1
		   a.At = fromMS(at)
		  m.addAlert(a)
	   }
	   as.Close()
	   if err := as.Err(); err != nil {
		   return err
	   }
	   ds, err := q.QueryContext(ctx, `SELECT id, intent_id, state, opened_by, approved_by, executed_by, reason, created_ms, updated_ms FROM led_shard_desk`)
	  if err != nil {
		   return err
	   }
	  for ds.Next() {
		  var d DeskCase
		   var state string
		   var created, updated int64
		   if err := ds.Scan(&d.ID, &d.IntentID, &state, &d.OpenedBy, &d.ApprovedBy, &d.ExecutedBy, &d.Reason, &created, &updated); err != nil {
			  ds.Close()
			  return err
		  }
		  d.State = DeskState(state)
		   d.CreatedAt = fromMS(created)
		   d.UpdatedAt = fromMS(updated)
		  m.putDesk(d)
	   }
	   ds.Close()
	   if err := ds.Err(); err != nil {
		   return err
	   }
	  hs, err := q.QueryContext(ctx, `SELECT id, kind, intent_id, voucher_id, src_shard, dst_shard, asset, amount, at_ms FROM led_shard_hints`)
	   if err != nil {
		   return err
	   }
	   defer hs.Close()
	   for hs.Next() {
		   var h Hint
		   var amount sqlNum
		   var at int64
		   if err := hs.Scan(&h.ID, &h.Kind, &h.IntentID, &h.VoucherID, &h.SrcShard, &h.DstShard, &h.Asset, &amount, &at); err != nil {
			  return err
		   }
		   h.Amount = amount.String()
		  h.At = fromMS(at)
		   m.hints = append(m.hints, h)
	  }
	  return hs.Err()
  }

var snapshotTables = []string{
	"led_shard_hints",
	"led_shard_desk",
	"led_shard_alerts",
	"led_shard_idem",
	"led_shard_postings",
	"led_shard_balances",
	"led_shard_vouchers",
	"led_shard_steps",
	"led_shard_intents",
	"led_shard_routes",
	"led_shard_shards",
}

  func ms(t time.Time) int64 {
	   if t.IsZero() {
		  return 0
	  }
	   return t.UTC().UnixMilli()
   }

   func fromMS(n int64) time.Time {
	  if n == 0 {
		  return time.Time{}
	   }
	   return time.UnixMilli(n).UTC()
  }

   func b2i(v bool) int {
	  if v {
		   return 1
	  }
	   return 0
  }

type sqlNum string

   func (n *sqlNum) Scan(v any) error {
	  switch t := v.(type) {
	  case nil:
		   *n = "0"
	   case string:
		  *n = sqlNum(t)
	  case []byte:
		  *n = sqlNum(string(t))
	  case int64:
		   *n = sqlNum(strconv.FormatInt(t, 10))
	  case float64:
		  *n = sqlNum(strconv.FormatFloat(t, 'f', -1, 64))
	   default:
		  return sql.ErrNoRows
	  }
	  return nil
  }

  func (n sqlNum) String() string {
	  s := strings.TrimSpace(string(n))
	   if s == "" {
		   return "0"
	  }
	  return s
   }
