// 领域服务：业务用例编排与校验。

// Author: Cason
// Date: 2021.06.07

package fundgraph

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Service 对外提供摄取、血缘查询、巡检和保留计划。写入只经过 Store。
type Service struct {
	store Store
	cfg Config
	mu  sync.Mutex
	now func() time.Time
	conflicts int64
	ingests int64
	replays int64
	queries int64
	lastQuery int64
}

// New 装配服务。store 为空时使用进程内图。

  func New(store Store, cfg Config) *Service {
	   if store == nil {
		  store = NewMemStore()
	  }
	  return &Service{
		   store: store,
		  cfg:   cfg.norm(),
		   now:   func() time.Time { return time.Now().UTC() },
	   }
   }

// SetClock 同时替换服务和存储的时钟（存储 SetClock 时）。

   func (s *Service) SetClock(fn func() time.Time) {
	  if s == nil || fn == nil {
		  return
	   }
	  s.mu.Lock()
	   s.now = fn
	  s.mu.Unlock()
	   if c, ok := s.store.(interface{ SetClock(func() time.Time) }); ok {
		  c.SetClock(fn)
	  }
   }

  func (s *Service) clock() time.Time {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if s.now == nil {
		  return time.Now().UTC()
	  }
	  return s.now().UTC()
   }

// Config 返回生效配置的副本。
  func (s *Service) Config() Config {
	   if s == nil {
		   return DefaultConfig()
	   }
	   return s.cfg
  }

// Ingest 写入一批事实。冲突时返回 ErrConflict，图内容保持原样。
   func (s *Service) Ingest(ctx context.Context, b IngestBatch) (IngestResult, error) {
	  if s == nil {
		  return IngestResult{}, ErrInvalid
	  }
	   res, err := s.store.Ingest(ctx, b)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if err == nil {
		   s.ingests++
		   s.replays += int64(res.ReplayEdges + res.ReplayNodes)
	   }
	   if errors.Is(err, ErrConflict) {
		  s.conflicts++
	   }
	   return res, err
  }

