package fundgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// 运维标记不参与幂等比较，避免冷热标记把重放误判成冲突。
var operationalAttrs = map[string]bool{
	"tier": true,
	"tier_at": true,
}

// NodeID 用种类和业务引用导出稳定主键。过长时退化为带种类前缀的摘要，避免超出索引长度。

  func NodeID(kind NodeKind, ref string) string {
	  raw := string(kind) + ":" + strings.TrimSpace(ref)
	   if len(raw) <= 120 {
		  return raw
	   }
	   sum := sha256.Sum256([]byte(raw))
	   prefix := string(kind)
	   if len(prefix) > 24 {
		  prefix = prefix[:24]
	  }
	  return prefix + ":" + hex.EncodeToString(sum[:16])
   }

// EdgeID 用四元组导出稳定边主键。
   func EdgeID(from, to string, typ EdgeType, ref string) string {
	   sum := sha256.Sum256([]byte(from + "|" + to + "|" + string(typ) + "|" + ref))
	  return "e_" + hex.EncodeToString(sum[:16])
   }

// String 是边键的可读形式。超长时改为摘要，保证巡检单列能放下。
  func (k EdgeKey) String() string {
	   s := k.From + "|" + k.To + "|" + string(k.Type) + "|" + k.Ref
	   if len(s) <= 300 {
		  return s
	  }
	  sum := sha256.Sum256([]byte(s))
	   return "k_" + hex.EncodeToString(sum[:16])
   }

// IncidentID 由开单指纹导出，同一问题重复巡检命中同一张单。
  func IncidentID(kind, reason, ref, nodeID, edgeKey string) string {
	  raw := kind + "|" + reason + "|" + ref + "|" + nodeID + "|" + edgeKey
	   sum := sha256.Sum256([]byte(raw))
	  return "inc_" + hex.EncodeToString(sum[:16])
  }

// AuditID 导出审计行主键。
  func AuditID(action, edgeKey, nodeID string, at time.Time) string {
	   raw := action + "|" + edgeKey + "|" + nodeID + "|" + at.UTC().Format(time.RFC3339Nano)
	  sum := sha256.Sum256([]byte(raw))
	  return "aud_" + hex.EncodeToString(sum[:16])
   }

// NormAmount 把十进制金额收成无指数的规范字符串。空串合法。负数非法。
  func NormAmount(s string) (string, error) {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return "", nil
	  }
	   d, err := decimal.NewFromString(s)
	   if err != nil {
		  return "", err
	   }
	  if d.IsNegative() {
		  return "", ErrInvalid
	   }
	  return d.String(), nil
   }

// NormAsset 统一资产代码为大写。

  func NormAsset(s string) string {
	   return strings.ToUpper(strings.TrimSpace(s))
   }

  func normMap(in map[string]string) map[string]string {
	   if len(in) == 0 {
		  return nil
	  }
	  out := make(map[string]string, len(in))
	  for k, v := range in {
		   k = strings.TrimSpace(k)
		  v = strings.TrimSpace(v)
		  if k == "" || v == "" {
			  continue
		   }
		  out[k] = v
	  }
	   if len(out) == 0 {
		   return nil
	  }
	  return out
  }

  func cloneMap(in map[string]string) map[string]string {
	   if len(in) == 0 {
		   return nil
	   }
	   out := make(map[string]string, len(in))
	  for k, v := range in {
		  out[k] = v
	   }
	   return out
   }

  func cloneNode(n FundNode) FundNode {
	  n.Attrs = cloneMap(n.Attrs)
	  return n
  }

   func cloneEdge(e FundEdge) FundEdge {
	  e.Attrs = cloneMap(e.Attrs)
	   return e
  }

// canonAttrs 生成与键顺序无关的属性串，运维标记除外。

  func canonAttrs(m map[string]string) string {
	  if len(m) == 0 {
		   return ""
	   }
	  keys := make([]string, 0, len(m))
	   for k := range m {
		   if operationalAttrs[k] {
			   continue
		   }
		  keys = append(keys, k)
	   }
	  sort.Strings(keys)
	  var b strings.Builder
	   for _, k := range keys {
		  v := m[k]
		   if strings.TrimSpace(v) == "" {
			  continue
		   }
		  b.WriteString(k)
		  b.WriteByte('=')
		  b.WriteString(v)
		   b.WriteByte(';')
	   }
	  return b.String()
  }

  func sameTime(stored, incoming time.Time) bool {
	   if incoming.IsZero() || stored.IsZero() {
		  return true
	   }
	  return stored.UTC().Equal(incoming.UTC())
   }

  func timeText(t time.Time) string {
	  if t.IsZero() {
		  return ""
	   }
	  return t.UTC().Format(time.RFC3339Nano)
   }

