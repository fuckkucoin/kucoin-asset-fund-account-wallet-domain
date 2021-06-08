package fundgraph

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemStore 是进程内因果图，供查询服务、巡检和夹具使用。锁内复制返回值，调用方改 map 不会写回。

type MemStore struct {
	mu    sync.Mutex
	nodes map[string]FundNode
	edges map[string]FundEdge
	byKey map[string]string
	out   map[string][]string
	in    map[string][]string
	cursors   map[string]Cursor
	incidents map[string]GraphIncident
	audits []AuditRecord
	now    func() time.Time
}

// NewMemStore 构造空图。
  func NewMemStore() *MemStore {
	  return &MemStore{
		  nodes: map[string]FundNode{},
		   edges: map[string]FundEdge{},
		   byKey: map[string]string{},
		  out: map[string][]string{},
		   in:        map[string][]string{},
		  cursors:   map[string]Cursor{},
		   incidents: map[string]GraphIncident{},
		   now:       func() time.Time { return time.Now().UTC() },
	   }
   }

// SetClock 替换存储时钟，便于滞后夹具。
   func (m *MemStore) SetClock(fn func() time.Time) {
	   if m == nil || fn == nil {
		  return
	   }
	   m.mu.Lock()
	   m.now = fn
	  m.mu.Unlock()
  }

  func (m *MemStore) clock() time.Time {
	   if m.now == nil {
		  return time.Now().UTC()
	   }
	   return m.now().UTC()
  }

  func cursorMapKey(source, stream string) string {
	   return source + "\x1f" + stream
   }

// Ingest 原子写入一批节点和边。任一冲突则整批不落图，只追加冲突审计。
   func (m *MemStore) Ingest(ctx context.Context, b IngestBatch) (IngestResult, error) {
	  if err := ctx.Err(); err != nil {
		   return IngestResult{}, err
	   }
	   now := m.clock()
	   prepared, err := prepareBatch(b, now)
	  if err != nil {
		   return IngestResult{}, err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var conflicts []Conflict
	  seenNode := map[string]FundNode{}
	  for _, n := range prepared.Nodes {
		   if prev, ok := seenNode[n.ID]; ok {
			   conflicts = append(conflicts, diffNode(prev, n)...)
			   continue
		   }
		   if old, ok := m.nodes[n.ID]; ok {
			   conflicts = append(conflicts, diffNode(old, n)...)
		  }
		   seenNode[n.ID] = n
	   }
	   seenEdge := map[string]FundEdge{}
	  seenEdgeID := map[string]FundEdge{}
	  for _, e := range prepared.Edges {
		   key := e.Key().String()
		   if prev, ok := seenEdge[key]; ok {
			   conflicts = append(conflicts, diffEdge(prev, e)...)
			   continue
		  }
		   if prev, ok := seenEdgeID[e.ID]; ok && prev.Key().String() != key {
			  _,
				   cs := edgeReplay(prev, e, true)
			  conflicts = append(conflicts, cs...)
			   continue
		  }
		   if id, ok := m.byKey[key]; ok {
			   conflicts = append(conflicts, diffEdge(m.edges[id], e)...)
		   } else if old, ok := m.edges[e.ID]; ok {
			  _,
				   cs := edgeReplay(old, e, true)
			   conflicts = append(conflicts, cs...)
		   }
		  seenEdge[key] = e
		   seenEdgeID[e.ID] = e
	   }
	  if len(conflicts) > 0 {
		   rec := AuditRecord{
			   ID: AuditID(ActionConflict, conflicts[0].EdgeKey, conflicts[0].NodeID, now),
			   Action:  ActionConflict,
			  EdgeKey: conflicts[0].EdgeKey,
			  NodeID:  conflicts[0].NodeID,
			  Detail:  conflictsJSON(conflicts),
			   At: now,
		  }
		   m.audits = append(m.audits, rec)
		   return IngestResult{Conflicts: conflicts, Audited: true}, ErrConflict
	   }
	  res := IngestResult{}
	  for _, n := range prepared.Nodes {
		  if _, ok := m.nodes[n.ID]; ok {
			   res.ReplayNodes++
			   continue
		   }
		   n.OccurredAt = stampOccurred(n.OccurredAt, now)
		  m.nodes[n.ID] = cloneNode(n)
		   res.InsertedNodes++
	   }
	   for _, e := range prepared.Edges {
		  key := e.Key().String()
		  if _, ok := m.byKey[key]; ok {
			  res.ReplayEdges++
			  continue
		   }
		   e.OccurredAt = stampOccurred(e.OccurredAt, now)
		   m.edges[e.ID] = cloneEdge(e)
		   m.byKey[key] = e.ID
		  m.out[e.FromID] = append(m.out[e.FromID], e.ID)
		   m.in[e.ToID] = append(m.in[e.ToID], e.ID)
		  res.InsertedEdges++
	   }
	   if prepared.Source != "" {
		  ck := cursorMapKey(prepared.Source, prepared.Stream)
		   prev := m.cursors[ck]
		   cur := cursorFromBatch(prepared, prev, now)
		  m.cursors[ck] = cur
		   res.Cursor = cur
	   }
	  return res, nil
   }

// GetNode 按主键读取。不存在时 ok 为 false，不返回错误。

   func (m *MemStore) GetNode(ctx context.Context, id string) (FundNode, bool, error) {
	   if err := ctx.Err(); err != nil {
		  return FundNode{}, false, err
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   n, ok := m.nodes[id]
	  if !ok {
		   return FundNode{}, false, nil
	  }
	   return cloneNode(n), true, nil
  }

// ListNodes 按过滤条件扫描。结果按发生时间、主键稳定排序。
   func (m *MemStore) ListNodes(ctx context.Context, f NodeFilter) ([]FundNode, error) {
	   if err := ctx.Err(); err != nil {
		  return nil, err
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]FundNode, 0)
	  for _, n := range m.nodes {
		   if f.UserID != "" && n.UserID != f.UserID {
			  continue
		  }
		   if f.Asset != "" && n.Asset != NormAsset(f.Asset) {
			  continue
		   }
		   if f.Ref != "" && n.Ref != f.Ref && n.Corr() != f.Ref {
			   continue
		  }
		   if f.Kind != "" && n.Kind != f.Kind {
			   continue
		  }
		   if !f.From.IsZero() && n.OccurredAt.Before(f.From) {
			   continue
		  }
		   if !f.To.IsZero() && n.OccurredAt.After(f.To) {
			  continue
		   }
		   out = append(out, cloneNode(n))
	  }
	  sort.Slice(out, func(i, j int) bool {
		  if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			   return out[i].OccurredAt.Before(out[j].OccurredAt)
		  }
		  return out[i].ID < out[j].ID
	  })
	   if f.Limit > 0 && len(out) > f.Limit {
		   out = out[:f.Limit]
	   }
	   return out, nil
   }

