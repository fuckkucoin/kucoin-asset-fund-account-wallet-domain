// - Gate 是唯一对外入口；外部包通过 Gate 调用，不直接访问 Engine/Binder/Detector。
// - IssueLease：签发三联租约（Preflight 成功后调用）。
// - Bind：广播前世代复验绑定（出金广播前强制调用）。
// - RequireBroadcastAllowed：广播口校验入口（最终防线）。
// - ConsumeLease：广播成功后消费租约。
// - ConfirmDriftClose：四眼确认关闭漂移案件。
// - LeaseStatus：查询租约完整状态。
// 不变量：
// - Gate 方法均幂等；重复调用不产生副作用。
// - Gate 不改写 withdraw 广播 FSM；通过 RequireBroadcastAllowed 前置挡单。

// Author: Blaise
// Date: 2021.12.29

package wdtriplegate

import (
	"strings"
	"time"
)

// Gate 三联闸统一入口。

type Gate struct {
	store  Store
	engine *Engine
	binder *Binder
	detector *Detector
	clock func() time.Time
}

// GateConfig 闸门配置。

type GateConfig struct {
	Store Store
}

// NewGate 构造三联闸门。

   func NewGate(cfg GateConfig) *Gate {
	   store := cfg.Store
	  if store == nil {
		  store = NewMemStore()
	   }
	  engine := NewEngine(store)
	  detector := NewDetector(store)
	  binder := NewBinder(store, engine, detector)
	  return &Gate{
		  store: store,
		  engine: engine,
		   binder:   binder,
		  detector: detector,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟（用）。
  func (g *Gate) WithClock(fn func() time.Time) *Gate {
	   if fn != nil {
		  g.clock = fn
		   g.engine.WithClock(fn)
		  g.binder.WithClock(fn)
		   g.detector.WithClock(fn)
	   }
	   return g
  }

   // IssueLease Preflight/CheckTriple 成功后签发三联租约。
   func (g *Gate) IssueLease(req IssueLeaseReq) (*TripleLease, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	   }
	  return g.engine.IssueLease(req)
  }

// Bind 广播前世代复验绑定主路径。
// 返回 BindResult.BroadcastAllowed=false 时严禁广播。
  func (g *Gate) Bind(req BindReq) (BindResult, error) {
	   if g == nil {
		  return BindResult{}, ErrUnavailable
	   }
	   return g.binder.Bind(req)
   }

// RequireBroadcastAllowed 广播口最终校验。
// 返回 nil 表示可广播；否则返回拒绝原因。
  func (g *Gate) RequireBroadcastAllowed(movementID string) error {
	   if g == nil {
		   return ErrUnavailable
	  }
	   return g.binder.RequireBroadcastAllowed(strings.TrimSpace(movementID))
  }

// ConsumeLease 广播成功后消费租约（Bound→Consumed）。
  func (g *Gate) ConsumeLease(leaseID string) (*TripleLease, error) {
	  if g == nil {
		  return nil, ErrUnavailable
	   }
	  return g.engine.ConsumeLease(strings.TrimSpace(leaseID))
   }

// ConfirmDriftClose 四眼确认关闭漂移案件。

   func (g *Gate) ConfirmDriftClose(req ConfirmDriftCaseReq) (*TripleDriftCase, error) {
	   if g == nil {
		  return nil, ErrUnavailable
	  }
	  return g.detector.ConfirmClose(req)
   }

// AttachDriftProof 将证明摘要附上（Open→Proven）。

  func (g *Gate) AttachDriftProof(driftCaseID, proofChecksum string) (*TripleDriftCase, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	  }
	   return g.detector.AttachProof(driftCaseID, proofChecksum)
   }

// LeaseStatus 查询 movement_id 的完整租约状态（租约+绑定+漂移案件）。
   func (g *Gate) LeaseStatus(movementID string) (*LeaseStatusResp, error) {
	  if g == nil {
		  return nil, ErrUnavailable
	   }
	   movementID = strings.TrimSpace(movementID)
	   lease, err := g.store.GetActiveLeaseByMovement(movementID)
	  if err != nil && err != ErrLeaseNotFound {
		   return nil, err
	   }
	   // 如果没有活跃租约，尝试历史记录
	   if lease == nil {
		   leases, listErr := g.store.ListLeasesByMovement(movementID)
		   if listErr == nil && len(leases) > 0 {
			  lease = leases[0]
		   }
	  }
	   resp := &LeaseStatusResp{Lease: lease}
	  bind, bindErr := g.store.GetBindByMovement(movementID)
	  if bindErr == nil {
		   resp.Bind = bind
	   }
	  driftCases, _ := g.store.ListDriftCasesByMovement(movementID)
	   resp.DriftCases = driftCases
	   return resp, nil
  }

// ListOpenDriftCases 列出所有 Open 漂移案件（运维巡检）。

   func (g *Gate) ListOpenDriftCases(limit int) ([]*TripleDriftCase, error) {
	  if g == nil {
		  return nil, ErrUnavailable
	   }
	  return g.detector.ListOpenCases(limit)
   }

// GetDriftCase 按 ID 查询漂移案件。
  func (g *Gate) GetDriftCase(driftCaseID string) (*TripleDriftCase, error) {
	   if g == nil {
		   return nil, ErrUnavailable
	  }
	  return g.store.GetDriftCase(strings.TrimSpace(driftCaseID))
  }

// BlockLeaseByDrift 外部触发租约挡写（供 wdguard hook 调用）。
   func (g *Gate) BlockLeaseByDrift(leaseID, driftCaseID, reason string) (*TripleLease, error) {
	   if g == nil {
		   return nil, ErrUnavailable
	   }
	   return g.engine.BlockLease(
		  strings.TrimSpace(leaseID),
		   strings.TrimSpace(driftCaseID),
		   strings.TrimSpace(reason),
	  )
   }

// ForceExpire 强制将租约标记为 Expired（运维重检触发）。
   func (g *Gate) ForceExpire(leaseID string) (*TripleLease, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	   }
	  return g.engine.ExpireLease(strings.TrimSpace(leaseID))
  }

// Store 返回底层 Store（Worker 使用）。
  func (g *Gate) Store() Store { return g.store }

// forceClockNow 辅助：返回当前 gate 时钟值。
  func (g *Gate) now() time.Time {
	   if g.clock != nil {
		  return g.clock()
	   }
	   return time.Now().UTC()
  }
