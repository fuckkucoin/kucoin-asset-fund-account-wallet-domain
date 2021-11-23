package riskpack

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func (s *Store) BindSQL(db *persist.SQLDB) *Store {
	   if s == nil || db == nil || db.DB == nil {
		  return s
	   }
	   s.mu.Lock()
	  s.sql = db
	  s.mu.Unlock()
	   if err := s.loadSQL(); err != nil {
		  return s
	   }
	  _, _ = s.Recompute(s.now())
	  return s
  }

   func (s *Store) persistNode(n DestNode) error {
	  if !s.sqlOn() {
		  return nil
	  }
	   ctx := context.Background()
	  q := s.sql.Conn(ctx)
	   var stmt string
	   if s.sql.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_dest_node
(node_key, chain, address, cluster_id, tier, score, kyt_hint, label, version, first_seen, last_seen)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (node_key) DO UPDATE SET
cluster_id=EXCLUDED.cluster_id, tier=EXCLUDED.tier, score=EXCLUDED.score,
kyt_hint=EXCLUDED.kyt_hint, label=EXCLUDED.label, version=EXCLUDED.version, last_seen=EXCLUDED.last_seen`
	   } else {
		   stmt = `INSERT INTO aa_dest_node
(node_key, chain, address, cluster_id, tier, score, kyt_hint, label, version, first_seen, last_seen)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
cluster_id=VALUES(cluster_id), tier=VALUES(tier), score=VALUES(score),
kyt_hint=VALUES(kyt_hint), label=VALUES(label), version=VALUES(version), last_seen=VALUES(last_seen)`
	   }
	  _, err := q.ExecContext(ctx, stmt,
		  n.Key, n.Chain, n.Address, n.ClusterID, n.Tier, n.Score, n.KYTHint, n.Label, n.Version, n.FirstSeen, n.LastSeen)
	   return err
  }

  func (s *Store) persistEdge(e DestEdge) error {
	  if !s.sqlOn() {
		   return nil
	   }
	  ctx := context.Background()
	   q := s.sql.Conn(ctx)
	   active := 0
	   if e.Active {
		  active = 1
	   }
	  var stmt string
	  if s.sql.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_dest_edge 
(edge_id, left_key, right_key, anchor_weight, weight, kind, evidence, half_life_sec, active, born_at, anchor_at, touched_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (edge_id) DO UPDATE SET
anchor_weight=EXCLUDED.anchor_weight, weight=EXCLUDED.weight, kind=EXCLUDED.kind,
evidence=EXCLUDED.evidence, half_life_sec=EXCLUDED.half_life_sec, active=EXCLUDED.active,
anchor_at=EXCLUDED.anchor_at, touched_at=EXCLUDED.touched_at`
	   } else {
		   stmt = `INSERT INTO aa_dest_edge
(edge_id, left_key, right_key, anchor_weight, weight, kind, evidence, half_life_sec, active, born_at, anchor_at, touched_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
anchor_weight=VALUES(anchor_weight), weight=VALUES(weight), kind=VALUES(kind),
evidence=VALUES(evidence), half_life_sec=VALUES(half_life_sec), active=VALUES(active),
anchor_at=VALUES(anchor_at), touched_at=VALUES(touched_at)`
	   }
	  _, err := q.ExecContext(ctx, stmt,
		   e.ID, e.LeftKey, e.RightKey, e.AnchorWeight, e.Weight, e.Kind, e.Evidence,
		   e.HalfLifeSec,
		   active, e.BornAt, e.AnchorAt, e.TouchedAt)
	   return err
   }

  func (s *Store) persistCluster(c ClusterExposure) error {
	  if !s.sqlOn() {
		   return nil
	   }
	   ctx := context.Background()
	   q := s.sql.Conn(ctx)
	   asset := c.Asset
	   var stmt string
	   if s.sql.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_dest_cluster 
(cluster_id, asset, member_n, score, tier, max_weight, reserved_amt, consumed_amt, computed_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT (cluster_id, asset) DO UPDATE SET
member_n=EXCLUDED.member_n, score=EXCLUDED.score, tier=EXCLUDED.tier,
max_weight=EXCLUDED.max_weight, reserved_amt=EXCLUDED.reserved_amt,
consumed_amt=EXCLUDED.consumed_amt, computed_at=EXCLUDED.computed_at`
	  } else {
		   stmt = `INSERT INTO aa_dest_cluster
(cluster_id, asset, member_n, score, tier, max_weight, reserved_amt, consumed_amt, computed_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
member_n=VALUES(member_n), score=VALUES(score), tier=VALUES(tier),
max_weight=VALUES(max_weight), reserved_amt=VALUES(reserved_amt),
consumed_amt=VALUES(consumed_amt), computed_at=VALUES(computed_at)`
	  }
	  _, err := q.ExecContext(ctx, stmt,
		  c.ClusterID, asset, c.MemberN, c.Score, c.Tier, c.MaxWeight, c.Reserved, c.Consumed, c.ComputedAt)
	   return err
  }

  func (s *Store) persistAudit(a GraphAudit) error {
	  if !s.sqlOn() {
		   return nil
	  }
	  ctx := context.Background()
	  _, err := s.sql.Conn(ctx).ExecContext(ctx, `INSERT INTO aa_dest_risk_audit
(audit_id, kind, subject, before_v, after_v, actor, detail, at_time)
VALUES (?,?,?,?,?,?,?,?)`,
		   a.ID, a.Kind, a.Subject, a.Before, a.After, "", a.Detail, a.At)
	  return err
   }

  func (s *Store) loadSQL() error {
	   if !s.sqlOn() {
		   return nil
	   }
	  ctx := context.Background()
	   q := s.sql.Conn(ctx)
	  rows, err := q.QueryContext(ctx, `SELECT node_key, chain, address, cluster_id, tier, score, kyt_hint, label, version, first_seen, last_seen FROM aa_dest_node`)
	   if err != nil {
		   return err
	  }
	  defer rows.Close()
	   nodes := map[string]*DestNode{}
	   for rows.Next() {
		   var n DestNode
		   if err := rows.Scan(&n.Key, &n.Chain, &n.Address, &n.ClusterID, &n.Tier, &n.Score, &n.KYTHint, &n.Label, &n.Version, &n.FirstSeen, &n.LastSeen); err != nil {
			  return err
		   }
		  cp := n
		   nodes[n.Key] = &cp
	  }
	  if err := rows.Err(); err != nil {
		   return err
	  }
	  erows, err := q.QueryContext(ctx, `SELECT edge_id, left_key, right_key, anchor_weight, weight, kind, evidence, half_life_sec, active, born_at, anchor_at, touched_at FROM aa_dest_edge`)
	   if err != nil {
		  return err
	   }
	  defer erows.Close()
	  edges := map[string]*DestEdge{}
	  pair := map[string]string{}
	   for erows.Next() {
		   var e DestEdge
		   var active int
		   if err := erows.Scan(&e.ID, &e.LeftKey, &e.RightKey, &e.AnchorWeight, &e.Weight, &e.Kind, &e.Evidence, &e.HalfLifeSec, &active, &e.BornAt, &e.AnchorAt, &e.TouchedAt); err != nil {
			  return err
		   }
		  e.Active = active != 0
		   cp := e
		   edges[e.ID] = &cp
		  pair[pairKey(e.LeftKey, e.RightKey)] = e.ID
	  }
	   if err := erows.Err(); err != nil {
		  return err
	   }
	   crows, err := q.QueryContext(ctx, `SELECT cluster_id, asset, member_n, score, tier, max_weight, reserved_amt, consumed_amt, computed_at FROM aa_dest_cluster`)
	  if err != nil {
		  return err
	   }
	   defer crows.Close()
	   clusters := map[string]*ClusterExposure{}
	   for crows.Next() {
		  var c ClusterExposure
		  if err := crows.Scan(&c.ClusterID, &c.Asset, &c.MemberN, &c.Score, &c.Tier, &c.MaxWeight, &c.Reserved, &c.Consumed, &c.ComputedAt); err != nil {
			  return err
		   }
		   cp := c
		   key := c.ClusterID
		  if c.Asset != "" {
			   key = c.ClusterID + "|" + c.Asset
		   }
		  clusters[key] = &cp
	   }
	   if err := crows.Err(); err != nil {
		   return err
	   }
	   if len(nodes) == 0 && len(edges) == 0 {
		  return nil
	   }
	   s.mu.Lock()
	  s.nodes = nodes
	  s.edges = edges
	   s.pair = pair
	  if len(clusters) > 0 {
		  s.clusters = clusters
	   }
	  s.mu.Unlock()
	   return nil
  }

// TouchClock 与 worker 一致时刻。零值回落到当前时钟。
   func (s *Store) TouchClock(now time.Time) {
	  if s == nil {
		   return
	   }
	  if now.IsZero() {
		  return
	   }
	  frozen := now.UTC()
	   s.WithClock(func() time.Time { return frozen })
   }