// GetEdge 按边主键读取。
  func (m *MemStore) GetEdge(ctx context.Context, id string) (FundEdge, bool, error) {
	   if err := ctx.Err(); err != nil {
		   return FundEdge{}, false, err
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   e, ok := m.edges[id]
	  if !ok {
		   return FundEdge{}, false, nil
	   }
	  return cloneEdge(e), true, nil
   }

  func (m *MemStore) edgesByIDs(ids []string) []FundEdge {
	   out := make([]FundEdge,
		   0, len(ids))
	  for _, id := range ids {
		   e, ok := m.edges[id]
		   if !ok {
			   continue
		  }
		  out = append(out, cloneEdge(e))
	   }
	  sort.Slice(out, func(i, j int) bool {
		   if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			  return out[i].OccurredAt.Before(out[j].OccurredAt)
		   }
		  return out[i].ID < out[j].ID
	  })
	   return out
   }

// EdgesFrom 返回从该节点出发的边。

   func (m *MemStore) EdgesFrom(ctx context.Context, nodeID string) ([]FundEdge, error) {
	  if err := ctx.Err(); err != nil {
		   return nil, err
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   return m.edgesByIDs(m.out[nodeID]), nil
  }

// EdgesTo 返回指向该节点的边。

   func (m *MemStore) EdgesTo(ctx context.Context, nodeID string) ([]FundEdge, error) {
	   if err := ctx.Err(); err != nil {
		  return nil, err
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   return m.edgesByIDs(m.in[nodeID]), nil
   }

// ListEdges 按过滤条件扫描边。
  func (m *MemStore) ListEdges(ctx context.Context, f EdgeFilter) ([]FundEdge, error) {
	  if err := ctx.Err(); err != nil {
		   return nil, err
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]FundEdge, 0)
	   for _, e := range m.edges {
		   if f.Ref != "" && e.Ref != f.Ref {
			  continue
		   }
		  if f.FromID != "" && e.FromID != f.FromID {
			   continue
		  }
		   if f.ToID != "" && e.ToID != f.ToID {
			   continue
		   }
		  if f.EdgeType != "" && e.EdgeType != f.EdgeType {
			  continue
		  }
		   if f.UserID != "" && e.UserID != f.UserID {
			   continue
		   }
		   if f.Asset != "" && e.Asset != NormAsset(f.Asset) {
			  continue
		  }
		   out = append(out, cloneEdge(e))
	  }
	   sort.Slice(out, func(i, j int) bool {
		   if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			   return out[i].OccurredAt.Before(out[j].OccurredAt)
		   }
		   return out[i].ID < out[j].ID
	   })
	   if f.Limit > 0 && len(out) > f.Limit {
		  out = out[:f.Limit]
	   }
	   return out, nil
  }

