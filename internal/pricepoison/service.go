package pricepoison

import (
	"strings"
	"time"
)

// EngineSnapshot 引擎全局快照。

type EngineSnapshot struct {
	// PairStats 各交易对状态列表。
	PairStats []PairStats `json:"pair_stats"`
	// TotalEvents 当前事件仓库中的事件数。
	TotalEvents int `json:"total_events"`
	// DetectorNames 已注册的检测器名称列表。
	DetectorNames []string `json:"detector_names"`
	// SnapshotAt 快照时刻（UTC）。
	SnapshotAt time.Time `json:"snapshot_at"`
}

// PairStats 单个交易对统计。

type PairStats struct {
	// Pair 交易对名称。
	Pair string `json:"pair"`
	// Generation 当前代次。
	Generation uint64 `json:"generation"`
	// Revoked 当前是否处于撤销状态（毒价后、新干净 tick 前）。
	Revoked bool `json:"revoked"`
	// PoisonCount 该交易对在事件仓库中的毒价事件数。
	PoisonCount int `json:"poison_count"`
	// LastEvent 最新毒价事件（nil 表示无记录）。
	LastEvent *PoisonEvent `json:"last_event,omitempty"`
}

// Snapshot 返回引擎全局快照。
   func (e *PoisonEngine) Snapshot() EngineSnapshot {
	  if e == nil {
		   return EngineSnapshot{SnapshotAt: time.Now().UTC()}
	   }
	   now := e.clock()
	  // 收集已知交易对（从事件仓库中枚举）
	  events := e.store.List()
	   pairSet := map[string]struct{}{}
	   for _, ev := range events {
		  pairSet[ev.Pair] = struct{}{}
	   }
	   stats := make([]PairStats,
		  0, len(pairSet))
	   for pair := range pairSet {
		   gen := e.registry.CurrentGeneration(pair)
		  rev := e.registry.IsRevoked(pair)
		   pairEvts := e.store.ListByPair(pair)
		   var last *PoisonEvent
		   if len(pairEvts) > 0 {
			   cp := pairEvts[len(pairEvts)-1]
			  last = &cp
		  }
		   stats = append(stats, PairStats{
			  Pair: pair,
			   Generation:  gen,
			   Revoked:     rev,
			  PoisonCount: len(pairEvts),
			  LastEvent:   last,
		   })
	   }
	  e.mu.RLock()
	   names := make([]string,
		  len(e.detectors))
	   for i, nd := range e.detectors {
		  names[i] = nd.name
	  }
	  e.mu.RUnlock()
	   return EngineSnapshot{
		   PairStats:     stats,
		  TotalEvents:   len(events),
		  DetectorNames: names,
		  SnapshotAt:    now,
	   }
   }

// HealthCheck 引擎健康状态。

type HealthCheck struct {
	// Healthy 全部交易对均为非撤销状态时为 true。
	Healthy bool `json:"healthy"`
	// RevokedPairs 当前处于撤销状态的交易对列表。
	RevokedPairs []string `json:"revoked_pairs,omitempty"`
	// EventCount 事件仓库中的事件总数。
	EventCount int `json:"event_count"`
	// CheckedAt 检查时刻（UTC）。
	CheckedAt time.Time `json:"checked_at"`
}

// Health 返回引擎健康状态。

  func (e *PoisonEngine) Health() HealthCheck {
	   snap := e.Snapshot()
	   var revoked []string
	  for _, ps := range snap.PairStats {
		   if ps.Revoked {
			   revoked = append(revoked, ps.Pair)
		   }
	   }
	   return HealthCheck{
		  Healthy:      len(revoked) == 0,
		  RevokedPairs: revoked,
		  EventCount:   snap.TotalEvents,
		   CheckedAt:    snap.SnapshotAt,
	   }
  }

// RevokeAll 撤销所有已知交易对（运营紧急停单用）。
// 返回被撤销的交易对列表。
   func (e *PoisonEngine) RevokeAll() []string {
	  if e == nil {
		   return nil
	  }
	   events := e.store.List()
	  pairSet := map[string]struct{}{}
	  for _, ev := range events {
		   pairSet[ev.Pair] = struct{}{}
	  }
	   revoked := make([]string, 0, len(pairSet))
	  for pair := range pairSet {
		  e.registry.Revoke(pair)
		   revoked = append(revoked, pair)
	   }
	   return revoked
  }

