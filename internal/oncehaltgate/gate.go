package oncehaltgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/outboxonce"
)

// Gate 停机投递闸。
type Gate struct {
	svc   *outboxonce.Service
	clock func() time.Time
}

// NewGate 创建 Gate，要求绑定 outboxonce.Service。
  func NewGate(svc *outboxonce.Service) *Gate {
	   if svc == nil {
		  panic("oncehaltgate: NewGate requires non-nil outboxonce.Service")
	   }
	   return &Gate{
		  svc: svc,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

   func (g *Gate) WithClock(clk func() time.Time) *Gate {
	  if g != nil && clk != nil {
		  g.clock = clk
	   }
	   return g
   }

   // Switch 切换停机状态，返回切换结果（含被吊销的键列表与新世代）。
  func (g *Gate) Switch(ctx context.Context, req SwitchRequest) (SwitchResult, error) {
	  if g == nil || g.svc == nil {
		   return SwitchResult{},
			  ErrUnavailable
	   }
	   _ = ctx
	   lease, revoked := g.svc.SetHaltByActor(req.On, req.Actor)
	  result := SwitchResult{
		   RevokedAt: g.clock(),
		  CaseCount: len(g.svc.Cases()),
	   }
	  if lease != nil {
		  result.Generation = lease.Generation
		   result.State = leaseStateToGateState(lease.State)
	  }
	   for _, dg := range revoked {
		  result.RevokedKeys = append(result.RevokedKeys, dg.Key)
	   }
	  return result, nil
  }

// Deliver 代理 outboxonce.Service.Deliver，附加停机世代校验。
   func (g *Gate) Deliver(ctx context.Context, req DeliverRequest) (DeliverResponse, error) {
	   if g == nil || g.svc == nil {
		  return DeliverResponse{}, ErrUnavailable
	   }
	   got, err := g.svc.Deliver(ctx, req.Key, req.Epoch, req.Token)
	   if err != nil {
		   return DeliverResponse{}, mapOutboxErr(err)
	   }
	  return DeliverResponse{
		   Key: got.Key,
		  ID:  got.ID,
		  Delivered: got.Delivered,
		  Replay:    got.Replay,
		  HaltGen: got.HaltGen,
	   }, nil
  }

// Status 返回闸实时状态（运维快照）。
  func (g *Gate) Status() GateStatus {
	  if g == nil || g.svc == nil {
		   return GateStatus{AsOf: time.Now().UTC()}
	  }
	   view := g.svc.HaltGenView()
	  cases := g.svc.Cases()
	  return GateStatus{
		  State: viewToGateState(view),
		   Generation: view.Generation,
		   CaseCount:  len(cases),
		   AsOf: g.clock(),
	  }
  }

// HaltGenView 返回停机世代只读视图（供外部包消费）。

  func (g *Gate) HaltGenView() outboxonce.HaltGenReadView {
	   if g == nil || g.svc == nil {
		   return outboxonce.HaltGenReadView{}
	  }
	   return g.svc.HaltGenView()
   }

// ExportProof 导出证明摘要。

   func (g *Gate) ExportProof(q ProofQuery) outboxonce.ProofExport {
	  if g == nil || g.svc == nil {
		   return outboxonce.ProofExport{}
	   }
	  proof := g.svc.ExportProof()
	   if q.HaltGen > 0 || q.Kind != "" {
		  filtered := make([]outboxonce.OnceHaltCase, 0, len(proof.Cases))
		   for _, c := range proof.Cases {
			   if q.HaltGen > 0 && c.HaltGen != q.HaltGen {
				  continue
			   }
			   if q.Kind != "" && string(c.Kind) != q.Kind {
				  continue
			  }
			  filtered = append(filtered, c)
		  }
		   lim := q.Limit
		  if lim <= 0 {
			  lim = 50
		   }
		  if len(filtered) > lim {
			  filtered = filtered[:lim]
		   }
		   proof.Cases = filtered
		  proof.Total = len(filtered)
	  }
	  return proof
  }

// leaseStateToGateState 映射 HaltLease.State → GateState。
   func leaseStateToGateState(s outboxonce.HaltState) GateState {
	  switch s {
	  case outboxonce.HaltStateRaised:
		   return GateRaising
	  case outboxonce.HaltStateBlocking:
		   return GateBlocking
	   case outboxonce.HaltStateCleared:
		  return GateOpen
	  default:
		  return GateOpen
	   }
   }

// viewToGateState 从 HaltGenReadView 推导 GateState。
   func viewToGateState(v outboxonce.HaltGenReadView) GateState {
	  if !v.Halted {
		  return GateOpen
	   }
	   return leaseStateToGateState(v.State)
   }

// mapOutboxErr 将 outboxonce 错误映射到 oncehaltgate 错误（保留语义）。
  func mapOutboxErr(err error) error {
	  switch err {
	   case outboxonce.ErrHalt:
		  return ErrHalt
	  case outboxonce.ErrHaltGenCAS:
		   return ErrHaltGenCAS
	  case outboxonce.ErrHaltGenRevoked:
		   return ErrHaltGenRevoked
	   case outboxonce.ErrStaleFenceRevive:
		  return ErrStaleFence
	  case outboxonce.ErrDoubleDeliver, outboxonce.ErrDoubleMark:
		  return ErrDoubleDeliver
	   default:
		  return err
	  }
  }