// Counts 返回节点数和边数。
  func (m *MemStore) Counts(ctx context.Context) (int, int, error) {
	   if err := ctx.Err(); err != nil {
		  return 0, 0, err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   return len(m.nodes),
		   len(m.edges), nil
   }

// Snapshot 复制全图供巡检。超过 SnapshotCap 时标记截断。

  func (m *MemStore) Snapshot(ctx context.Context) (Snapshot, error) {
	   if err := ctx.Err(); err != nil {
		  return Snapshot{}, err
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  snap := Snapshot{}
	  for _, n := range m.nodes {
		  if len(snap.Nodes) >= SnapshotCap {
			   snap.Truncated = true
			  break
		  }
		   snap.Nodes = append(snap.Nodes, cloneNode(n))
	   }
	   for _, e := range m.edges {
		   if len(snap.Edges) >= SnapshotCap {
			   snap.Truncated = true
			   break
		  }
		   snap.Edges = append(snap.Edges, cloneEdge(e))
	   }
	   for _, c := range m.cursors {
		   snap.Cursors = append(snap.Cursors, c)
	   }
	  sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].ID < snap.Nodes[j].ID })
	  sort.Slice(snap.Edges, func(i, j int) bool { return snap.Edges[i].ID < snap.Edges[j].ID })
	   sort.Slice(snap.Cursors, func(i, j int) bool {
		  if snap.Cursors[i].Source != snap.Cursors[j].Source {
			   return snap.Cursors[i].Source < snap.Cursors[j].Source
		   }
		   return snap.Cursors[i].Stream < snap.Cursors[j].Stream
	  })
	  return snap, nil
  }

// Put 覆盖写入游标。
  func (m *MemStore) PutCursor(ctx context.Context, c Cursor) error {
	  if err := ctx.Err(); err != nil {
		   return err
	  }
	  if c.Source == "" {
		   return ErrInvalid
	   }
	  if c.Stream == "" {
		   c.Stream = "default"
	  }
	   if c.UpdatedAt.IsZero() {
		   c.UpdatedAt = m.clock()
	   }
	  m.mu.Lock()
	   m.cursors[cursorMapKey(c.Source, c.Stream)] = c
	   m.mu.Unlock()
	  return nil
  }

// Get 读取游标。
   func (m *MemStore) GetCursor(ctx context.Context, source, stream string) (Cursor, bool, error) {
	  if err := ctx.Err(); err != nil {
		  return Cursor{}, false, err
	   }
	   if stream == "" {
		  stream = "default"
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  c, ok := m.cursors[cursorMapKey(source, stream)]
	  return c, ok, nil
  }

// Lists 返回全部游标。
   func (m *MemStore) ListCursors(ctx context.Context) ([]Cursor, error) {
	  if err := ctx.Err(); err != nil {
		  return nil, err
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Cursor,
		   0, len(m.cursors))
	  for _, c := range m.cursors {
		   out = append(out, c)
	   }
	  sort.Slice(out, func(i, j int) bool {
		   if out[i].Source != out[j].Source {
			   return out[i].Source < out[j].Source
		  }
		  return out[i].Stream < out[j].Stream
	  })
	  return out, nil
  }

// OpenIncident 按指纹开单。已开放或已确认的单不重复开；已关闭的问题会重新打开。

   func (m *MemStore) OpenIncident(ctx context.Context, inc GraphIncident) (GraphIncident, bool, error) {
	  if err := ctx.Err(); err != nil {
		  return GraphIncident{}, false, err
	  }
	  if inc.Kind == "" || inc.Reason == "" {
		   return GraphIncident{}, false, ErrInvalid
	  }
	   if inc.ID == "" {
		   inc.ID = IncidentID(inc.Kind, inc.Reason, inc.Ref, inc.NodeID, inc.EdgeKey)
	  }
	  now := m.clock()
	   if inc.OpenedAt.IsZero() {
		  inc.OpenedAt = now
	  }
	   if inc.UpdatedAt.IsZero() {
		   inc.UpdatedAt = inc.OpenedAt
	   }
	  inc.Status = StatusOpen
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if old, ok := m.incidents[inc.ID]; ok {
		   if old.Status == StatusOpen || old.Status == StatusAck {
			  return old, false, nil
		   }
		   old.Status = StatusOpen
		   old.Detail = inc.Detail
		   old.OpenedAt = inc.OpenedAt
		  old.UpdatedAt = inc.UpdatedAt
		  m.incidents[inc.ID] = old
		  return old, true, nil
	   }
	   m.incidents[inc.ID] = inc
	  return inc, true, nil
  }

// ListIncidents 列出巡检单。
  func (m *MemStore) ListIncidents(ctx context.Context, f IncidentFilter) ([]GraphIncident, error) {
	  if err := ctx.Err(); err != nil {
		  return nil, err
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]GraphIncident, 0, len(m.incidents))
	  for _, inc := range m.incidents {
		  if f.Status != "" && inc.Status != f.Status {
			  continue
		   }
		  if f.Kind != "" && inc.Kind != f.Kind {
			  continue
		   }
		   if f.Ref != "" && inc.Ref != f.Ref {
			   continue
		  }
		  out = append(out, inc)
	  }
	  sort.Slice(out, func(i, j int) bool {
		  if !out[i].OpenedAt.Equal(out[j].OpenedAt) {
			   return out[i].OpenedAt.Before(out[j].OpenedAt)
		   }
		  return out[i].ID < out[j].ID
	  })
	  if f.Limit > 0 && len(out) > f.Limit {
		  out = out[:f.Limit]
	  }
	   return out, nil
   }