// Lineage 执行正向影响、反向根因或最短责任路径。超限会被截断，缺边写入缺口标记。
  func (s *Service) Lineage(ctx context.Context, q LineageQuery) (LineageResult, error) {
	  if s == nil {
		   return LineageResult{}, ErrInvalid
	  }
	  started := time.Now()
	   defer func() {
		   s.noteQuery(time.Since(started))
	   }()
	   q = normalizeQuery(q)
	  maxHops, limit, clamped := s.bounds(q)
	   fp := queryFingerprint(q,
		   maxHops, limit)
	  off := 0
	  if q.PageToken != "" {
		  st, err := decodePage(q.PageToken)
		   if err != nil {
			  return LineageResult{}, err
		  }
		   if st.FP != fp || st.Limit != limit || st.MaxHops != maxHops {
			  return LineageResult{}, ErrPage
		  }
		  off = st.Off
	   }
	  anchors, gaps, err := s.anchors(ctx, q, q.Direction)
	   if err != nil {
		  return LineageResult{}, err
	   }
	  res := LineageResult{
		  Direction: q.Direction,
		   Gaps:      append([]GapMarker{}, gaps...),
		   Clamped:   clamped,
		   Paths: []LineagePath{},
		   Reached: []FundNode{},
		  Edges: []FundEdge{},
	  }
	  if clamped {
		  res.Gaps = append(res.Gaps, GapMarker{Code: GapClamped, Detail: "hop or page size clamped to hard cap"})
	   }
	  if len(anchors) == 0 {
		   res.Gaps = dedupeGaps(res.Gaps)
		  res.Complete = false
		  return res, nil
	  }
	  res.AnchorID = anchors[0].ID
	   stop := q.Direction == DirShortest
	  var merged walkOut
	   budget := s.cfg.HardLimit
	   for _, anchor := range anchors {
		   if err := ctx.Err(); err != nil {
			   return LineageResult{}, err
		   }
		   if budget <= 1 {
			  res.Truncated = true
			  res.Gaps = append(res.Gaps, GapMarker{Code: GapHopLimit, AtNodeID: anchor.ID, Detail: "anchor budget exhausted"})
			   break
		   }
		  w, err := s.walk(ctx, anchor, q, maxHops, budget, stop)
		   if err != nil {
			   return LineageResult{}, err
		  }
		   if stop && w.found == "" && !q.wants(anchor) {
			   w.gaps = append(w.gaps, GapMarker{
				   Code:     GapTargetUnreached,
				   AtNodeID: anchor.ID,
				   Detail: "target not reached within hop limit",
			  })
		   }
		  leaf := w.found
		   if leaf == "" {
			  leaf = chooseLeaf(w.order, w.hops, q.Direction, q)
		  }
		   path := buildPath(anchor.ID, leaf, w, q.Direction)
		   if w.truncated {
			   path.Complete = false
		   }
		  res.Paths = append(res.Paths, path)
		   before := len(merged.order)
		  mergeWalk(&merged, w)
		   budget -= len(merged.order) - before
		   if len(anchors) == 1 {
			  break
		  }
	  }
	  if merged.truncated {
		  res.Truncated = true
	  }
	   if merged.hopsUsed > res.HopsUsed {
		  res.HopsUsed = merged.hopsUsed
	  }
	   res.Gaps = append(res.Gaps, merged.gaps...)
	  res.Edges = append([]FundEdge{}, merged.edges...)
	   full := merged.order
	  if off > len(full) {
		  off = len(full)
	   }
	  end := off + limit
	  if end > len(full) {
		  end = len(full)
	  }
	   if off < end {
		   res.Reached = append([]FundNode{}, full[off:end]...)
	  } else {
		  res.Reached = []FundNode{}
	  }
	  if end < len(full) {
		   res.NextPageToken = encodePage(pageState{V: 1, Off: end, FP: fp, Limit: limit, MaxHops: maxHops})
	   }
	   res.Gaps = dedupeGaps(res.Gaps)
	  res.Complete = !res.Truncated && !hasBlocking(res.Gaps) && len(res.Paths) > 0 && pathsComplete(res.Paths)
	  return res, nil
  }

  func pathsComplete(paths []LineagePath) bool {
	  if len(paths) == 0 {
		  return false
	  }
	  for _, p := range paths {
		   if !p.Complete {
			   return false
		  }
	   }
	  return true
  }

   func (s *Service) noteQuery(d time.Duration) {
	  s.mu.Lock()
	  s.queries++
	   ms := d.Milliseconds()
	   if ms < 0 {
		   ms = 0
	  }
	   s.lastQuery = ms
	  s.mu.Unlock()
   }

// Inspect 巡检当前图并开单。快照被截断时拒绝开单。

  func (s *Service) Inspect(ctx context.Context) (InspectReport, error) {
	   if s == nil {
		  return InspectReport{}, ErrInvalid
	  }
	  snap, err := s.store.Snapshot(ctx)
	   if err != nil {
		   return InspectReport{}, err
	  }
	  if snap.Truncated {
		  return InspectReport{}, ErrSnapshot
	   }
	   now := s.clock()
	   found := Evaluate(snap, now, s.cfg)
	  report := InspectReport{At: now, Opened: []GraphIncident{}, Candidates: len(found)}
	   for _, inc := range found {
		   switch inc.Kind {
		  case IncidentBroken:
			  report.Broken++
		  case IncidentCycle:
			   report.Cycles++
		   case IncidentMultiParent:
			  report.MultiParent++
		   case IncidentLag:
			  report.Lag++
		   }
		   if inc.Reason == ReasonDanglingFrom || inc.Reason == ReasonDanglingTo {
			  report.Dangling++
		   }
		  inc.OpenedAt = now
		  inc.UpdatedAt = now
		  saved, created, err := s.store.OpenIncident(ctx, inc)
		   if err != nil {
			   return InspectReport{}, err
		   }
		  if created {
			  report.Opened = append(report.Opened, saved)
		  }
	  }
	   return report, nil
  }

// Node 读取单个节点。
  func (s *Service) Node(ctx context.Context, id string) (FundNode, error) {
	  n, ok, err := s.store.GetNode(ctx, id)
	   if err != nil {
		  return FundNode{}, err
	   }
	   if !ok {
		  return FundNode{}, ErrNotFound
	  }
	   return n, nil
   }

// Nodes 列出节点。

   func (s *Service) Nodes(ctx context.Context,
	   f NodeFilter) ([]FundNode, error) {
	  return s.store.ListNodes(ctx, f)
  }

