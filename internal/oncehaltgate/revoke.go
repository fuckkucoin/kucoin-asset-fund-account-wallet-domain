package oncehaltgate

import (
	"context"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/outboxonce"
)

// RevokeScanner 在途 Deliver 吊销扫描器。
// 在 Halt 激活期间定期扫描，确保所有在途 Deliver 被吊销后才进入 blocking 状态。

type RevokeScanner struct {
	gate *Gate
	interval time.Duration
	clock func() time.Time
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	lastRep RevokeReport
}

// NewRevokeScanner 创建扫描器。interval 建议 200ms–2s；0 使用默认 500ms。

   func NewRevokeScanner(gate *Gate, interval time.Duration) *RevokeScanner {
	  if interval <= 0 {
		   interval = 500 * time.Millisecond
	  }
	  clk := func() time.Time { return time.Now().UTC() }
	  if gate != nil && gate.clock != nil {
		  clk = gate.clock
	   }
	   return &RevokeScanner{
		  gate: gate,
		   interval: interval,
		  clock:    clk,
	  }
  }

// Start 启动后台扫描循环。幂等：已启动则忽略。

   func (rs *RevokeScanner) Start(ctx context.Context) {
	   rs.mu.Lock()
	   defer rs.mu.Unlock()
	  if rs.running {
		  return
	  }
	   ctx2, cancel := context.WithCancel(ctx)
	  rs.cancel = cancel
	  rs.running = true
	   go rs.loop(ctx2)
   }

// Stop 停止后台扫描循环。
   func (rs *RevokeScanner) Stop() {
	  rs.mu.Lock()
	  defer rs.mu.Unlock()
	   if rs.cancel != nil {
		  rs.cancel()
	   }
	  rs.running = false
  }

// LastReport 返回最近一次扫描报告。

  func (rs *RevokeScanner) LastReport() RevokeReport {
	  rs.mu.Lock()
	  defer rs.mu.Unlock()
	  return rs.lastRep
  }

   func (rs *RevokeScanner) loop(ctx context.Context) {
	  ticker := time.NewTicker(rs.interval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   rep := rs.ScanOnce(ctx)
			  rs.mu.Lock()
			   rs.lastRep = rep
			  rs.mu.Unlock()
		  }
	   }
   }

// ScanOnce 执行一次在途 Deliver 吊销扫描。
// 若 Gate 不处于 Halt 状态，直接返回（跳过扫描）。
   func (rs *RevokeScanner) ScanOnce(ctx context.Context) RevokeReport {
	   rep := RevokeReport{ScannedAt: rs.clock()}
	   if rs.gate == nil || rs.gate.svc == nil {
		   return rep
	  }
	  svc := rs.gate.svc
	  view := svc.HaltGenView()
	  rep.Generation = view.Generation
	   if !view.Halted {
		  return rep
	   }
	   // 统计在途与被吊销的 Deliver 数量（通过 hgm 暴露的计数器）。
	  // haltGenMgr.SetHalt 在切换时已内联吊销；此处仅做后置确认。
	   rep.InFlight = svc.InFlightCount()
	  rep.Revoked = svc.RevokedFenceCount()
	   return rep
  }

// DoubleMark patrol 双 Mark 巡检 Worker。
type DoubleMarkPatrol struct {
	gate     *Gate
	interval time.Duration
	clock func() time.Time
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	lastRep PatrolReport
}

// NewDoubleMarkPatrol 创建双 Mark 巡检器。

   func NewDoubleMarkPatrol(gate *Gate, interval time.Duration) *DoubleMarkPatrol {
	   if interval <= 0 {
		  interval = 2 * time.Second
	   }
	   clk := func() time.Time { return time.Now().UTC() }
	  if gate != nil && gate.clock != nil {
		   clk = gate.clock
	  }
	  return &DoubleMarkPatrol{
		   gate:     gate,
		  interval: interval,
		   clock: clk,
	  }
  }

// Start 启动巡检循环。幂等。

   func (p *DoubleMarkPatrol) Start(ctx context.Context) {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  if p.running {
		   return
	   }
	   ctx2, cancel := context.WithCancel(ctx)
	  p.cancel = cancel
	   p.running = true
	   go p.loop(ctx2)
   }

// Stop 停止巡检循环。
   func (p *DoubleMarkPatrol) Stop() {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   if p.cancel != nil {
		   p.cancel()
	   }
	   p.running = false
   }

// LastReport 返回最近一次巡检报告。
   func (p *DoubleMarkPatrol) LastReport() PatrolReport {
	  p.mu.Lock()
	   defer p.mu.Unlock()
	  return p.lastRep
  }

  func (p *DoubleMarkPatrol) loop(ctx context.Context) {
	  ticker := time.NewTicker(p.interval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   rep := p.PatrolOnce(ctx)
			   p.mu.Lock()
			  p.lastRep = rep
			  p.mu.Unlock()
		  }
	  }
   }

// PatrolOnce 执行一次双 Mark 巡检。
// 扫描 outboxonce 的 Cases，统计 double_mark / double_deliver 数量。

  func (p *DoubleMarkPatrol) PatrolOnce(ctx context.Context) PatrolReport {
	   rep := PatrolReport{ScannedAt: p.clock()}
	  if p.gate == nil || p.gate.svc == nil {
		  return rep
	   }
	   _ = ctx
	   cases := p.gate.svc.Cases()
	  rep.Checked = len(cases)
	   for _, c := range cases {
		   switch c.Kind {
		  case outboxonce.CaseKindDoubleMark,
			   outboxonce.CaseKindDoubleDeliver:
			  rep.DoubleMarks++
		   }
	   }
	  return rep
  }
