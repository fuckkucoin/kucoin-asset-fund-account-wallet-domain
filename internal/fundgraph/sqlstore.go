package fundgraph

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"github.com/go-sql-driver/mysql"
)

// SQLStore 把因果图落在 fund_graph_* 表。冲突检测在事务内完成，失败不覆盖原边。
type SQLStore struct {
	db  *sql.DB
	mu  sync.Mutex
	now func() time.Time
}

const (
	nodeCols = `id, kind, ref, user_id, asset, amount, chain, tx_hash, source, occurred_at, ingested_at, attrs_json`
	edgeCols = `id, from_id, to_id, edge_type, ref, user_id, asset, amount, source, envelope_id, occurred_at, ingested_at, attrs_json`
	curCols  = `source, stream, cursor_token, last_event_at, updated_at, lag_ms`
	incCols = `id, kind, status, reason, edge_key, node_id, ref, detail, opened_at, updated_at`
	audCols = `id, action, edge_key, node_id, detail, at`
)

// NewSQLStore 包装一个已打开的库连接。

  func NewSQLStore(db *sql.DB) *SQLStore {
	   return &SQLStore{db: db, now: func() time.Time { return time.Now().UTC() }}
  }

// OpenMySQL 打开连接并探活。调用方负责迁移。
   func OpenMySQL(dsn string) (*SQLStore, error) {
	   db, err := sql.Open("mysql", dsn)
	   if err != nil {
		  return nil, err
	   }
	  db.SetMaxOpenConns(16)
	  db.SetMaxIdleConns(4)
	  db.SetConnMaxLifetime(30 * time.Minute)
	  ctx,
		  cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	   if err := db.PingContext(ctx); err != nil {
		  _ = db.Close()
		  return nil, err
	  }
	   return NewSQLStore(db), nil
   }

// DB 返回底层连接，便于执行建表脚本。
  func (s *SQLStore) DB() *sql.DB {
	  if s == nil {
		   return nil
	  }
	   return s.db
   }

// Close 关闭连接。

   func (s *SQLStore) Close() error {
	  if s == nil || s.db == nil {
		   return nil
	  }
	   return s.db.Close()
   }

