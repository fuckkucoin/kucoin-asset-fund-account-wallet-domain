package fundgraph

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type walkOut struct {
	order []FundNode
	nodes map[string]FundNode
	edges      []FundEdge
	edgeByID   map[string]FundEdge
	parentNode map[string]string
	parentEdge map[string]string
	hops map[string]int
	gaps []GapMarker
	truncated bool
	hopsUsed  int
	found     string
}

type pageState struct {
	V   int `json:"v"`
	Off int `json:"off"`
	FP    string `json:"fp"`
	Limit int    `json:"limit"`
	MaxHops int `json:"max_hops"`
}

  func (q LineageQuery) normDir() string {
	  switch strings.ToLower(strings.TrimSpace(q.Direction)) {
	  case DirForward, "impact", "down":
		  return DirForward
	   case DirShortest, "short", "responsibility":
		  return DirShortest
	  default:
		   return DirReverse
	  }
   }

   func normalizeQuery(q LineageQuery) LineageQuery {
	  q.StartNodeID = strings.TrimSpace(q.StartNodeID)
	   q.StartRef = strings.TrimSpace(q.StartRef)
	   q.StartKind = NodeKind(strings.TrimSpace(string(q.StartKind)))
	   q.UserID = strings.TrimSpace(q.UserID)
	  q.Asset = NormAsset(q.Asset)
	   q.TargetNodeID = strings.TrimSpace(q.TargetNodeID)
	   q.TargetKind = NodeKind(strings.TrimSpace(string(q.TargetKind)))
	   q.TargetRef = strings.TrimSpace(q.TargetRef)
	  q.Direction = q.normDir()
	  q.PageToken = strings.TrimSpace(q.PageToken)
	   if !q.From.IsZero() {
		  q.From = q.From.UTC()
	  }
	   if !q.To.IsZero() {
		  q.To = q.To.UTC()
	  }
	   types := make([]EdgeType,
		   0, len(q.EdgeTypes))
	  for _, t := range q.EdgeTypes {
		  t = EdgeType(strings.TrimSpace(string(t)))
		   if t == "" {
			  continue
		   }
		  types = append(types, t)
	  }
	  sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	   q.EdgeTypes = types
	   return q
  }

   func queryFingerprint(q LineageQuery, maxHops, limit int) string {
	  payload := struct {
		  StartNodeID string `json:"start_node_id"`
		   StartRef    string `json:"start_ref"`
		   StartKind NodeKind `json:"start_kind"`
		  UserID string `json:"user_id"`
		   Asset string `json:"asset"`
		   From         time.Time `json:"from"`
		  To           time.Time `json:"to"`
		  Direction    string    `json:"direction"`
		   TargetNodeID string    `json:"target_node_id"`
		  TargetKind   NodeKind  `json:"target_kind"`
		  TargetRef    string    `json:"target_ref"`
		  EdgeTypes []EdgeType `json:"edge_types"`
		   MaxHops int `json:"max_hops"`
		  Limit int `json:"limit"`
	   }{
		   q.StartNodeID, q.StartRef, q.StartKind, q.UserID, q.Asset, q.From, q.To,
		  q.Direction, q.TargetNodeID, q.TargetKind, q.TargetRef, q.EdgeTypes, maxHops, limit,
	  }
	  b, _ := json.Marshal(payload)
	   return string(b)
  }

   func encodePage(st pageState) string {
	  b, _ := json.Marshal(st)
	  return base64.RawURLEncoding.EncodeToString(b)
  }

  func decodePage(tok string) (pageState, error) {
	  raw, err := base64.RawURLEncoding.DecodeString(tok)
	  if err != nil {
		  return pageState{}, ErrPage
	   }
	   var st pageState
	   if err := json.Unmarshal(raw, &st); err != nil {
		  return pageState{}, ErrPage
	  }
	  if st.V != 1 || st.Off < 0 {
		  return pageState{}, ErrPage
	  }
	   return st, nil
  }

  func (s *Service) bounds(q LineageQuery) (maxHops, limit int, clamped bool) {
	   maxHops = q.MaxHops
	  if maxHops <= 0 {
		   maxHops = s.cfg.DefaultMaxHops
	  }
	   if maxHops > s.cfg.HardMaxHops {
		   maxHops = s.cfg.HardMaxHops
		   clamped = true
	  }
	   limit = q.Limit
	   if limit <= 0 {
		  limit = s.cfg.DefaultLimit
	   }
	   if limit > s.cfg.HardLimit {
		   limit = s.cfg.HardLimit
		  clamped = true
	  }
	   return maxHops, limit, clamped
   }

   func (s *Service) anchors(ctx context.Context, q LineageQuery, dir string) ([]FundNode, []GapMarker, error) {
	  if q.StartNodeID != "" {
		   n, ok, err := s.store.GetNode(ctx, q.StartNodeID)
		   if err != nil {
			   return nil, nil, err
		  }
		  if !ok {
			   return nil, []GapMarker{{
				   Code: GapStartMissing,
				  AtNodeID: q.StartNodeID,
				   Detail:   "anchor node missing",
			   }}, nil
		   }
		  return []FundNode{n}, nil, nil
	   }
	  if q.StartRef == "" && q.UserID == "" {
		  return nil, []GapMarker{{
			  Code:   GapNoAnchor,
			   Detail: "start_node_id, start_ref or user_id required",
		   }}, nil
	  }
	  limit := s.cfg.HardLimit
	   nodes, err := s.store.ListNodes(ctx, NodeFilter{
		  UserID: q.UserID,
		  Asset: q.Asset,
		   Ref:   q.StartRef,
		  Kind: q.StartKind,
		  From: q.From,
		  To:   q.To,
		  Limit: limit,
	  })
	  if err != nil {
		  return nil, nil, err
	   }
	  if len(nodes) == 0 {
		   return nil, []GapMarker{{
			  Code: GapNoAnchor,
			   Detail: "no node matched",
		  }}, nil
	  }
	  if q.StartRef != "" && q.StartKind == "" && q.UserID == "" {
		   latest := dir != DirForward
		   return []FundNode{pickStage(nodes, latest)}, nil, nil
	  }
	   if len(nodes) > limit {
		   nodes = nodes[:limit]
	   }
	   return nodes, nil, nil
  }

   func pickStage(nodes []FundNode, latest bool) FundNode {
	   best := nodes[0]
	  bestRank,
		   bestKnown := spineIndex(best.Kind)
	  for _, n := range nodes[1:] {
		   rank, known := spineIndex(n.Kind)
		   if !known && bestKnown {
			  continue
		   }
		  if known && !bestKnown {
			   best, bestRank, bestKnown = n, rank, true
			   continue
		   }
		  if !known && !bestKnown {
			  if laterNode(n, best) {
				  best = n
			   }
			   continue
		  }
		  better := false
		   if latest {
			   better = rank > bestRank || (rank == bestRank && laterNode(n, best))
		  } else {
			  better = rank < bestRank || (rank == bestRank && earlierNode(n, best))
		  }
		  if better {
			   best, bestRank = n, rank
		  }
	   }
	  return best
   }

  func laterNode(a, b FundNode) bool {
	   if !a.OccurredAt.Equal(b.OccurredAt) {
		  return a.OccurredAt.After(b.OccurredAt)
	  }
	  return a.ID < b.ID
  }

  func earlierNode(a, b FundNode) bool {
	   if !a.OccurredAt.Equal(b.OccurredAt) {
		   return a.OccurredAt.Before(b.OccurredAt)
	  }
	  return a.ID < b.ID
  }

   func filterEdgeTypes(edges []FundEdge, allow []EdgeType) []FundEdge {
	  if len(allow) == 0 {
		  return append([]FundEdge(nil),
			   edges...)
	   }
	  set := make(map[EdgeType]bool, len(allow))
	  for _, t := range allow {
		   set[t] = true
	  }
	  out := make([]FundEdge, 0, len(edges))
	   for _, e := range edges {
		   if set[e.EdgeType] {
			   out = append(out, e)
		   }
	  }
	  return out
   }

  func sortWalkEdges(edges []FundEdge, prefer EdgeType) {
	   sort.SliceStable(edges, func(i, j int) bool {
		   pi, pj := 1, 1
		  if prefer != "" && edges[i].EdgeType == prefer {
			   pi = 0
		   }
		   if prefer != "" && edges[j].EdgeType == prefer {
			   pj = 0
		  }
		   if pi != pj {
			   return pi < pj
		   }
		   if !edges[i].OccurredAt.Equal(edges[j].OccurredAt) {
			  return edges[i].OccurredAt.Before(edges[j].OccurredAt)
		  }
		   return edges[i].ID < edges[j].ID
	  })
  }

  func hasAnyType(edges []FundEdge, types []EdgeType) bool {
	   if len(types) == 0 {
		   return true
	  }
	  set := make(map[EdgeType]bool, len(types))
	  for _, t := range types {
		  set[t] = true
	  }
	   for _, e := range edges {
		   if set[e.EdgeType] {
			  return true
		  }
	  }
	  return false
   }

   func isAncestor(next, cur string, parent map[string]string) bool {
	   p := cur
	   for seen := 0; p != "" && seen < 256; seen++ {
		   if p == next {
			   return true
		   }
		  nxt, ok := parent[p]
		   if !ok {
			  return false
		   }
		  p = nxt
	   }
	   return false
   }

   func joinTypes(types []EdgeType) string {
	   parts := make([]string, len(types))
	   for i, t := range types {
		   parts[i] = string(t)
	   }
	   return strings.Join(parts, ",")
  }

   func (s *Service) walk(ctx context.Context, start FundNode, q LineageQuery, maxHops, nodeCap int, stopAtTarget bool) (walkOut, error) {
	  out := walkOut{
		   order: []FundNode{start},
		   nodes:      map[string]FundNode{start.ID: start},
		  edgeByID:   map[string]FundEdge{},
		  parentNode: map[string]string{},
		   parentEdge: map[string]string{},
		   hops:       map[string]int{start.ID: 0},
	  }
	  if stopAtTarget && q.wants(start) {
		   out.found = start.ID
		  return out, nil
	   }
	  type item struct {
		  id   string
		  hops int
	  }
	  queue := []item{{id: start.ID, hops: 0}}
	  seen := map[string]bool{start.ID: true}
	   forward := q.Direction == DirForward
	   for len(queue) > 0 {
		  if err := ctx.Err(); err != nil {
			  return out, err
		   }
		  cur := queue[0]
		   queue = queue[1:]
		  curNode := out.nodes[cur.id]
		  var (
			  edges []FundEdge
			   err   error
		   )
		  if forward {
			  edges, err = s.store.EdgesFrom(ctx, cur.id)
		  } else {
			  edges, err = s.store.EdgesTo(ctx, cur.id)
		   }
		   if err != nil {
			  return out, err
		  }
		  edges = filterEdgeTypes(edges, q.EdgeTypes)
		  prefer := PreferredOutgoing(curNode.Kind)
		  if !forward {
			   prefer = PreferredIncoming(curNode.Kind)
		  }
		  sortWalkEdges(edges, prefer)
		  if !forward {
			   if need := AcceptedIncoming(curNode.Kind); len(need) > 0 && !hasAnyType(edges, need) {
				   out.gaps = append(out.gaps, GapMarker{
					   Code:     GapMissingEdge,
					   AtNodeID: cur.id,
					   EdgeType: joinTypes(need),
					   Detail:   "required incoming edge missing",
				   })
			  }
		   }
		   if cur.hops >= maxHops {
			   if len(edges) > 0 {
				   out.truncated = true
				   out.gaps = append(out.gaps, GapMarker{
					  Code:     GapHopLimit,
					  AtNodeID: cur.id,
					  Detail:   "hop limit reached",
				  })
			  }
			  continue
		   }
		   for _, e := range edges {
			  nextID := e.ToID
			  if !forward {
				   nextID = e.FromID
			  }
			  if nextID == "" {
				   out.gaps = append(out.gaps, GapMarker{
					   Code: GapDangling,
					   AtNodeID: cur.id,
					  EdgeID: e.ID,
					  Detail: "empty endpoint",
				  })
				  continue
			   }
			   if isAncestor(nextID, cur.id, out.parentNode) {
				  out.gaps = append(out.gaps, GapMarker{
					   Code:     GapCycleCut,
					   AtNodeID: cur.id,
					  EdgeID:   e.ID,
					   EdgeType: string(e.EdgeType),
					  Detail: "cycle cut before repeat",
				   })
				   continue
			  }
			  if seen[nextID] {
				   continue
			  }
			   n,
				  ok, err := s.store.GetNode(ctx, nextID)
			  if err != nil {
				   return out, err
			  }
			   if !ok {
				   out.gaps = append(out.gaps, GapMarker{
					  Code: GapDangling,
					   AtNodeID: cur.id,
					  EdgeID: e.ID,
					   Detail: "endpoint node missing",
				  })
				   continue
			   }
			  seen[nextID] = true
			  out.parentNode[nextID] = cur.id
			  out.parentEdge[nextID] = e.ID
			  out.edgeByID[e.ID] = e
			  hop := cur.hops + 1
			  out.hops[nextID] = hop
			   if hop > out.hopsUsed {
				   out.hopsUsed = hop
			   }
			  out.nodes[nextID] = n
			   out.order = append(out.order,
				  n)
			   out.edges = append(out.edges, e)
			   if stopAtTarget && q.wants(n) {
				  out.found = n.ID
				   return out, nil
			   }
			   if len(out.order) >= nodeCap {
				  out.truncated = true
				  out.gaps = append(out.gaps, GapMarker{
					  Code: GapHopLimit,
					  AtNodeID: nextID,
					   Detail:   "node cap reached",
				  })
				   return out, nil
			  }
			   queue = append(queue, item{id: nextID, hops: hop})
		  }
	   }
	  return out, nil
  }

  func (q LineageQuery) wants(n FundNode) bool {
	  specified := q.TargetNodeID != "" || q.TargetKind != "" || q.TargetRef != ""
	   if q.TargetNodeID != "" && n.ID != q.TargetNodeID {
		   return false
	   }
	   if q.TargetKind != "" && n.Kind != q.TargetKind {
		   return false
	   }
	   if q.TargetRef != "" && n.Ref != q.TargetRef && n.Corr() != q.TargetRef {
		   return false
	   }
	  if specified {
		   return true
	  }
	  return n.Kind == NodeDepositObserve
   }

   func chooseLeaf(order []FundNode, hops map[string]int, dir string, q LineageQuery) string {
	  if len(order) == 0 {
		  return ""
	  }
	  if q.TargetNodeID != "" {
		   for _, n := range order {
			  if n.ID == q.TargetNodeID {
				   return n.ID
			  }
		  }
	   }
	  if q.TargetKind != "" {
		  for _, n := range order {
			   if n.Kind == q.TargetKind {
				   return n.ID
			   }
		   }
	   }
	  if q.TargetRef != "" {
		  for _, n := range order {
			  if n.Ref == q.TargetRef || n.Corr() == q.TargetRef {
				   return n.ID
			   }
		   }
	  }
	   if dir == DirForward {
		  best := order[0]
		  bestRank := -1
		  if r, ok := spineIndex(best.Kind); ok {
			  bestRank = r
		   }
		  for _, n := range order[1:] {
			  r, ok := spineIndex(n.Kind)
			   rank := -1
			  if ok {
				   rank = r
			  }
			   if rank > bestRank || (rank == bestRank && hops[n.ID] > hops[best.ID]) {
				   best = n
				   bestRank = rank
			   }
		  }
		   return best.ID
	  }
	  best := order[0]
	   bestRank := 100
	  found := false
	   for _, n := range order {
		  r, ok := spineIndex(n.Kind)
		  if !ok {
			   continue
		  }
		  if !found || r < bestRank || (r == bestRank && hops[n.ID] > hops[best.ID]) {
			  best = n
			  bestRank = r
			  found = true
		   }
	  }
	  if found {
		   return best.ID
	   }
	   for _, n := range order[1:] {
		   if hops[n.ID] > hops[best.ID] {
			  best = n
		   }
	   }
	   return best.ID
  }

  func buildPath(anchor, leaf string, w walkOut, dir string) LineagePath {
	   path := LineagePath{Direction: dir}
	   if leaf == "" || w.nodes == nil {
		  path.Gaps = append(path.Gaps, GapMarker{Code: GapNoAnchor, Detail: "empty walk"})
		   return path
	  }
	   chain := make([]string, 0, 16)
	   cur := leaf
	  seen := map[string]bool{}
	  for guard := 0; cur != "" && guard < 256; guard++ {
		   if seen[cur] {
			  path.Gaps = append(path.Gaps, GapMarker{Code: GapCycleCut, AtNodeID: cur, Detail: "path reconstruction cut"})
			   break
		  }
		   seen[cur] = true
		  chain = append(chain, cur)
		   if cur == anchor {
			   break
		  }
		   cur = w.parentNode[cur]
	  }
	  for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		  chain[i], chain[j] = chain[j], chain[i]
	  }
	   onPath := map[string]bool{}
	  for _, id := range chain {
		  n, ok := w.nodes[id]
		   if !ok {
			  path.Gaps = append(path.Gaps, GapMarker{Code: GapDangling, AtNodeID: id, Detail: "path node missing"})
			   continue
		  }
		   path.Nodes = append(path.Nodes, n)
		   onPath[id] = true
	   }
	  for i := 1; i < len(chain); i++ {
		   eid := w.parentEdge[chain[i]]
		   if e, ok := w.edgeByID[eid]; ok {
			  path.Edges = append(path.Edges, e)
		  }
	  }
	  for _, g := range w.gaps {
		  if onPath[g.AtNodeID] {
			   path.Gaps = append(path.Gaps, g)
		   }
	  }
	  path.Hops = len(path.Edges)
	   path.Complete = !hasBlocking(path.Gaps)
	   return path
   }

   func hasBlocking(gaps []GapMarker) bool {
	  for _, g := range gaps {
		   if blockingGap(g.Code) {
			   return true
		  }
	   }
	   return false
  }

   func blockingGap(code string) bool {
	   switch code {
	  case GapMissingEdge, GapDangling, GapCycleCut, GapHopLimit, GapStartMissing, GapTargetUnreached, GapNoAnchor:
		  return true
	  default:
		  return false
	  }
  }

  func dedupeGaps(in []GapMarker) []GapMarker {
	  if len(in) == 0 {
		   return []GapMarker{}
	  }
	  seen := map[string]bool{}
	   out := make([]GapMarker, 0, len(in))
	   for _, g := range in {
		   k := g.Code + "|" + g.AtNodeID + "|" + g.EdgeID + "|" + g.EdgeType + "|" + g.Detail
		   if seen[k] {
			   continue
		   }
		   seen[k] = true
		  out = append(out, g)
	  }
	  return out
   }

   func mergeWalk(dst *walkOut, src walkOut) {
	   if dst.nodes == nil {
		   dst.nodes = map[string]FundNode{}
		   dst.edgeByID = map[string]FundEdge{}
		   dst.parentNode = map[string]string{}
		  dst.parentEdge = map[string]string{}
		   dst.hops = map[string]int{}
	  }
	   for _, n := range src.order {
		  if _, ok := dst.nodes[n.ID]; ok {
			  continue
		   }
		  dst.nodes[n.ID] = n
		   dst.order = append(dst.order, n)
		  if src.hops[n.ID] > dst.hopsUsed {
			   dst.hopsUsed = src.hops[n.ID]
		   }
		  dst.hops[n.ID] = src.hops[n.ID]
	  }
	   for _, e := range src.edges {
		   if _, ok := dst.edgeByID[e.ID]; ok {
			   continue
		   }
		  dst.edgeByID[e.ID] = e
		  dst.edges = append(dst.edges, e)
	   }
	   dst.gaps = append(dst.gaps, src.gaps...)
	   if src.truncated {
		  dst.truncated = true
	  }
	   if src.hopsUsed > dst.hopsUsed {
		  dst.hopsUsed = src.hopsUsed
	  }
  }