// SetIncidentStatus 迁移工单状态。

   func (m *MemStore) SetIncidentStatus(ctx context.Context, id, status string, at time.Time) error {
	  if err := ctx.Err(); err != nil {
		   return err
	  }
	  if status != StatusAck && status != StatusClosed && status != StatusOpen {
		   return ErrStatus
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   inc, ok := m.incidents[id]
	   if !ok {
		   return ErrNotFound
	  }
	   if !statusAllowed(inc.Status, status) {
		  return ErrStatus
	   }
	   if at.IsZero() {
		   at = m.clock()
	   }
	  inc.Status = status
	   inc.UpdatedAt = at.UTC()
	  m.incidents[id] = inc
	   return nil
   }

   func statusAllowed(from, to string) bool {
	   if from == to {
		   return true
	  }
	   switch from {
	  case StatusOpen:
		   return to == StatusAck || to == StatusClosed
	  case StatusAck:
		   return to == StatusClosed
	   default:
		  return false
	  }
  }

// AppendAudit 追加一条本包审计。
   func (m *MemStore) AppendAudit(ctx context.Context, a AuditRecord) error {
	   if err := ctx.Err(); err != nil {
		  return err
	   }
	   if a.ID == "" {
		   a.ID = AuditID(a.Action, a.EdgeKey, a.NodeID, a.At)
	  }
	  if a.At.IsZero() {
		  a.At = m.clock()
	   }
	   m.mu.Lock()
	  m.audits = append(m.audits, a)
	   m.mu.Unlock()
	   return nil
  }

// ListAudit 按边键过滤审计。edgeKey 为空时返回最近的记录。
  func (m *MemStore) ListAudit(ctx context.Context, edgeKey string, limit int) ([]AuditRecord, error) {
	   if err := ctx.Err(); err != nil {
		  return nil, err
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]AuditRecord, 0)
	   for _, a := range m.audits {
		  if edgeKey != "" && a.EdgeKey != edgeKey {
			  continue
		  }
		   out = append(out, a)
	   }
	   if limit > 0 && len(out) > limit {
		   out = out[len(out)-limit:]
	   }
	  return out, nil
  }

// SetEdgeTier 只改运维标记，不改变幂等比较用的业务属性。
   func (m *MemStore) SetEdgeTier(ctx context.Context, id, tier string, at time.Time) error {
	   if err := ctx.Err(); err != nil {
		   return err
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  e, ok := m.edges[id]
	  if !ok {
		   return ErrNotFound
	   }
	  if e.Attrs == nil {
		  e.Attrs = map[string]string{}
	  }
	  e.Attrs["tier"] = tier
	  if at.IsZero() {
		   at = m.clock()
	   }
	  e.Attrs["tier_at"] = at.UTC().Format(time.RFC3339Nano)
	   m.edges[id] = e
	   return nil
  }

// DeleteEdges 按主键删除边并维护出入索引。调用方必须先通过确认口令。
  func (m *MemStore) DeleteEdges(ctx context.Context, ids []string) error {
	   if err := ctx.Err(); err != nil {
		  return err
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, id := range ids {
		  e, ok := m.edges[id]
		  if !ok {
			  continue
		  }
		  delete(m.edges, id)
		  delete(m.byKey, e.Key().String())
		  m.out[e.FromID] = removeID(m.out[e.FromID], id)
		   m.in[e.ToID] = removeID(m.in[e.ToID], id)
	  }
	  return nil
   }

   func removeID(sl []string, id string) []string {
	  out := make([]string,
		   0, len(sl))
	  for _, x := range sl {
		  if x != id {
			  out = append(out, x)
		  }
	   }
	  return out
   }
