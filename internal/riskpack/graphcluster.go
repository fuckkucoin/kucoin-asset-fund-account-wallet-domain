package riskpack

import (
	"sort"
	"time"
	"github.com/shopspring/decimal"
)

   func (s *Store) find(k string) string {
	   p, ok := s.parent[k]
	   if !ok || p == "" || p == k {
		  s.parent[k] = k
		   return k
	   }
	   s.parent[k] = s.find(p)
	  return s.parent[k]
   }

  func (s *Store) union(a, b string) {
	   ra, rb := s.find(a), s.find(b)
	   if ra == rb {
		  return
	  }
	   if rb < ra {
		   ra, rb = rb, ra
	   }
	   s.parent[rb] = ra
   }

// Recompute 按当前边权重建簇，并回写节点档位。

   func (s *Store) Recompute(now time.Time) ([]ClusterExposure, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  if now.IsZero() {
		   now = s.now()
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   s.parent = map[string]string{}
	   keys := make([]string,
		  0, len(s.nodes))
	  for k := range s.nodes {
		  keys = append(keys, k)
		   s.parent[k] = k
	  }
	  sort.Strings(keys)
	   for _, e := range s.edges {
		   if e == nil || !e.Active {
			  continue
		   }
		   w := parseNonNeg(e.Weight)
		   if w.LessThan(s.threshold) {
			  continue
		   }
		   if _, ok := s.nodes[e.LeftKey]; !ok {
			   continue
		   }
		   if _, ok := s.nodes[e.RightKey]; !ok {
			   continue
		   }
		   s.union(e.LeftKey, e.RightKey)
	   }
	   members := map[string][]string{}
	  for _, k := range keys {
		  root := s.find(k)
		   cid := "clu|" + root
		   members[cid] = append(members[cid], k)
		   n := s.nodes[k]
		  n.ClusterID = cid
	   }
	  // 清掉旧的结构快照（不带资产后缀的键），资产占用行保留。
	  for id, row := range s.clusters {
		  if row == nil || row.Asset == "" {
			   delete(s.clusters, id)
		   }
	   }
	  out := make([]ClusterExposure, 0, len(members))
	   for cid, ms := range members {
		  sort.Strings(ms)
		   score, maxW, tier := s.scoreLocked(ms)
		  row := &ClusterExposure{
			   ClusterID: cid, MemberN: len(ms), Score: score.String(), Tier: tier,
			  MaxWeight: maxW.String(), Reserved: "0", Consumed: "0",
			  ComputedAt: now, Members: ms,
		  }
		   prev := s.clusters[cid]
		   if prev != nil && prev.Asset == "" {
			  row.Reserved = prev.Reserved
			   row.Consumed = prev.Consumed
		  }
		  s.clusters[cid] = row
		   for _, k := range ms {
			  n := s.nodes[k]
			  n.Score = row.Score
			   n.Tier = row.Tier
			   n.LastSeen = now
			  n.Version++
			  _ = s.persistNode(*n)
		   }
		   cp := copyExposure(row)
		  out = append(out, cp)
		   _ = s.persistCluster(cp)
	  }
	  s.pushAudit(GraphAudit{
		   Kind: AuditCluster, Subject: "graph", Before: "", After: itoa(len(out)),
		  Detail: "threshold=" + s.threshold.String(), At: now,
	   })
	   sort.Slice(out, func(i, j int) bool { return out[i].ClusterID < out[j].ClusterID })
	   return out, nil
   }

   func (s *Store) scoreLocked(members []string) (decimal.Decimal, decimal.Decimal, string) {
	  maxW := decimal.Zero
	  set := map[string]struct{}{}
	   for _, k := range members {
		   set[k] = struct{}{}
	   }
	  for _, e := range s.edges {
		   if e == nil || !e.Active {
			   continue
		   }
		  if _, ok := set[e.LeftKey]; !ok {
			  continue
		  }
		   if _, ok := set[e.RightKey]; !ok {
			  continue
		  }
		  w := parseNonNeg(e.Weight)
		  if w.GreaterThan(maxW) {
			  maxW = w
		   }
	   }
	   bump := int64(0)
	   for _, k := range members {
		  n := s.nodes[k]
		   if n == nil {
			  continue
		   }
		   if b := kytBump(n.KYTHint); b > bump {
			  bump = b
		   }
	  }
	  score := capScore(maxW.Mul(decimal.NewFromInt(100)).Add(decimal.NewFromInt(bump)))
	  return score, maxW, TierOfScore(score)
  }

// NoteOccupancy 回写某簇某资产的占用额，供簇查询与审计一致。

   func (s *Store) NoteOccupancy(clusterID, asset, reserved, consumed string) {
	  if s == nil {
		  return
	  }
	  clusterID = trim(clusterID)
	   asset = upper(asset)
	   if clusterID == "" {
		  return
	  }
	   res := parseNonNeg(reserved).String()
	   con := parseNonNeg(consumed).String()
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  base := s.clusters[clusterID]
	   key := clusterID
	  if asset != "" {
		   key = clusterID + "|" + asset
	  }
	   row := s.clusters[key]
	  if row == nil {
		   row = &ClusterExposure{ClusterID: clusterID, Asset: asset, Reserved: "0", Consumed: "0"}
		   if base != nil {
			   row.MemberN = base.MemberN
			   row.Score = base.Score
			   row.Tier = base.Tier
			   row.MaxWeight = base.MaxWeight
			  row.Members = append([]string(nil), base.Members...)
		  }
		  s.clusters[key] = row
	  }
	   row.Reserved = res
	   row.Consumed = con
	   row.ComputedAt = s.now()
	   _ = s.persistCluster(copyExposure(row))
  }

   func itoa(n int) string {
	   if n == 0 {
		  return "0"
	  }
	   neg := n < 0
	   if neg {
		   n = -n
	  }
	  var b [16]byte
	  i := len(b)
	  for n > 0 {
		   i--
		  b[i] = byte('0' + n%10)
		   n /= 10
	  }
	  if neg {
		   i--
		   b[i] = '-'
	  }
	  return string(b[i:])
  }

  func trim(s string) string {
	  for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		  s = s[1:]
	   }
	   for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		  s = s[:len(s)-1]
	   }
	  return s
  }

  func upper(s string) string {
	   b := []byte(s)
	   for i, c := range b {
		  if c >= 'a' && c <= 'z' {
			   b[i] = c - 32
		   }
	   }
	  return trim(string(b))
   }