// Edges 列出边。

   func (s *Service) Edges(ctx context.Context, f EdgeFilter) ([]FundEdge, error) {
	   return s.store.ListEdges(ctx, f)
   }

// Incidents 列出巡检单。
  func (s *Service) Incidents(ctx context.Context, f IncidentFilter) ([]GraphIncident, error) {
	   return s.store.ListIncidents(ctx,
		   f)
  }

// AckIncident 确认巡检单。
  func (s *Service) AckIncident(ctx context.Context, id string) error {
	   return s.store.SetIncidentStatus(ctx, id, StatusAck, s.clock())
  }

// CloseIncident 关闭巡检单。
   func (s *Service) CloseIncident(ctx context.Context, id string) error {
	  return s.store.SetIncidentStatus(ctx, id, StatusClosed, s.clock())
  }

// Audits 读取冲突审计。
   func (s *Service) Audits(ctx context.Context, edgeKey string, limit int) ([]AuditRecord, error) {
	  return s.store.ListAudit(ctx, edgeKey, limit)
   }

// Get 读取摄取游标。

  func (s *Service) GetCursor(ctx context.Context, source, stream string) (Cursor, bool, error) {
	  return s.store.GetCursor(ctx, source, stream)
  }

// Advance 推进游标。空 token 视为非法，避免把消费位置擦掉。

  func (s *Service) AdvanceCursor(ctx context.Context, source, stream, token string, eventAt time.Time) error {
	  if source == "" || token == "" {
		   return ErrInvalid
	  }
	   if stream == "" {
		   stream = "default"
	  }
	   prev, _, err := s.store.GetCursor(ctx, source, stream)
	   if err != nil {
		  return err
	   }
	  now := s.clock()
	  c := prev
	   c.Source = source
	  c.Stream = stream
	   c.Token = token
	  c.UpdatedAt = now
	  if !eventAt.IsZero() {
		   c.LastEventAt = eventAt.UTC()
	  }
	  if !c.LastEventAt.IsZero() {
		  lag := now.Sub(c.LastEventAt).Milliseconds()
		  if lag < 0 {
			   lag = 0
		  }
		   c.LagMs = lag
	   }
	   return s.store.PutCursor(ctx, c)
  }

// Put 写入调用方给出的游标，供滞后夹具使用。

  func (s *Service) PutCursor(ctx context.Context, c Cursor) error {
	   return s.store.PutCursor(ctx, c)
   }

// Metrics 汇总边基数、未关闭断链和最近查询耗时。
  func (s *Service) Metrics(ctx context.Context) (Metrics, error) {
	   nodes, edges, err := s.store.Counts(ctx)
	   if err != nil {
		   return Metrics{}, err
	  }
	   incs, err := s.store.ListIncidents(ctx, IncidentFilter{})
	  if err != nil {
		  return Metrics{}, err
	   }
	   curs,
		  err := s.store.ListCursors(ctx)
	  if err != nil {
		   return Metrics{}, err
	  }
	   m := Metrics{Nodes: nodes, Edges: edges}
	  var maxLag int64
	  for _, c := range curs {
		   if c.LagMs > maxLag {
			   maxLag = c.LagMs
		   }
	  }
	   now := s.clock()
	  for _, c := range curs {
		  if c.UpdatedAt.IsZero() {
			  continue
		  }
		   stall := now.Sub(c.UpdatedAt).Milliseconds()
		   if stall > maxLag {
			  maxLag = stall
		   }
	   }
	   m.IngestLagMs = maxLag
	   for _, inc := range incs {
		   if inc.Status != StatusOpen && inc.Status != StatusAck {
			   continue
		   }
		   m.OpenIncidents++
		   switch inc.Kind {
		   case IncidentBroken:
			   m.BrokenOpen++
		  case IncidentCycle:
			   m.CycleOpen++
		   case IncidentMultiParent:
			   m.MultiParentOpen++
		   case IncidentLag:
			  m.LagOpen++
		  }
	   }
	  s.mu.Lock()
	   m.Conflicts = s.conflicts
	  m.Ingests = s.ingests
	  m.Replays = s.replays
	   m.Queries = s.queries
	  m.LastQueryMs = s.lastQuery
	  s.mu.Unlock()
	   return m, nil
   }