// diffNode 比较两份节点的业务属性。一致时返回空。

   func diffNode(stored, incoming FundNode) []Conflict {
	   var out []Conflict
	   add := func(field, a, b string) {
		   if a == b {
			  return
		   }
		   out = append(out, Conflict{NodeID: stored.ID, Field: field, Stored: a, Incoming: b})
	   }
	  add("kind", string(stored.Kind), string(incoming.Kind))
	  add("ref", stored.Ref, incoming.Ref)
	  add("user_id", stored.UserID, incoming.UserID)
	   add("asset", stored.Asset, incoming.Asset)
	  add("amount", stored.Amount, incoming.Amount)
	  add("chain", stored.Chain, incoming.Chain)
	   add("tx_hash", stored.TxHash, incoming.TxHash)
	  add("attrs", canonAttrs(stored.Attrs), canonAttrs(incoming.Attrs))
	  if !sameTime(stored.OccurredAt, incoming.OccurredAt) {
		   add("occurred_at", timeText(stored.OccurredAt), timeText(incoming.OccurredAt))
	  }
	   return out
  }

// diffEdge 比较两份边的业务属性。来源名称不参与比较，先写入的来源保留。

   func diffEdge(stored, incoming FundEdge) []Conflict {
	  var out []Conflict
	   key := stored.Key().String()
	   add := func(field, a, b string) {
		  if a == b {
			  return
		  }
		   out = append(out, Conflict{EdgeKey: key, Field: field, Stored: a, Incoming: b})
	   }
	  add("from_id", stored.FromID, incoming.FromID)
	  add("to_id", stored.ToID, incoming.ToID)
	   add("edge_type", string(stored.EdgeType), string(incoming.EdgeType))
	   add("ref", stored.Ref, incoming.Ref)
	  add("user_id", stored.UserID, incoming.UserID)
	   add("asset", stored.Asset, incoming.Asset)
	  add("amount", stored.Amount, incoming.Amount)
	   add("envelope_id", stored.EnvelopeID, incoming.EnvelopeID)
	   add("attrs", canonAttrs(stored.Attrs), canonAttrs(incoming.Attrs))
	  if !sameTime(stored.OccurredAt, incoming.OccurredAt) {
		  add("occurred_at", timeText(stored.OccurredAt), timeText(incoming.OccurredAt))
	  }
	  return out
   }

   func conflictsJSON(list []Conflict) string {
	  if len(list) == 0 {
		  return ""
	  }
	  b, err := json.Marshal(list)
	   if err != nil {
		  return "conflict"
	  }
	   return string(b)
  }

// prepareNode 清洗节点。ID 为空时按种类和引用生成。
  func prepareNode(n FundNode, now time.Time, source string) (FundNode, error) {
	   n.Kind = NodeKind(strings.TrimSpace(string(n.Kind)))
	  n.Ref = strings.TrimSpace(n.Ref)
	   n.UserID = strings.TrimSpace(n.UserID)
	   n.Asset = NormAsset(n.Asset)
	  n.Chain = strings.ToUpper(strings.TrimSpace(n.Chain))
	   n.TxHash = strings.TrimSpace(n.TxHash)
	  n.Source = strings.TrimSpace(n.Source)
	   if n.Source == "" {
		   n.Source = source
	   }
	  amt, err := NormAmount(n.Amount)
	  if err != nil {
		  return FundNode{}, err
	   }
	   n.Amount = amt
	  n.Attrs = normMap(n.Attrs)
	   if n.Kind == "" || !KnownNodeKind(n.Kind) {
		  return FundNode{}, ErrInvalid
	   }
	  if n.Ref == "" {
		   return FundNode{}, ErrInvalid
	  }
	   if strings.TrimSpace(n.ID) == "" {
		  n.ID = NodeID(n.Kind, n.Ref)
	  } else {
		   n.ID = strings.TrimSpace(n.ID)
	  }
	  if !n.OccurredAt.IsZero() {
		   n.OccurredAt = n.OccurredAt.UTC()
	  }
	   if n.IngestedAt.IsZero() {
		  n.IngestedAt = now.UTC()
	   } else {
		  n.IngestedAt = n.IngestedAt.UTC()
	  }
	  return n, nil
   }

