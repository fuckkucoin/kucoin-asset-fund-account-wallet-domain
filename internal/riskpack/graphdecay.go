package riskpack

import (
	"time"
	"github.com/shopspring/decimal"
)

// Decay 按给定时刻重算全部活动边的当前权。返回权值发生变化的边数。
  func (s *Store) Decay(now time.Time) (int, error) {
	  if s == nil {
		   return 0, ErrUnavailable
	  }
	   if now.IsZero() {
		   now = s.now()
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   n := 0
	   for _, e := range s.edges {
		  if e == nil || !e.Active {
			  continue
		   }
		   anchor := parseNonNeg(e.AnchorWeight)
		   if anchor.IsZero() {
			   anchor = parseNonNeg(e.Weight)
		   }
		  next := WeightAt(anchor, e.AnchorAt, now, e.HalfLifeSec)
		   before := e.Weight
		  active := true
		  if next.LessThan(s.floor) {
			   active = false
		   }
		   changed := before != next.String() || e.Active != active
		   e.Weight = next.String()
		  e.Active = active
		   e.TouchedAt = now
		   if !changed {
			  continue
		  }
		  n++
		  detail := "half_life_sec=" + itoa(int(e.HalfLifeSec)) +
			  " threshold=" + s.threshold.String() +
			   " floor=" + s.floor.String() +
			  " anchor=" + e.AnchorWeight
		  s.pushAudit(GraphAudit{
			  Kind: AuditEdgeDecay, Subject: e.ID, Before: before, After: e.Weight,
			   Detail: detail, At: now,
		   })
		  if err := s.persistEdge(*e); err != nil {
			   return n, err
		  }
	   }
	  if n == 0 {
		   s.pushAudit(GraphAudit{
			   Kind: AuditEdgeDecay, Subject: "graph", Before: "0", After: "0",
			  Detail: "no weight crossed a half-life", At: now,
		   })
	  }
	  return n, nil
   }

// ProjectWeight 只读投影某边在时刻 now 的权，不落库。
   func (s *Store) ProjectWeight(edgeID string, now time.Time) (decimal.Decimal, bool) {
	   if s == nil {
		  return decimal.Zero, false
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  e := s.edges[edgeID]
	  if e == nil {
		   return decimal.Zero, false
	   }
	   anchor := parseNonNeg(e.AnchorWeight)
	   if now.IsZero() {
		   now = s.now()
	   }
	   return WeightAt(anchor, e.AnchorAt, now, e.HalfLifeSec), true
   }