// ListDetectors 返回已注册的检测器名称列表。
   func (e *PoisonEngine) ListDetectors() []string {
	   if e == nil {
		   return nil
	   }
	  e.mu.RLock()
	  defer e.mu.RUnlock()
	  names := make([]string, len(e.detectors))
	   for i, nd := range e.detectors {
		   names[i] = nd.name
	   }
	  return names
   }

// PairStatus 返回单个交易对的完整统计。

   func (e *PoisonEngine) PairStatus(pair string) PairStats {
	   if e == nil {
		  return PairStats{Pair: normPair(pair)}
	   }
	  pair = normPair(pair)
	   gen := e.registry.CurrentGeneration(pair)
	  rev := e.registry.IsRevoked(pair)
	   pairEvts := e.store.ListByPair(pair)
	  var last *PoisonEvent
	   if len(pairEvts) > 0 {
		   cp := pairEvts[len(pairEvts)-1]
		  last = &cp
	  }
	   return PairStats{
		  Pair: pair,
		   Generation: gen,
		  Revoked:     rev,
		   PoisonCount: len(pairEvts),
		  LastEvent: last,
	  }
  }

// IngestBatch 批量 Ingest：对多个 tick 执行检测流水线。
// 任意 tick 毒价则立即返回该 tick 的错误（fail-fast），
// 并撤销后续未处理 tick 的令牌，确保批次不出现部分成功。

   func (e *PoisonEngine) IngestBatch(ticks []BatchTick) ([]GenToken, error) {
	   if e == nil {
		   return nil, ErrRegistryUnavailable
	  }
	   tokens := make([]GenToken, 0, len(ticks))
	  for i, bt := range ticks {
		  tok, err := e.Ingest(bt.Pair, bt.Price, bt.At, bt.Scope)
		  if err != nil {
			   // 撤销后续未处理的 pair（防止批次部分成功）
			   for _, remaining := range ticks[i+1:] {
				   e.registry.Revoke(normPair(remaining.Pair))
			  }
			  return nil, err
		   }
		  tokens = append(tokens, tok)
	  }
	   return tokens, nil
  }

// BatchTick 批量 Ingest 的单个 tick 输入。

type BatchTick struct {
	Pair  string `json:"pair"`
	Price string `json:"price"`
	At time.Time `json:"at"`
	Scope string `json:"scope"`
}

// GradeLevel 对单个 tick 运行所有检测器，返回最严重的等级。
// 不签发令牌；只用于预检查或监控。

  func (e *PoisonEngine) GradeLevel(pair, tickPrice string, tickAt time.Time) PoisonGrade {
	  if e == nil {
		  return GradeOK
	   }
	  pair = normPair(pair)
	  e.mu.RLock()
	  dets := make([]namedDetector, len(e.detectors))
	   copy(dets, e.detectors)
	   e.mu.RUnlock()
	  worst := GradeOK
	  for _, nd := range dets {
		   detail, err := nd.d.Detect(pair, tickPrice, tickAt)
		   if err != nil {
			  continue
		  }
		  if detail.Grade > worst {
			   worst = detail.Grade
		  }
	   }
	   return worst
   }

// GradeAll 对单个 tick 运行所有检测器，返回所有非 OK 的 GradeDetail 列表。

  func (e *PoisonEngine) GradeAll(pair, tickPrice string, tickAt time.Time) []GradeDetail {
	  if e == nil {
		  return nil
	   }
	  pair = normPair(pair)
	   e.mu.RLock()
	   dets := make([]namedDetector, len(e.detectors))
	  copy(dets, e.detectors)
	   e.mu.RUnlock()
	  var out []GradeDetail
	   for _, nd := range dets {
		  detail, err := nd.d.Detect(pair, tickPrice, tickAt)
		  if err != nil {
			   continue
		  }
		   if detail.Grade.IsPoison() {
			   d := detail
			   d.Extra = strings.TrimSpace(d.Extra + " detector=" + nd.name)
			  out = append(out, d)
		  }
	  }
	   return out
  }
