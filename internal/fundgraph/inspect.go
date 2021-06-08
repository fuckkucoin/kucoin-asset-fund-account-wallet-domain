package fundgraph

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Evaluate 在快照上检查断链、环路、多父和摄取滞后。不写存储，不改节点。

  func Evaluate(snap Snapshot, now time.Time, cfg Config) []GraphIncident {
	  cfg = cfg.norm()
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   var out []GraphIncident
	   out = append(out, evalDangling(snap)...)
	   out = append(out, evalSpine(snap)...)
	  out = append(out, evalCycles(snap)...)
	  out = append(out, evalMultiParent(snap)...)
	  out = append(out,
		  evalLag(snap, now, cfg)...)
	   return dedupeIncidents(out)
  }

  func dedupeIncidents(in []GraphIncident) []GraphIncident {
	   seen := map[string]bool{}
	   out := make([]GraphIncident,
		  0, len(in))
	  for _, inc := range in {
		   id := IncidentID(inc.Kind, inc.Reason, inc.Ref, inc.NodeID, inc.EdgeKey)
		   if seen[id] {
			   continue
		   }
		  seen[id] = true
		  inc.ID = id
		  if inc.Status == "" {
			  inc.Status = StatusOpen
		  }
		  out = append(out, inc)
	   }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].ID < out[j].ID })
	   return out
  }

  func evalDangling(snap Snapshot) []GraphIncident {
	  have := make(map[string]bool, len(snap.Nodes))
	  for _, n := range snap.Nodes {
		  have[n.ID] = true
	   }
	  var out []GraphIncident
	   for _, e := range snap.Edges {
		   key := e.Key().String()
		   if !have[e.FromID] {
			  out = append(out, GraphIncident{
				   Kind:    IncidentBroken,
				  Reason:  ReasonDanglingFrom,
				  EdgeKey: key,
				   NodeID:  e.FromID,
				  Ref: e.Ref,
				  Detail: "edge from node missing",
			   })
		  }
		  if !have[e.ToID] {
			  out = append(out, GraphIncident{
				   Kind:   IncidentBroken,
				   Reason: ReasonDanglingTo,
				   EdgeKey: key,
				  NodeID: e.ToID,
				  Ref:    e.Ref,
				  Detail: "edge to node missing",
			   })
		  }
	   }
	   return out
  }