// prepareEdge 清洗边并补齐稳定主键。

   func prepareEdge(e FundEdge, now time.Time, source string) (FundEdge, error) {
	   e.FromID = strings.TrimSpace(e.FromID)
	   e.ToID = strings.TrimSpace(e.ToID)
	  e.EdgeType = EdgeType(strings.TrimSpace(string(e.EdgeType)))
	   e.Ref = strings.TrimSpace(e.Ref)
	   e.UserID = strings.TrimSpace(e.UserID)
	  e.Asset = NormAsset(e.Asset)
	   e.Source = strings.TrimSpace(e.Source)
	   e.EnvelopeID = strings.TrimSpace(e.EnvelopeID)
	  if e.Source == "" {
		  e.Source = source
	   }
	   amt, err := NormAmount(e.Amount)
	  if err != nil {
		   return FundEdge{}, err
	  }
	  e.Amount = amt
	   e.Attrs = normMap(e.Attrs)
	  if e.FromID == "" || e.ToID == "" || e.Ref == "" || !KnownEdgeType(e.EdgeType) {
		   return FundEdge{}, ErrInvalid
	   }
	   if strings.TrimSpace(e.ID) == "" {
		   e.ID = EdgeID(e.FromID, e.ToID, e.EdgeType, e.Ref)
	   } else {
		   e.ID = strings.TrimSpace(e.ID)
	  }
	  if !e.OccurredAt.IsZero() {
		  e.OccurredAt = e.OccurredAt.UTC()
	  }
	  if e.IngestedAt.IsZero() {
		   e.IngestedAt = now.UTC()
	   } else {
		   e.IngestedAt = e.IngestedAt.UTC()
	   }
	  return e, nil
   }

// prepareBatch 清洗整批事实。空批次且没有来源时拒绝。
   func prepareBatch(b IngestBatch, now time.Time) (IngestBatch, error) {
	   now = now.UTC()
	  if now.IsZero() {
		   now = time.Now().UTC()
	  }
	   b.Source = strings.TrimSpace(b.Source)
	  b.Stream = strings.TrimSpace(b.Stream)
	   if b.Stream == "" {
		   b.Stream = "default"
	  }
	   b.Cursor = strings.TrimSpace(b.Cursor)
	   if !b.EventAt.IsZero() {
		   b.EventAt = b.EventAt.UTC()
	  }
	   if len(b.Nodes) == 0 && len(b.Edges) == 0 && b.Source == "" {
		  return IngestBatch{}, ErrInvalid
	   }
	  nodes := make([]FundNode,
		  0, len(b.Nodes))
	   for _, n := range b.Nodes {
		  pn, err := prepareNode(n, now, b.Source)
		   if err != nil {
			   return IngestBatch{}, err
		  }
		   nodes = append(nodes, pn)
	   }
	   edges := make([]FundEdge, 0, len(b.Edges))
	   for _, e := range b.Edges {
		  pe, err := prepareEdge(e, now, b.Source)
		   if err != nil {
			  return IngestBatch{}, err
		  }
		   edges = append(edges, pe)
	   }
	   b.Nodes = nodes
	  b.Edges = edges
	   return b, nil
   }

   func cursorFromBatch(b IngestBatch, prev Cursor, now time.Time) Cursor {
	  c := Cursor{
		   Source: b.Source,
		  Stream: b.Stream,
		  Token:  b.Cursor,
		   LastEventAt: b.EventAt,
		  UpdatedAt:   now.UTC(),
	   }
	   if c.Token == "" {
		   c.Token = prev.Token
	  }
	  if c.LastEventAt.IsZero() {
		  c.LastEventAt = prev.LastEventAt
	  }
	   if !c.LastEventAt.IsZero() {
		  lag := now.Sub(c.LastEventAt).Milliseconds()
		   if lag < 0 {
			   lag = 0
		  }
		   c.LagMs = lag
	   }
	  return c
   }

   func stampOccurred(t, now time.Time) time.Time {
	   if t.IsZero() {
		   return now.UTC()
	   }
	   return t.UTC()
   }

// nodeReplay 判断已存在节点是幂等回放还是属性冲突。读不到原行时失败即关。

   func nodeReplay(stored FundNode, incoming FundNode, found bool) (bool, []Conflict) {
	  if !found {
		   return false, []Conflict{{NodeID: incoming.ID, Field: "id", Incoming: incoming.ID}}
	   }
	  cs := diffNode(stored, incoming)
	  if len(cs) > 0 {
		  return false, cs
	   }
	  return true, nil
  }

// edgeReplay 判断已存在边是幂等回放还是属性冲突。主键被另一条边占用时失败即关。

  func edgeReplay(stored FundEdge, incoming FundEdge, found bool) (bool, []Conflict) {
	   if !found {
		  return false, []Conflict{{EdgeKey: incoming.Key().String(), Field: "id", Incoming: incoming.ID}}
	   }
	   if stored.Key().String() != incoming.Key().String() {
		  return false, []Conflict{{
			  EdgeKey: incoming.Key().String(),
			   Field:    "id",
			  Stored:   stored.Key().String(),
			  Incoming: incoming.Key().String(),
		  }}
	   }
	   cs := diffEdge(stored, incoming)
	   if len(cs) > 0 {
		   return false, cs
	   }
	   return true, nil
   }