// SetClock 替换写入时间。
  func (s *SQLStore) SetClock(fn func() time.Time) {
	   if s == nil || fn == nil {
		  return
	   }
	   s.mu.Lock()
	   s.now = fn
	  s.mu.Unlock()
   }

   func (s *SQLStore) clock() time.Time {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if s.now == nil {
		  return time.Now().UTC()
	   }
	  return s.now().UTC()
   }

   func isDupKey(err error) bool {
	   var me *mysql.MySQLError
	   return errors.As(err, &me) && me.Number == 1062
   }

   func marshalAttrs(m map[string]string) string {
	   if m == nil {
		  m = map[string]string{}
	  }
	   b, err := json.Marshal(m)
	  if err != nil {
		   return "{}"
	   }
	   return string(b)
   }

   func unmarshalAttrs(s string) map[string]string {
	   s = strings.TrimSpace(s)
	  if s == "" || s == "null" || s == "{}" {
		  return nil
	   }
	   var m map[string]string
	   if err := json.Unmarshal([]byte(s), &m); err != nil {
		   return nil
	  }
	  if len(m) == 0 {
		   return nil
	   }
	   return m
  }

  func nullTime(t time.Time) sql.NullTime {
	   if t.IsZero() {
		  return sql.NullTime{}
	  }
	   return sql.NullTime{Time: t.UTC(), Valid: true}
  }

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanNode(sc rowScanner) (FundNode, error) {
	   var n FundNode
	   var kind, attrs string
	  err := sc.Scan(&n.ID,
		  &kind, &n.Ref, &n.UserID, &n.Asset, &n.Amount, &n.Chain, &n.TxHash, &n.Source, &n.OccurredAt, &n.IngestedAt, &attrs)
	   if err != nil {
		  return FundNode{}, err
	  }
	   n.Kind = NodeKind(kind)
	   n.Attrs = unmarshalAttrs(attrs)
	  n.OccurredAt = n.OccurredAt.UTC()
	   n.IngestedAt = n.IngestedAt.UTC()
	  return n, nil
   }

   func scanEdge(sc rowScanner) (FundEdge, error) {
	  var e FundEdge
	  var typ, attrs string
	  err := sc.Scan(&e.ID, &e.FromID, &e.ToID, &typ, &e.Ref, &e.UserID, &e.Asset, &e.Amount, &e.Source, &e.EnvelopeID, &e.OccurredAt, &e.IngestedAt, &attrs)
	   if err != nil {
		   return FundEdge{}, err
	  }
	   e.EdgeType = EdgeType(typ)
	   e.Attrs = unmarshalAttrs(attrs)
	   e.OccurredAt = e.OccurredAt.UTC()
	   e.IngestedAt = e.IngestedAt.UTC()
	   return e, nil
   }

  func scanCursor(sc rowScanner) (Cursor, error) {
	  var c Cursor
	  var last sql.NullTime
	  err := sc.Scan(&c.Source, &c.Stream, &c.Token, &last, &c.UpdatedAt, &c.LagMs)
	   if err != nil {
		   return Cursor{}, err
	  }
	  if last.Valid {
		  c.LastEventAt = last.Time.UTC()
	   }
	   c.UpdatedAt = c.UpdatedAt.UTC()
	  return c, nil
   }

   func scanIncident(sc rowScanner) (GraphIncident, error) {
	  var inc GraphIncident
	   err := sc.Scan(&inc.ID, &inc.Kind, &inc.Status, &inc.Reason, &inc.EdgeKey, &inc.NodeID, &inc.Ref, &inc.Detail, &inc.OpenedAt, &inc.UpdatedAt)
	   if err != nil {
		   return GraphIncident{}, err
	   }
	  inc.OpenedAt = inc.OpenedAt.UTC()
	   inc.UpdatedAt = inc.UpdatedAt.UTC()
	  return inc, nil
  }

  func scanAudit(sc rowScanner) (AuditRecord, error) {
	  var a AuditRecord
	   err := sc.Scan(&a.ID, &a.Action, &a.EdgeKey, &a.NodeID, &a.Detail, &a.At)
	   if err != nil {
		  return AuditRecord{}, err
	  }
	  a.At = a.At.UTC()
	   return a, nil
  }

   func (s *SQLStore) Ingest(ctx context.Context, b IngestBatch) (IngestResult, error) {
	   if s == nil || s.db == nil {
		  return IngestResult{},
			  ErrInvalid
	  }
	   now := s.clock()
	   prepared, err := prepareBatch(b, now)
	   if err != nil {
		   return IngestResult{}, err
	   }
	   tx, err := s.db.BeginTx(ctx, nil)
	  if err != nil {
		  return IngestResult{}, err
	   }
	   committed := false
	  defer func() {
		   if !committed {
			   _ = tx.Rollback()
		  }
	   }()
	  var conflicts []Conflict
	   seenN := map[string]FundNode{}
	   for _, n := range prepared.Nodes {
		  if prev, ok := seenN[n.ID]; ok {
			   conflicts = append(conflicts, diffNode(prev, n)...)
			  continue
		   }
		   old, ok, err := nodeByIDTx(ctx, tx, n.ID)
		   if err != nil {
			   return IngestResult{}, err
		   }
		   if ok {
			  conflicts = append(conflicts, diffNode(old, n)...)
		  }
		  seenN[n.ID] = n
	   }
	   seenE := map[string]FundEdge{}
	  seenEID := map[string]FundEdge{}
	  for _, e := range prepared.Edges {
		  ks := e.Key().String()
		   if prev, ok := seenE[ks]; ok {
			  conflicts = append(conflicts, diffEdge(prev, e)...)
			  continue
		   }
		  if prev, ok := seenEID[e.ID]; ok && prev.Key().String() != ks {
			  _,
				   cs := edgeReplay(prev, e, true)
			   conflicts = append(conflicts, cs...)
			   continue
		  }
		  old, ok, err := edgeByKeyTx(ctx, tx, e.Key())
		   if err != nil {
			   return IngestResult{}, err
		   }
		   if ok {
			   conflicts = append(conflicts, diffEdge(old, e)...)
		  } else {
			   byID, okID, err := edgeByIDTx(ctx, tx, e.ID)
			   if err != nil {
				  return IngestResult{}, err
			  }
			   if okID {
				  _, cs := edgeReplay(byID, e, true)
				  conflicts = append(conflicts, cs...)
			  }
		   }
		   seenE[ks] = e
		   seenEID[e.ID] = e
	  }
	   if len(conflicts) > 0 {
		   return s.rejectConflict(ctx, tx, &committed, conflicts, now)
	   }
	   res := IngestResult{}
	   wroteN := map[string]bool{}
	  for _, n := range prepared.Nodes {
		   if wroteN[n.ID] {
			  res.ReplayNodes++
			  continue
		  }
		  if _, ok := seenN[n.ID]; ok {
			   _,
				  exists, err := nodeByIDTx(ctx, tx, n.ID)
			   if err != nil {
				  return IngestResult{}, err
			  }
			   if exists {
				   res.ReplayNodes++
				  wroteN[n.ID] = true
				  continue
			   }
		  }
		   fresh := n
		   fresh.OccurredAt = stampOccurred(n.OccurredAt, now)
		   if err := insertNodeTx(ctx, tx, fresh); err != nil {
			   if isDupKey(err) {
				   old, ok, rerr := nodeByIDTx(ctx, tx, n.ID)
				  if rerr != nil {
					   return IngestResult{}, rerr
				   }
				  replay,
					   cs := nodeReplay(old, n, ok)
				  if !replay {
					   return s.rejectConflict(ctx, tx, &committed, cs, now)
				  }
				   res.ReplayNodes++
				  wroteN[n.ID] = true
				  continue
			   }
			   return IngestResult{}, err
		  }
		   res.InsertedNodes++
		  wroteN[n.ID] = true
	  }
	   wroteE := map[string]bool{}
	  for _, e := range prepared.Edges {
		  ks := e.Key().String()
		  if wroteE[ks] {
			  res.ReplayEdges++
			  continue
		   }
		   _, exists, err := edgeByKeyTx(ctx, tx, e.Key())
		  if err != nil {
			   return IngestResult{}, err
		   }
		  if exists {
			   res.ReplayEdges++
			   wroteE[ks] = true
			  continue
		   }
		  fresh := e
		  fresh.OccurredAt = stampOccurred(e.OccurredAt, now)
		  if err := insertEdgeTx(ctx, tx, fresh); err != nil {
			  if isDupKey(err) {
				  old, ok, rerr := edgeByKeyTx(ctx, tx, e.Key())
				  if rerr != nil {
					   return IngestResult{}, rerr
				  }
				   if !ok {
					  old,
						  ok, rerr = edgeByIDTx(ctx, tx, e.ID)
					   if rerr != nil {
						  return IngestResult{}, rerr
					  }
				  }
				   replay, cs := edgeReplay(old, e, ok)
				   if !replay {
					   return s.rejectConflict(ctx, tx, &committed, cs, now)
				   }
				   res.ReplayEdges++
				  wroteE[ks] = true
				  continue
			   }
			  return IngestResult{}, err
		   }
		  res.InsertedEdges++
		   wroteE[ks] = true
	   }
	   if prepared.Source != "" {
		  prev, _, err := cursorByKeyTx(ctx, tx, prepared.Source, prepared.Stream)
		   if err != nil {
			   return IngestResult{}, err
		   }
		  cur := cursorFromBatch(prepared, prev, now)
		  if err := upsertCursorTx(ctx, tx, cur); err != nil {
			  return IngestResult{}, err
		  }
		  res.Cursor = cur
	  }
	   if err := tx.Commit(); err != nil {
		  return IngestResult{}, err
	  }
	  committed = true
	   return res, nil
   }

   func nodeByIDTx(ctx context.Context, tx *sql.Tx, id string) (FundNode, bool, error) {
	   row := tx.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM fund_graph_nodes WHERE id = ? FOR UPDATE`, id)
	  n, err := scanNode(row)
	   if errors.Is(err, sql.ErrNoRows) {
		  return FundNode{}, false, nil
	   }
	   if err != nil {
		  return FundNode{}, false, err
	  }
	  return n, true, nil
  }

  func (s *SQLStore) rejectConflict(ctx context.Context, tx *sql.Tx, committed *bool, conflicts []Conflict, now time.Time) (IngestResult, error) {
	  _ = tx.Rollback()
	  *committed = true
	  if len(conflicts) == 0 {
		   return IngestResult{}, ErrConflict
	  }
	   rec := AuditRecord{
		   ID:      AuditID(ActionConflict, conflicts[0].EdgeKey, conflicts[0].NodeID, now),
		   Action:  ActionConflict,
		  EdgeKey: conflicts[0].EdgeKey,
		   NodeID:  conflicts[0].NodeID,
		   Detail:  conflictsJSON(conflicts),
		   At: now,
	  }
	   if err := s.AppendAudit(ctx, rec); err != nil {
		  return IngestResult{Conflicts: conflicts}, err
	  }
	  return IngestResult{Conflicts: conflicts, Audited: true}, ErrConflict
   }

  func edgeByIDTx(ctx context.Context, tx *sql.Tx, id string) (FundEdge, bool, error) {
	  row := tx.QueryRowContext(ctx, `SELECT `+edgeCols+` FROM fund_graph_edges WHERE id = ? FOR UPDATE`, id)
	   e, err := scanEdge(row)
	  if errors.Is(err, sql.ErrNoRows) {
		   return FundEdge{}, false, nil
	   }
	  if err != nil {
		  return FundEdge{}, false, err
	   }
	   return e, true, nil
   }

   func edgeByKeyTx(ctx context.Context, tx *sql.Tx, key EdgeKey) (FundEdge, bool, error) {
	  row := tx.QueryRowContext(ctx, `SELECT `+edgeCols+` FROM fund_graph_edges WHERE from_id = ? AND to_id = ? AND edge_type = ? AND ref = ? FOR UPDATE`, key.From, key.To, string(key.Type), key.Ref)
	   e, err := scanEdge(row)
	   if errors.Is(err, sql.ErrNoRows) {
		  return FundEdge{}, false, nil
	  }
	   if err != nil {
		  return FundEdge{}, false, err
	  }
	  return e, true, nil
   }

  func cursorByKeyTx(ctx context.Context, tx *sql.Tx, source, stream string) (Cursor, bool, error) {
	  row := tx.QueryRowContext(ctx, `SELECT `+curCols+` FROM fund_graph_cursors WHERE source = ? AND stream = ? FOR UPDATE`, source, stream)
	   c, err := scanCursor(row)
	   if errors.Is(err, sql.ErrNoRows) {
		   return Cursor{}, false, nil
	   }
	   if err != nil {
		  return Cursor{}, false, err
	   }
	   return c, true, nil
   }

  func insertNodeTx(ctx context.Context, tx *sql.Tx, n FundNode) error {
	   _, err := tx.ExecContext(ctx, `INSERT INTO fund_graph_nodes (`+nodeCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		  n.ID,
		   string(n.Kind), n.Ref, n.UserID, n.Asset, n.Amount, n.Chain, n.TxHash, n.Source, n.OccurredAt.UTC(), n.IngestedAt.UTC(), marshalAttrs(n.Attrs))
	  return err
  }

   func insertEdgeTx(ctx context.Context, tx *sql.Tx, e FundEdge) error {
	   _, err := tx.ExecContext(ctx, `INSERT INTO fund_graph_edges (`+edgeCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   e.ID, e.FromID, e.ToID, string(e.EdgeType), e.Ref, e.UserID, e.Asset, e.Amount, e.Source, e.EnvelopeID, e.OccurredAt.UTC(), e.IngestedAt.UTC(), marshalAttrs(e.Attrs))
	   return err
   }

   func upsertCursorTx(ctx context.Context, tx *sql.Tx, c Cursor) error {
	   _, err := tx.ExecContext(ctx, `INSERT INTO fund_graph_cursors (source, stream, cursor_token, last_event_at, updated_at, lag_ms)
		VALUES (?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE cursor_token=VALUES(cursor_token), last_event_at=VALUES(last_event_at), updated_at=VALUES(updated_at), lag_ms=VALUES(lag_ms)`,
		  c.Source,
		  c.Stream, c.Token, nullTime(c.LastEventAt), c.UpdatedAt.UTC(), c.LagMs)
	   return err
  }

  func (s *SQLStore) GetNode(ctx context.Context, id string) (FundNode, bool, error) {
	   row := s.db.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM fund_graph_nodes WHERE id = ?`, id)
	   n, err := scanNode(row)
	  if errors.Is(err, sql.ErrNoRows) {
		  return FundNode{}, false, nil
	  }
	  if err != nil {
		   return FundNode{}, false, err
	   }
	  return n, true, nil
  }

   func (s *SQLStore) ListNodes(ctx context.Context, f NodeFilter) ([]FundNode, error) {
	  q := `SELECT ` + nodeCols + ` FROM fund_graph_nodes`
	   where, args := nodeWhere(f)
	   if len(where) > 0 {
		  q += " WHERE " + strings.Join(where, " AND ")
	   }
	   q += " ORDER BY occurred_at, id LIMIT ?"
	  lim := f.Limit
	   if lim <= 0 || lim > 50000 {
		   lim = 50000
	   }
	  args = append(args, lim)
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []FundNode
	   for rows.Next() {
		   n, err := scanNode(rows)
		  if err != nil {
			   return nil, err
		  }
		  if f.Ref != "" && n.Ref != f.Ref && n.Corr() != f.Ref {
			  continue
		   }
		  out = append(out, n)
	   }
	   if out == nil {
		   out = []FundNode{}
	   }
	   return out, rows.Err()
   }

  func nodeWhere(f NodeFilter) ([]string, []any) {
	  var where []string
	   var args []any
	  if f.UserID != "" {
		  where = append(where, "user_id = ?")
		   args = append(args, f.UserID)
	   }
	  if f.Asset != "" {
		   where = append(where, "asset = ?")
		  args = append(args,
			   NormAsset(f.Asset))
	   }
	  if f.Kind != "" {
		  where = append(where, "kind = ?")
		   args = append(args, string(f.Kind))
	   }
	   if !f.From.IsZero() {
		  where = append(where, "occurred_at >= ?")
		   args = append(args, f.From.UTC())
	   }
	  if !f.To.IsZero() {
		   where = append(where, "occurred_at <= ?")
		   args = append(args, f.To.UTC())
	  }
	   if f.Ref != "" {
		   where = append(where, "(ref = ? OR attrs_json LIKE ?)")
		  args = append(args, f.Ref, "%\"corr\":\""+escapeLike(f.Ref)+"\"%")
	   }
	  return where, args
  }

   func escapeLike(s string) string {
	  s = strings.ReplaceAll(s, `\`, `\\`)
	  s = strings.ReplaceAll(s, `%`, `\%`)
	   s = strings.ReplaceAll(s, `_`, `\_`)
	   return s
   }

  func (s *SQLStore) GetEdge(ctx context.Context, id string) (FundEdge, bool, error) {
	   row := s.db.QueryRowContext(ctx, `SELECT `+edgeCols+` FROM fund_graph_edges WHERE id = ?`, id)
	  e, err := scanEdge(row)
	  if errors.Is(err, sql.ErrNoRows) {
		   return FundEdge{}, false, nil
	   }
	   if err != nil {
		   return FundEdge{}, false, err
	  }
	  return e, true, nil
   }

  func (s *SQLStore) EdgesFrom(ctx context.Context, nodeID string) ([]FundEdge, error) {
	  return s.queryEdges(ctx, `SELECT `+edgeCols+` FROM fund_graph_edges WHERE from_id = ? ORDER BY occurred_at, id`, nodeID)
  }

  func (s *SQLStore) EdgesTo(ctx context.Context, nodeID string) ([]FundEdge, error) {
	   return s.queryEdges(ctx, `SELECT `+edgeCols+` FROM fund_graph_edges WHERE to_id = ? ORDER BY occurred_at, id`, nodeID)
   }

  func (s *SQLStore) queryEdges(ctx context.Context, q string, args ...any) ([]FundEdge, error) {
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := []FundEdge{}
	   for rows.Next() {
		  e, err := scanEdge(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, e)
	  }
	   return out, rows.Err()
   }

  func (s *SQLStore) ListEdges(ctx context.Context, f EdgeFilter) ([]FundEdge, error) {
	  q := `SELECT ` + edgeCols + ` FROM fund_graph_edges`
	  var where []string
	  var args []any
	  if f.Ref != "" {
		   where = append(where, "ref = ?")
		  args = append(args, f.Ref)
	  }
	  if f.FromID != "" {
		  where = append(where, "from_id = ?")
		  args = append(args, f.FromID)
	  }
	   if f.ToID != "" {
		  where = append(where, "to_id = ?")
		   args = append(args, f.ToID)
	  }
	   if f.EdgeType != "" {
		   where = append(where, "edge_type = ?")
		  args = append(args, string(f.EdgeType))
	  }
	   if f.UserID != "" {
		   where = append(where, "user_id = ?")
		  args = append(args, f.UserID)
	  }
	  if f.Asset != "" {
		  where = append(where, "asset = ?")
		   args = append(args, NormAsset(f.Asset))
	  }
	  if len(where) > 0 {
		   q += " WHERE " + strings.Join(where, " AND ")
	   }
	  q += " ORDER BY occurred_at, id LIMIT ?"
	   lim := f.Limit
	   if lim <= 0 || lim > 50000 {
		   lim = 50000
	  }
	   args = append(args, lim)
	   return s.queryEdges(ctx, q, args...)
   }

  func (s *SQLStore) Counts(ctx context.Context) (int, int, error) {
	   var nodes, edges int
	   if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fund_graph_nodes`).Scan(&nodes); err != nil {
		  return 0, 0, err
	   }
	   if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fund_graph_edges`).Scan(&edges); err != nil {
		  return 0, 0, err
	   }
	   return nodes, edges, nil
  }

   func (s *SQLStore) Snapshot(ctx context.Context) (Snapshot, error) {
	   snap := Snapshot{}
	  nodes, err := s.queryNodes(ctx, `SELECT `+nodeCols+` FROM fund_graph_nodes ORDER BY id LIMIT ?`, SnapshotCap+1)
	  if err != nil {
		   return Snapshot{}, err
	  }
	  if len(nodes) > SnapshotCap {
		  snap.Truncated = true
		   nodes = nodes[:SnapshotCap]
	  }
	   edges, err := s.queryEdges(ctx, `SELECT `+edgeCols+` FROM fund_graph_edges ORDER BY id LIMIT ?`, SnapshotCap+1)
	  if err != nil {
		  return Snapshot{}, err
	  }
	  if len(edges) > SnapshotCap {
		  snap.Truncated = true
		  edges = edges[:SnapshotCap]
	   }
	   curs, err := s.ListCursors(ctx)
	  if err != nil {
		   return Snapshot{}, err
	   }
	  snap.Nodes = nodes
	   snap.Edges = edges
	  snap.Cursors = curs
	  return snap, nil
   }

   func (s *SQLStore) queryNodes(ctx context.Context, q string, args ...any) ([]FundNode, error) {
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := []FundNode{}
	  for rows.Next() {
		  n, err := scanNode(rows)
		   if err != nil {
			  return nil, err
		   }
		   out = append(out, n)
	   }
	   return out, rows.Err()
   }

   func (s *SQLStore) PutCursor(ctx context.Context, c Cursor) error {
	  if c.Source == "" {
		   return ErrInvalid
	  }
	   if c.Stream == "" {
		  c.Stream = "default"
	   }
	   if c.UpdatedAt.IsZero() {
		   c.UpdatedAt = s.clock()
	  }
	   _, err := s.db.ExecContext(ctx, `INSERT INTO fund_graph_cursors (source, stream, cursor_token, last_event_at, updated_at, lag_ms)
		VALUES (?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE cursor_token=VALUES(cursor_token), last_event_at=VALUES(last_event_at), updated_at=VALUES(updated_at), lag_ms=VALUES(lag_ms)`,
		   c.Source, c.Stream, c.Token, nullTime(c.LastEventAt), c.UpdatedAt.UTC(), c.LagMs)
	   return err
  }

   func (s *SQLStore) GetCursor(ctx context.Context, source, stream string) (Cursor, bool, error) {
	  if stream == "" {
		   stream = "default"
	   }
	   row := s.db.QueryRowContext(ctx, `SELECT `+curCols+` FROM fund_graph_cursors WHERE source = ? AND stream = ?`, source, stream)
	  c, err := scanCursor(row)
	   if errors.Is(err, sql.ErrNoRows) {
		  return Cursor{}, false, nil
	   }
	  if err != nil {
		   return Cursor{}, false, err
	  }
	  return c, true, nil
   }

  func (s *SQLStore) ListCursors(ctx context.Context) ([]Cursor, error) {
	   rows, err := s.db.QueryContext(ctx, `SELECT `+curCols+` FROM fund_graph_cursors ORDER BY source, stream`)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := []Cursor{}
	  for rows.Next() {
		   c, err := scanCursor(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, c)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) OpenIncident(ctx context.Context, inc GraphIncident) (GraphIncident, bool, error) {
	   if inc.Kind == "" || inc.Reason == "" {
		  return GraphIncident{}, false, ErrInvalid
	  }
	  if inc.ID == "" {
		   inc.ID = IncidentID(inc.Kind, inc.Reason, inc.Ref, inc.NodeID, inc.EdgeKey)
	  }
	   now := s.clock()
	  if inc.OpenedAt.IsZero() {
		  inc.OpenedAt = now
	  }
	   if inc.UpdatedAt.IsZero() {
		   inc.UpdatedAt = inc.OpenedAt
	   }
	   inc.Status = StatusOpen
	  tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		   return GraphIncident{}, false, err
	  }
	  defer func() { _ = tx.Rollback() }()
	   row := tx.QueryRowContext(ctx, `SELECT `+incCols+` FROM fund_graph_incidents WHERE id = ? FOR UPDATE`, inc.ID)
	   old, err := scanIncident(row)
	  if err != nil && !errors.Is(err, sql.ErrNoRows) {
		   return GraphIncident{}, false, err
	  }
	  if err == nil {
		  if old.Status == StatusOpen || old.Status == StatusAck {
			  return old, false, nil
		  }
		  _, err = tx.ExecContext(ctx, `UPDATE fund_graph_incidents SET status=?, detail=?, opened_at=?, updated_at=? WHERE id=?`,
			   StatusOpen, inc.Detail, inc.OpenedAt.UTC(), inc.UpdatedAt.UTC(), inc.ID)
		  if err != nil {
			  return GraphIncident{}, false, err
		  }
		  if err := tx.Commit(); err != nil {
			  return GraphIncident{}, false, err
		   }
		  inc.Status = StatusOpen
		  return inc, true, nil
	  }
	   _, err = tx.ExecContext(ctx, `INSERT INTO fund_graph_incidents (`+incCols+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		   inc.ID, inc.Kind, StatusOpen, inc.Reason, inc.EdgeKey, inc.NodeID, inc.Ref, inc.Detail, inc.OpenedAt.UTC(), inc.UpdatedAt.UTC())
	  if err != nil {
		   return GraphIncident{}, false, err
	   }
	   if err := tx.Commit(); err != nil {
		   return GraphIncident{},
			  false, err
	   }
	  return inc, true, nil
   }

  func (s *SQLStore) ListIncidents(ctx context.Context, f IncidentFilter) ([]GraphIncident, error) {
	  q := `SELECT ` + incCols + ` FROM fund_graph_incidents`
	   var where []string
	  var args []any
	   if f.Status != "" {
		   where = append(where, "status = ?")
		   args = append(args, f.Status)
	   }
	  if f.Kind != "" {
		  where = append(where, "kind = ?")
		   args = append(args, f.Kind)
	   }
	  if f.Ref != "" {
		  where = append(where, "ref = ?")
		   args = append(args, f.Ref)
	   }
	   if len(where) > 0 {
		  q += " WHERE " + strings.Join(where, " AND ")
	  }
	   q += " ORDER BY opened_at, id LIMIT ?"
	   lim := f.Limit
	  if lim <= 0 || lim > 50000 {
		  lim = 50000
	  }
	   args = append(args, lim)
	   rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := []GraphIncident{}
	  for rows.Next() {
		  inc, err := scanIncident(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, inc)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) SetIncidentStatus(ctx context.Context, id, status string, at time.Time) error {
	  if status != StatusAck && status != StatusClosed && status != StatusOpen {
		   return ErrStatus
	  }
	   tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		  return err
	  }
	   defer func() { _ = tx.Rollback() }()
	  row := tx.QueryRowContext(ctx, `SELECT `+incCols+` FROM fund_graph_incidents WHERE id = ? FOR UPDATE`, id)
	   inc, err := scanIncident(row)
	  if errors.Is(err, sql.ErrNoRows) {
		  return ErrNotFound
	   }
	   if err != nil {
		   return err
	  }
	   if !statusAllowed(inc.Status, status) {
		  return ErrStatus
	   }
	  if at.IsZero() {
		  at = s.clock()
	  }
	  if _, err := tx.ExecContext(ctx, `UPDATE fund_graph_incidents SET status=?, updated_at=? WHERE id=?`, status, at.UTC(), id); err != nil {
		   return err
	  }
	  return tx.Commit()
  }

  func (s *SQLStore) AppendAudit(ctx context.Context, a AuditRecord) error {
	   if a.At.IsZero() {
		  a.At = s.clock()
	   }
	  if a.ID == "" {
		  a.ID = AuditID(a.Action, a.EdgeKey, a.NodeID, a.At)
	   }
	  _, err := s.db.ExecContext(ctx, `INSERT INTO fund_graph_audit (`+audCols+`) VALUES (?,?,?,?,?,?)`,
		  a.ID, a.Action, a.EdgeKey, a.NodeID, a.Detail, a.At.UTC())
	  if isDupKey(err) {
		  return nil
	  }
	  return err
   }

  func (s *SQLStore) ListAudit(ctx context.Context, edgeKey string, limit int) ([]AuditRecord, error) {
	  q := `SELECT ` + audCols + ` FROM fund_graph_audit`
	  var args []any
	  if edgeKey != "" {
		  q += ` WHERE edge_key = ?`
		   args = append(args, edgeKey)
	   }
	  q += ` ORDER BY at, id`
	   if limit <= 0 || limit > 5000 {
		   limit = 500
	  }
	   q += ` LIMIT ?`
	  args = append(args, limit)
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := []AuditRecord{}
	   for rows.Next() {
		  a, err := scanAudit(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, a)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) SetEdgeTier(ctx context.Context, id, tier string, at time.Time) error {
	   e, ok, err := s.GetEdge(ctx, id)
	  if err != nil {
		  return err
	  }
	  if !ok {
		   return ErrNotFound
	  }
	   if e.Attrs == nil {
		   e.Attrs = map[string]string{}
	  }
	   e.Attrs["tier"] = tier
	  if at.IsZero() {
		   at = s.clock()
	  }
	   e.Attrs["tier_at"] = at.UTC().Format(time.RFC3339Nano)
	   res, err := s.db.ExecContext(ctx, `UPDATE fund_graph_edges SET attrs_json = ? WHERE id = ?`, marshalAttrs(e.Attrs), id)
	  if err != nil {
		   return err
	  }
	  n, err := res.RowsAffected()
	   if err != nil {
		  return err
	  }
	   if n == 0 {
		  return ErrNotFound
	  }
	   return nil
  }

  func (s *SQLStore) DeleteEdges(ctx context.Context, ids []string) error {
	  tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		  return err
	  }
	  defer func() { _ = tx.Rollback() }()
	   for _, id := range ids {
		   if strings.TrimSpace(id) == "" {
			  continue
		   }
		  if _, err := tx.ExecContext(ctx, `DELETE FROM fund_graph_edges WHERE id = ?`, id); err != nil {
			   return err
		   }
	   }
	   return tx.Commit()
   }