type spineGroup struct {
	nodes []FundNode
	edges []FundEdge
}

   func evalSpine(snap Snapshot) []GraphIncident {
	   groups := map[string]*spineGroup{}
	  ensure := func(ref string) *spineGroup {
		  g, ok := groups[ref]
		   if !ok {
			   g = &spineGroup{}
			   groups[ref] = g
		  }
		  return g
	  }
	   nodeCorr := map[string]string{}
	   for _, n := range snap.Nodes {
		  if _, ok := spineIndex(n.Kind); !ok {
			  continue
		  }
		  c := n.Corr()
		   if c == "" {
			  continue
		  }
		   nodeCorr[n.ID] = c
		   ensure(c).nodes = append(ensure(c).nodes, n)
	   }
	  for _, e := range snap.Edges {
		   if !SpineEdge(e.EdgeType) {
			   continue
		  }
		  refs := map[string]struct{}{}
		   if e.Ref != "" {
			   refs[e.Ref] = struct{}{}
		   }
		  if c, ok := nodeCorr[e.FromID]; ok {
			   refs[c] = struct{}{}
		  }
		  if c, ok := nodeCorr[e.ToID]; ok {
			   refs[c] = struct{}{}
		  }
		   for ref := range refs {
			   ensure(ref).edges = append(ensure(ref).edges, e)
		  }
	   }
	  refs := make([]string, 0, len(groups))
	   for ref := range groups {
		  refs = append(refs, ref)
	  }
	   sort.Strings(refs)
	  var out []GraphIncident
	  for _, ref := range refs {
		   out = append(out, evalSpineGroup(ref, groups[ref])...)
	  }
	  return out
  }
		  if idx > maxS {
			   maxS = idx
		  }
	  }
	   if maxS <= minS {
		   return nil
	  }
	  var out []GraphIncident
	  for s := minS + 1; s <= maxS; s++ {
		  if len(present[s]) == 0 {
			  rep := stableNode(nearestPresent(present,
				  s, maxS))
			  out = append(out, GraphIncident{
				  Kind:   IncidentBroken,
				  Reason: ReasonMissingStage,
				   NodeID: rep.ID,
				  Ref:    ref,
				  Detail: fmt.Sprintf("stage %s missing between %s and %s", spineStages[s], spineStages[minS], spineStages[maxS]),
			  })
			  continue
		   }
		   if len(present[s-1]) == 0 {
			  continue
		   }
		  link, ok := SpineLink(spineStages[s])
		  if !ok {
			   continue
		   }
		   if !stageLinked(present[s-1], present[s], link, g.edges) {
			  rep := stableNode(present[s])
			  out = append(out, GraphIncident{
				  Kind:   IncidentBroken,
				   Reason: ReasonMissingLink,
				   NodeID: rep.ID,
				  Ref: ref,
				  EdgeKey: string(link),
				   Detail: fmt.Sprintf("missing %s into %s", link, spineStages[s]),
			  })
		   }
	  }
	  return out
   }

   func nearestPresent(present map[int][]FundNode, from, maxS int) []FundNode {
	   for s := from; s <= maxS; s++ {
		   if len(present[s]) > 0 {
			   return present[s]
		  }
	  }
	  for s := from; s >= 0; s-- {
		  if len(present[s]) > 0 {
			   return present[s]
		  }
	   }
	   return nil
   }

  func stableNode(nodes []FundNode) FundNode {
	  if len(nodes) == 0 {
		  return FundNode{}
	   }
	   best := nodes[0]
	  for _, n := range nodes[1:] {
		  if n.ID < best.ID {
			   best = n
		   }
	   }
	   return best
   }

   func stageLinked(fromNodes, toNodes []FundNode, typ EdgeType, edges []FundEdge) bool {
	  fromSet := make(map[string]bool, len(fromNodes))
	   toSet := make(map[string]bool, len(toNodes))
	  for _, n := range fromNodes {
		   fromSet[n.ID] = true
	  }
	  for _, n := range toNodes {
		  toSet[n.ID] = true
	   }
	  for _, e := range edges {
		   if e.EdgeType == typ && fromSet[e.FromID] && toSet[e.ToID] {
			   return true
		  }
	  }
	   return false
   }

  func evalCycles(snap Snapshot) []GraphIncident {
	  have := make(map[string]bool, len(snap.Nodes))
	  ids := make([]string, 0, len(snap.Nodes))
	  for _, n := range snap.Nodes {
		  if have[n.ID] {
			  continue
		  }
		  have[n.ID] = true
		   ids = append(ids, n.ID)
	  }
	   sort.Strings(ids)
	   adj := map[string][]FundEdge{}
	   for _, e := range snap.Edges {
		  if !have[e.FromID] || !have[e.ToID] {
			   continue
		   }
		   adj[e.FromID] = append(adj[e.FromID], e)
	   }
	  for id := range adj {
		  sort.Slice(adj[id], func(i, j int) bool { return adj[id][i].ID < adj[id][j].ID })
	  }
	  const (
		  white = 0
		  gray  = 1
		   black = 2
	   )
	  color := map[string]int{}
	  index := map[string]int{}
	  var stack []string
	   var out []GraphIncident
	   var dfs func(string)
	   dfs = func(id string) {
		  color[id] = gray
		  index[id] = len(stack)
		  stack = append(stack, id)
		  for _, e := range adj[id] {
			   to := e.ToID
			   switch color[to] {
			   case gray:
				   cyc := append([]string{}, stack[index[to]:]...)
				   cyc = append(cyc, to)
				  if len(cyc) > 32 {
					   cyc = cyc[:32]
				  }
				   out = append(out, GraphIncident{
					  Kind:    IncidentCycle,
					  Reason:  ReasonCycle,
					  EdgeKey: e.Key().String(),
					  NodeID: to,
					  Ref:    e.Ref,
					  Detail: "cycle " + strings.Join(cyc, "->"),
				  })
			  case white:
				  dfs(to)
			  }
		   }
		  stack = stack[:len(stack)-1]
		  delete(index, id)
		   color[id] = black
	   }
	  for _, id := range ids {
		  if color[id] == white {
			  dfs(id)
		   }
		   if len(out) >= 32 {
			   break
		  }
	   }
	  if len(out) > 32 {
		   out = out[:32]
	  }
	   return out
   }

  func evalMultiParent(snap Snapshot) []GraphIncident {
	  type acc struct {
		   froms map[string]FundEdge
		  ref   string
		   to string
		  typ EdgeType
	   }
	   groups := map[string]*acc{}
	   for _, e := range snap.Edges {
		  if !SingleParent(e.EdgeType) {
			   continue
		   }
		  k := e.ToID + "|" + string(e.EdgeType)
		   g, ok := groups[k]
		  if !ok {
			  g = &acc{froms: map[string]FundEdge{}, ref: e.Ref, to: e.ToID, typ: e.EdgeType}
			  groups[k] = g
		  }
		   g.froms[e.FromID] = e
		   if g.ref == "" {
			   g.ref = e.Ref
		   }
	   }
	  keys := make([]string, 0, len(groups))
	  for k := range groups {
		   keys = append(keys, k)
	  }
	  sort.Strings(keys)
	  var out []GraphIncident
	   for _, k := range keys {
		   g := groups[k]
		   if len(g.froms) <= 1 {
			  continue
		   }
		   fromIDs := make([]string,
			  0, len(g.froms))
		  var sample FundEdge
		  for id, e := range g.froms {
			  fromIDs = append(fromIDs, id)
			   sample = e
		   }
		  sort.Strings(fromIDs)
		   out = append(out, GraphIncident{
			  Kind: IncidentMultiParent,
			  Reason:  ReasonMultiParent,
			  EdgeKey: sample.Key().String(),
			  NodeID:  g.to,
			  Ref:     g.ref,
			  Detail: fmt.Sprintf("%s has parents %s", g.typ, strings.Join(fromIDs, ",")),
		  })
	   }
	   return out
  }

   func evalLag(snap Snapshot, now time.Time, cfg Config) []GraphIncident {
	   var out []GraphIncident
	  th := cfg.LagThreshold
	   if th <= 0 {
		  return nil
	  }
	  cursors := append([]Cursor(nil), snap.Cursors...)
	   sort.Slice(cursors, func(i, j int) bool {
		   if cursors[i].Source != cursors[j].Source {
			   return cursors[i].Source < cursors[j].Source
		   }
		   return cursors[i].Stream < cursors[j].Stream
	   })
	   for _, c := range cursors {
		   key := c.Source + "|" + c.Stream
		   ref := c.Source + "/" + c.Stream
		  if !c.UpdatedAt.IsZero() && now.Sub(c.UpdatedAt) > th {
			   out = append(out, GraphIncident{
				   Kind:   IncidentLag,
				   Reason: ReasonCursorStall,
				  EdgeKey: key,
				  Ref:    ref,
				  Detail: fmt.Sprintf("cursor stalled %s", now.Sub(c.UpdatedAt).Truncate(time.Millisecond)),
			  })
		  }
		   if c.LagMs > th.Milliseconds() {
			  out = append(out, GraphIncident{
				   Kind:   IncidentLag,
				   Reason: ReasonEventLag,
				   EdgeKey: key,
				   Ref:     ref,
				  Detail: fmt.Sprintf("event lag %dms", c.LagMs),
			   })
		   }
	   }
	   return out
   }
