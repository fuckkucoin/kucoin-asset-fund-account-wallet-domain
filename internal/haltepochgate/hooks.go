package haltepochgate

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

// GateReader 只读消费 custodyhalt.Gate 快照。

type GateReader interface {
	Snapshot(ctx context.Context) (custodyhalt.Flag, error)
	Halted(ctx context.Context) (bool,
		error)
}

// LaneReader 只读消费 Lane 闸位。
type LaneReader interface {
	LaneOf(ctx context.Context, lane custodyhalt.Lane, chain, asset string) custodyhalt.LaneStatus
	CheckLane(ctx context.Context, lane custodyhalt.Lane, chain, asset string) error
}

// HistoryReader 只读消费熔断审计环。
type HistoryReader interface {
	Recent(limit int) []custodyhalt.Event
}

// AuditReader 只读消费持久审计。

type AuditReader interface {
	Query(ctx context.Context,
		q custodyhalt.AuditQuery) []custodyhalt.Event
}

// QuorumReader Clear 法定人数只读视图。未装配则本包自持投票仓。
type QuorumReader interface {
	AllowClear(ctx context.Context, scope ScopeKey, actor string) error
	Threshold() int
}

// NopGate 缺省只读闸：视为未熔断。读失败由调用方 fail-closed。
type NopGate struct{}

  func (NopGate) Snapshot(context.Context) (custodyhalt.Flag, error) { return custodyhalt.Flag{}, nil }
   func (NopGate) Halted(context.Context) (bool, error)               { return false, nil }

// NopLane 缺省 Lane：不挡。
type NopLane struct{}

   func (NopLane) LaneOf(_ context.Context, lane custodyhalt.Lane, _, _ string) custodyhalt.LaneStatus {
	  return custodyhalt.LaneStatus{Lane: lane}
   }

  func (NopLane) CheckLane(context.Context, custodyhalt.Lane, string, string) error { return nil }

// NopHistory 空审计环。

type NopHistory struct{}

   func (NopHistory) Recent(int) []custodyhalt.Event { return nil }

// NopAudit 空审计仓。
type NopAudit struct{}

   func (NopAudit) Query(context.Context, custodyhalt.AuditQuery) []custodyhalt.Event { return nil }

// FailGate 读失败闸，供 fail-closed 夹具。

type FailGate struct{ Err error }

  func (f FailGate) Snapshot(context.Context) (custodyhalt.Flag, error) {
	  if f.Err == nil {
		  f.Err = errors.New("gate snapshot failed")
	   }
	   return custodyhalt.Flag{}, f.Err
   }
  func (f FailGate) Halted(context.Context) (bool,
	   error) {
	  return false, f.Err
   }

// GateAdapter 把 *custodyhalt.Gate 适配为只读 Gate+Lane。

type GateAdapter struct {
	Gate *custodyhalt.Gate
}

   func (a GateAdapter) Snapshot(ctx context.Context) (custodyhalt.Flag, error) {
	  if a.Gate == nil {
		   return custodyhalt.Flag{}, ErrReadGateFailed
	  }
	   return a.Gate.Snapshot(ctx)
  }

  func (a GateAdapter) Halted(ctx context.Context) (bool, error) {
	   if a.Gate == nil {
		  return false, ErrReadGateFailed
	   }
	   return a.Gate.Halted(ctx)
  }

  func (a GateAdapter) LaneOf(ctx context.Context,
	  lane custodyhalt.Lane, chain, asset string) custodyhalt.LaneStatus {
	  if a.Gate == nil {
		   return custodyhalt.LaneStatus{Lane: lane, Blocked: true, FailClosed: true, Reason: ErrReadGateFailed.Error()}
	   }
	  return a.Gate.LaneOf(ctx, lane, chain, asset)
   }

  func (a GateAdapter) CheckLane(ctx context.Context, lane custodyhalt.Lane, chain, asset string) error {
	   if a.Gate == nil {
		   return ErrReadGateFailed
	   }
	   return a.Gate.CheckLane(ctx, lane, chain, asset)
   }

  func MapLane(l FundLane) custodyhalt.Lane {
	   switch l {
	   case LaneWithdraw:
		  return custodyhalt.LaneWithdraw
	  case LaneSweep:
		  return custodyhalt.LaneSweep
	  default:
		   return custodyhalt.LaneDeposit
	   }
   }

   func ScopeFromHalt(scope, chain, asset string) ScopeKey {
	   return NormalizeScope(scope,
		   chain, asset)
  }

  func ActorOf(s string) string { return strings.TrimSpace(s) }
