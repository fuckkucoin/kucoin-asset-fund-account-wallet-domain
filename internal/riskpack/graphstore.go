package riskpack

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Store 持久图谱。簇关系不在写入边时立刻固化，须 Recompute。
type Store struct {
	mu sync.Mutex
	nodes    map[string]*DestNode
	edges    map[string]*DestEdge
	pair     map[string]string
	clusters map[string]*ClusterExposure
	parent map[string]string
	audits []GraphAudit
	sql    *persist.SQLDB
	clock  func() time.Time
	threshold decimal.Decimal
	floor     decimal.Decimal
}

  func NewStore() *Store {
	   th, _ := decimal.NewFromString(DefaultClusterThreshold)
	  fl,
		  _ := decimal.NewFromString(DefaultWeightFloor)
	   return &Store{
		   nodes: map[string]*DestNode{}, edges: map[string]*DestEdge{},
		   pair: map[string]string{}, clusters: map[string]*ClusterExposure{},
		  parent: map[string]string{}, threshold: th, floor: fl,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (s *Store) WithClock(c func() time.Time) *Store {
	  if s != nil && c != nil {
		   s.clock = c
	  }
	   return s
   }

  func (s *Store) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	   return s.clock().UTC()
  }

   func (s *Store) sqlOn() bool {
	   return s != nil && s.sql != nil && s.sql.DB != nil
   }

// SetThreshold 设置连通阈，取值闭区间 [0,1]。0 表示任意正权边都连通。
  func (s *Store) SetThreshold(v string) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  d, err := decimal.NewFromString(strings.TrimSpace(v))
	   if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(1)) {
		   return ErrDest
	   }
	   s.mu.Lock()
	   s.threshold = d
	  s.mu.Unlock()
	   return nil
   }

  func (s *Store) Threshold() string {
	   if s == nil {
		   return DefaultClusterThreshold
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.threshold.String()
   }

  func normDest(chain, addr string) (key, ch, a string, err error) {
	  ch, err = validate.Chain(chain)
	  if err != nil {
		   return "", "", "", err
	   }
	   a, err = validate.Address(ch, addr)
	   if err != nil {
		  a = strings.TrimSpace(addr)
		   if a == "" {
			  return "", "", "", err
		  }
		  if e2 := wallet.ValidChainAddress(ch, a); e2 != nil && len(a) < 8 {
			  return "", "", "", err
		   }
	  }
	  return nodeKey(ch, a), ch, a, nil
  }

// UpsertNode 登记或刷新节点。label 空则保留原备注。

  func (s *Store) UpsertNode(chain, addr, label string) (DestNode, error) {
	   if s == nil {
		  return DestNode{}, ErrUnavailable
	   }
	  key, ch, a, err := normDest(chain, addr)
	  if err != nil {
		   return DestNode{}, err
	   }
	   label = strings.TrimSpace(label)
	   now := s.now()
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  cur := s.nodes[key]
	   if cur == nil {
		   cur = &DestNode{
			  Key: key, Chain: ch, Address: a, ClusterID: "clu|" + key,
			  Tier: TierLow, Score: "0", Label: label, Version: 1,
			  FirstSeen: now, LastSeen: now,
		  }
		   s.nodes[key] = cur
		   s.parent[key] = key
		   s.pushAudit(GraphAudit{
			  Kind: AuditNodeUpsert, Subject: key, Before: "", After: cur.ClusterID,
			   Detail: "insert", At: now,
		   })
	  } else {
		  before := cur.Label
		   if label != "" {
			   cur.Label = label
		   }
		   cur.Version++
		   cur.LastSeen = now
		   s.pushAudit(GraphAudit{
			  Kind: AuditNodeUpsert, Subject: key, Before: before, After: cur.Label,
			  Detail: "touch", At: now,
		   })
	  }
	   out := *cur
	  if err := s.persistNode(out); err != nil {
		   return DestNode{}, err
	   }
	  return out, nil
  }

// Link 写入或刷新一条无向边，并立刻重算簇。

   func (s *Store) Link(aChain, aAddr, bChain, bAddr, weight, kind, evidence string, halfLife time.Duration) (DestEdge, error) {
	   if s == nil {
		  return DestEdge{}, ErrUnavailable
	  }
	   lk, _, _, err := normDest(aChain, aAddr)
	  if err != nil {
		   return DestEdge{}, err
	  }
	   rk, _, _, err := normDest(bChain, bAddr)
	  if err != nil {
		   return DestEdge{}, err
	   }
	  if lk == rk {
		   return DestEdge{}, ErrDest
	  }
	   w, err := parseWeight(weight)
	   if err != nil {
		  return DestEdge{}, err
	   }
	  kind = strings.TrimSpace(kind)
	  if kind == "" {
		  kind = EdgeExplicit
	  }
	   if halfLife <= 0 {
		   halfLife = 24 * time.Hour
	   }
	  evidence = strings.TrimSpace(evidence)
	   if len(evidence) > 240 {
		  evidence = evidence[:240]
	   }
	   if _, err := s.UpsertNode(aChain, aAddr, ""); err != nil {
		   return DestEdge{}, err
	  }
	   if _, err := s.UpsertNode(bChain, bAddr, ""); err != nil {
		   return DestEdge{}, err
	  }
	   now := s.now()
	   s.mu.Lock()
	  pk := pairKey(lk, rk)
	  id := s.pair[pk]
	   var edge *DestEdge
	  before := ""
	   if id != "" {
		   edge = s.edges[id]
	  }
	  if edge == nil {
		  edge = &DestEdge{ID: uuid.NewString(), LeftKey: lk, RightKey: rk, BornAt: now}
		   s.edges[edge.ID] = edge
		   s.pair[pk] = edge.ID
	  } else {
		  before = edge.Weight
	   }
	   edge.Kind = kind
	   edge.Evidence = evidence
	  edge.AnchorWeight = w.String()
	   edge.Weight = w.String()
	   edge.HalfLifeSec = int64(halfLife / time.Second)
	  if edge.HalfLifeSec <= 0 {
		  edge.HalfLifeSec = 1
	   }
	   edge.Active = true
	   edge.AnchorAt = now
	  edge.TouchedAt = now
	  out := *edge
	   s.pushAudit(GraphAudit{
		   Kind:    AuditEdgeLink,
		   Subject: edge.ID, Before: before, After: edge.Weight,
		  Detail: kind + " " + pk, At: now,
	  })
	   if err := s.persistEdge(out); err != nil {
		   s.mu.Unlock()
		  return DestEdge{}, err
	  }
	   s.mu.Unlock()
	   if _, err := s.Recompute(now); err != nil {
		  return DestEdge{}, err
	   }
	  return out, nil
  }

// Node 读取节点副本。

  func (s *Store) Node(chain, addr string) (DestNode, bool) {
	  if s == nil {
		   return DestNode{}, false
	   }
	   key, _, _, err := normDest(chain, addr)
	  if err != nil {
		   return DestNode{}, false
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   n := s.nodes[key]
	   if n == nil {
		  return DestNode{}, false
	   }
	   return *n, true
   }

// NodeByKey 按归一键读取。

   func (s *Store) NodeByKey(key string) (DestNode, bool) {
	  if s == nil {
		  return DestNode{}, false
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   n := s.nodes[key]
	  if n == nil {
		  return DestNode{}, false
	   }
	   return *n, true
  }

// EdgesOf 返回与节点相连的边副本。
  func (s *Store) EdgesOf(chain, addr string) []DestEdge {
	  if s == nil {
		   return nil
	  }
	   key, _, _, err := normDest(chain, addr)
	   if err != nil {
		   return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]DestEdge, 0)
	  for _, e := range s.edges {
		  if e.LeftKey == key || e.RightKey == key {
			  out = append(out, *e)
		   }
	   }
	  return out
   }

// Edge 按标识读取边。

   func (s *Store) Edge(id string) (DestEdge, bool) {
	   if s == nil {
		  return DestEdge{}, false
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  e := s.edges[strings.TrimSpace(id)]
	  if e == nil {
		   return DestEdge{}, false
	   }
	   return *e, true
   }

// Cluster 读取簇快照。结构分数以最近一次重算为准，资产行只覆盖占用量。
  func (s *Store) Cluster(id, asset string) (ClusterExposure, bool) {
	  if s == nil {
		  return ClusterExposure{}, false
	   }
	  id = trim(id)
	  asset = upper(asset)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   base := s.clusters[id]
	  var occ *ClusterExposure
	  if asset != "" {
		   occ = s.clusters[id+"|"+asset]
	   }
	   if base == nil && occ == nil {
		   return ClusterExposure{}, false
	   }
	   var out ClusterExposure
	  if base != nil {
		   out = copyExposure(base)
	  } else {
		   out = copyExposure(occ)
	  }
	   if occ != nil {
		   out.Asset = asset
		  out.Reserved = occ.Reserved
		  out.Consumed = occ.Consumed
		  if out.ClusterID == "" {
			  out.ClusterID = occ.ClusterID
		   }
	   }
	  if asset != "" {
		   out.Asset = asset
	  }
	  return out, true
   }

// ExposureOf 返回地址当前簇敞口。节点不存在则先按家族簇给一份空快照。
   func (s *Store) ExposureOf(chain, addr, asset string) (ClusterExposure, error) {
	   if s == nil {
		   return ClusterExposure{},
			   ErrUnavailable
	   }
	   n, ok := s.Node(chain, addr)
	   if !ok {
		  if _, err := s.UpsertNode(chain, addr, ""); err != nil {
			  return ClusterExposure{}, err
		  }
		  if _, err := s.Recompute(s.now()); err != nil {
			  return ClusterExposure{}, err
		  }
		   n, ok = s.Node(chain, addr)
		   if !ok {
			   return ClusterExposure{}, ErrDest
		   }
	   }
	  if c, ok := s.Cluster(n.ClusterID, asset); ok {
		  return c, nil
	  }
	   return ClusterExposure{
		  ClusterID: n.ClusterID,
		  Asset:     strings.ToUpper(strings.TrimSpace(asset)),
		  MemberN:   1, Score: n.Score, Tier: n.Tier, MaxWeight: "0",
		  Reserved: "0", Consumed: "0", ComputedAt: n.LastSeen, Members: []string{n.Key},
	  }, nil
   }

// Nodes 拷贝全部节点。
   func (s *Store) Nodes() []DestNode {
	  if s == nil {
		   return nil
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]DestNode, 0, len(s.nodes))
	  for _, n := range s.nodes {
		   out = append(out, *n)
	  }
	   return out
   }

// Audits 按种类过滤审计，kind 空则全部。新记录在后。

   func (s *Store) Audits(kind string, limit int) []GraphAudit {
	   if s == nil {
		   return nil
	   }
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	   kind = strings.TrimSpace(kind)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]GraphAudit, 0, limit)
	  for i := len(s.audits) - 1; i >= 0 && len(out) < limit; i-- {
		  if kind != "" && s.audits[i].Kind != kind {
			  continue
		  }
		  out = append(out, s.audits[i])
	  }
	   return out
   }

  func (s *Store) pushAudit(a GraphAudit) {
	  if a.ID == "" {
		  a.ID = uuid.NewString()
	  }
	  if a.At.IsZero() {
		  a.At = s.now()
	   }
	  s.audits = append(s.audits, a)
	  if len(s.audits) > 512 {
		   s.audits = append([]GraphAudit(nil),
			  s.audits[len(s.audits)-512:]...)
	   }
	  _ = s.persistAudit(a)
   }

  func copyExposure(c *ClusterExposure) ClusterExposure {
	   if c == nil {
		   return ClusterExposure{}
	   }
	  out := *c
	   if c.Members != nil {
		   out.Members = append([]string(nil), c.Members...)
	   }
	  return out
  }
