package wdtriplegate

import (
	"strings"
	"time"
	"github.com/google/uuid"
)

// Binder 广播前世代复验绑定器。
type Binder struct {
	store  Store
	engine *Engine
	detector *Detector
	clock func() time.Time
}

// NewBinder 构造绑定器。
  func NewBinder(store Store, engine *Engine, detector *Detector) *Binder {
	  if store == nil {
		   store = NewMemStore()
	   }
	   if engine == nil {
		   engine = NewEngine(store)
	   }
	   if detector == nil {
		   detector = NewDetector(store)
	   }
	   return &Binder{
		   store: store,
		   engine:   engine,
		  detector: detector,
		   clock:    func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟（用）。

  func (b *Binder) WithClock(fn func() time.Time) *Binder {
	  if fn != nil {
		   b.clock = fn
		   b.engine.WithClock(fn)
		   b.detector.WithClock(fn)
	  }
	   return b
  }

   // Bind 广播前世代复验主路径。
  // 流程：
   // 1. 查询 triple_lease_id 对应的 TripleLease。
   // 2. 校验租约未过期、未终态、state=Issued。
  // 3. req.CurrentListGen == lease.ListGen。
  // 4. req.CurrentPolicyVer == lease.PolicyVer。
   // 5. req.CurrentKYTPin == "allow"（KYT 钉未撤销）。
  // 6. 全部通过 → BroadcastAllowed=true → BindLease(leaseID) → AppendBind。
   // 7. 任一漂移 → DriftDetected=true → Detector.Prove → BlockLease → AppendBind(BroadcastAllowed=false)。

   func (b *Binder) Bind(req BindReq) (BindResult, error) {
	  if b == nil {
		   return BindResult{}, ErrUnavailable
	  }
	   movementID := strings.TrimSpace(req.MovementID)
	  leaseID := strings.TrimSpace(req.TripleLeaseID)
	   if movementID == "" {
		  return BindResult{}, ErrMovementRequired
	  }
	   if leaseID == "" {
		  return BindResult{}, ErrLeaseIDRequired
	  }
	  now := b.clock()
	   // 幂等：已绑定则直接返回
	  if existing, err := b.store.GetBindByMovement(movementID); err == nil {
		  return BindResult{
			   BindID:           existing.BindID,
			  BroadcastAllowed: existing.BroadcastAllowed,
			  DriftDetected:    existing.DriftDetected,
			  RefuseReason: existing.RefuseReason,
		   }, nil
	   }
	  lease, err := b.engine.GetLease(leaseID)
	  if err != nil {
		  return BindResult{}, err
	   }
	  // 检查 TTL
	   if now.After(lease.ExpiresAt) {
		   _, _ = b.engine.ExpireLease(leaseID)
		  return BindResult{BroadcastAllowed: false, RefuseReason: ErrLeaseExpired.Error()}, ErrLeaseExpired
	   }
	  if LeaseIsTerminal(lease.State) {
		   if lease.State == LeaseBlocked {
			   return BindResult{BroadcastAllowed: false, RefuseReason: ErrLeaseBlocked.Error()}, ErrLeaseBlocked
		   }
		  return BindResult{BroadcastAllowed: false, RefuseReason: ErrLeaseTerminal.Error()}, ErrLeaseTerminal
	  }
	  // 世代
	  driftKind, driftFound := b.detectDrift(lease, req)
	  bindID := uuid.NewString()
	  if driftFound {
		   // 漂移路径：产出 DriftCase → BlockLease → 记录绑定失败
		  driftCase, driftErr := b.detector.Prove(DetectReq{
			  MovementID:    movementID,
			  TripleLeaseID: leaseID,
			  DriftKind: driftKind,
			   LeaseListGen:   lease.ListGen,
			   CurrentListGen: req.CurrentListGen,
			  LeasePolicyVer:   lease.PolicyVer,
			   CurrentPolicyVer: req.CurrentPolicyVer,
			   LeaseKYTPin: lease.KYTPin,
			  CurrentKYTPin: req.CurrentKYTPin,
		  })
		   var driftCaseID string
		  if driftErr == nil && driftCase != nil {
			  driftCaseID = driftCase.DriftCaseID
		   }
		   reason := ErrDriftDetected.Error()
		  _,
			   _ = b.engine.BlockLease(leaseID, driftCaseID, reason)
		  bind := PreflightBind{
			  BindID:        bindID,
			  MovementID:    movementID,
			   TripleLeaseID: leaseID,
			  ListGenAtBind: req.CurrentListGen,
			   PolicyVerAtBind:  req.CurrentPolicyVer,
			  BroadcastAllowed: false,
			   RefuseReason:     reason,
			   DriftDetected: true,
			   BoundAt:       now,
			   CreatedAt:     now,
		   }
		   _ = b.store.AppendBind(bind)
		   MetricBindRefused.Inc()
		  return BindResult{
			  BindID:           bindID,
			   BroadcastAllowed: false,
			   DriftDetected: true,
			   DriftCaseID:   driftCaseID,
			  RefuseReason:  reason,
		   }, ErrDriftDetected
	   }
	   // 无漂移路径：BindLease → AppendBind
	   if _, err := b.engine.BindLease(leaseID); err != nil {
		  return BindResult{BroadcastAllowed: false, RefuseReason: err.Error()}, err
	   }
	   bind := PreflightBind{
		   BindID:        bindID,
		  MovementID:    movementID,
		  TripleLeaseID: leaseID,
		   ListGenAtBind: req.CurrentListGen,
		   PolicyVerAtBind:  req.CurrentPolicyVer,
		  BroadcastAllowed: true,
		  BoundAt:          now,
		   CreatedAt: now,
	  }
	  _ = b.store.AppendBind(bind)
	  MetricBindAllowed.Inc()
	   return BindResult{
		  BindID:           bindID,
		  BroadcastAllowed: true,
	   }, nil
   }

// RequireBroadcastAllowed 广播口强制校验：绑定未完成或漂移均拒绝。
// 出金广播路径必须在每次广播前调用此方法。
   func (b *Binder) RequireBroadcastAllowed(movementID string) error {
	  if b == nil {
		   return ErrUnavailable
	   }
	  bind, err := b.store.GetBindByMovement(strings.TrimSpace(movementID))
	  if err != nil {
		   return ErrBroadcastBlocked
	   }
	   if !bind.BroadcastAllowed {
		   return ErrBroadcastBlocked
	   }
	  return nil
  }

// GetBind 查询预检绑定记录。

  func (b *Binder) GetBind(movementID string) (*PreflightBind, error) {
	  if b == nil {
		  return nil, ErrUnavailable
	   }
	   return b.store.GetBindByMovement(strings.TrimSpace(movementID))
   }

// detectDrift 租约快照与当前读数，返回首个漂移类型。
  func (b *Binder) detectDrift(lease *TripleLease, req BindReq) (DriftKind, bool) {
	  if lease.ListGen != req.CurrentListGen {
		   return DriftListGen, true
	  }
	   if lease.PolicyVer != req.CurrentPolicyVer {
		   return DriftPolicyVer, true
	   }
	  if strings.TrimSpace(req.CurrentKYTPin) != "allow" {
		  return DriftKYTPin, true
	  }
	   return "", false
  }

// VerifyLeaseForBroadcast 仅校验不写入（幂等查询路径）。
// 返回 nil 表示可广播；否则返回拒绝原因。

   func VerifyLeaseForBroadcast(store Store, movementID string, now time.Time) error {
	   if store == nil {
		   return ErrUnavailable
	   }
	   bind, err := store.GetBindByMovement(strings.TrimSpace(movementID))
	   if err != nil {
		   return ErrBroadcastBlocked
	  }
	   if !bind.BroadcastAllowed {
		  return ErrBroadcastBlocked
	   }
	  // 额外：检查租约是否仍有效
	  lease, err := store.GetLease(bind.TripleLeaseID)
	  if err != nil {
		   return ErrBroadcastBlocked
	  }
	  if LeaseIsTerminal(lease.State) && lease.State != LeaseConsumed {
		  return ErrBroadcastBlocked
	   }
	   if lease.State == LeaseBlocked {
		   return ErrLeaseBlocked
	  }
	   return nil
  }
