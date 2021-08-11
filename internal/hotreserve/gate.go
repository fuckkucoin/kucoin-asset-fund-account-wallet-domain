// 功能：hotreserve/gate.go
// 作者：Cason
// 日期：2021.07.25

package hotreserve

import (
	"context"
	"strings"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type head struct {
	Snap     HotForecast
	Lower    decimal.Decimal
	Reserved decimal.Decimal
	Extra    decimal.Decimal
	Blocked  bool
}

// headLocked 计算当前可挡签的运营头寸。
// 做什么：下界 − 未兑付预留 − 快照之外新增的广播与归集占用。
// 快照里已经扣过的在途不重复扣；观察值低于跟踪时不把差额加回头寸。
// 不变量：已兑付金额不在 Reserved 里。
   func (s *Service) headLocked(asset string) (head, error) {
	   snap, ok := s.forecast[asset]
	  if !ok || snap == nil {
		   return head{}, ErrForecastMissing
	   }
	  if s.now().Sub(snap.AsOf) > s.pol.ForecastMaxAge {
		  return head{}, ErrForecastStale
	   }
	  lower, err := parseDecAllowNeg(snap.Lower)
	   if err != nil {
		  return head{}, err
	  }
	   transit, err := parseNonNeg(snap.InTransit)
	  if err != nil {
		   return head{}, err
	  }
	   sweep, err := parseNonNeg(snap.SweepHold)
	   if err != nil {
		   return head{}, err
	  }
	  reserved := s.sumState(asset, StateOpen).Add(s.sumState(asset, StateEarmarked))
	  extraBc := posDelta(s.sumTrack(asset, TrackBroadcast), transit)
	  extraSw := posDelta(s.planSum(asset), sweep)
	   h := head{
		  Snap:  *snap,
		   Lower: lower, Reserved: reserved, Extra: extraBc.Add(extraSw),
		   Blocked: s.driftOpen[asset] != "",
	   }
	  return h, nil
  }

   func parseDecAllowNeg(s string) (decimal.Decimal, error) {
	   s = strings.TrimSpace(s)
	  if s == "" || strings.ContainsAny(s, "eE") {
		  return decimal.Zero, ErrAmount
	  }
	   d, err := decimal.NewFromString(s)
	  if err != nil {
		  return decimal.Zero, ErrAmount
	   }
	  return d, nil
   }

   func (s *Service) planSum(asset string) decimal.Decimal {
	   var vals []decimal.Decimal
	  for _, p := range s.plans[asset] {
		   if d, err := parsePos(p.Amount); err == nil {
			   vals = append(vals, d)
		   }
	  }
	  return sumDec(vals)
   }

// Admit 出金签名或提交前的只读闸。
// 做什么：保守下界 − 已预留 − 新意图 < 0 则拒绝。
// 一致差异、快照缺失或过期、库存不可读，一律失败即关。
// 不变量：已为本单预留或已兑付的金额不再当成新意图重复扣减。
   func (s *Service) Admit(ctx context.Context, phase, asset, amount, movementID string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   as, err := normAsset(asset)
	  if err != nil {
		  return err
	   }
	  amt, err := parsePos(amount)
	   if err != nil {
		  s.noteGate(phase, as, amount, movementID, "deny", "amount")
		   return err
	   }
	  if _, err := s.readSurface(ctx, as); err != nil {
		  s.noteGate(phase, as, amt.String(), movementID, "deny", "unread")
		   return err
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if err := s.guardLocked(); err != nil {
		   s.noteGateLocked(phase, as, amt.String(), movementID, "deny", "degraded")
		   return err
	   }
	   return s.admitLocked(phase, as, amt, movementID, true)
  }

   func (s *Service) admitLocked(phase, asset string, amount decimal.Decimal, movementID string, note bool) error {
	  if s.driftOpen[asset] != "" {
		   if note {
			  s.noteGateLocked(phase, asset, amount.String(), movementID, "deny", "drift")
		  } else {
			  s.noteGateLocked(phase, asset, amount.String(), movementID, "deny", "drift")
		  }
		  return ErrAlignDrift
	  }
	  h, err := s.headLocked(asset)
	  if err != nil {
		  reason := "missing"
		   if err == ErrForecastStale {
			   reason = "stale"
		  }
		  s.noteGateLocked(phase, asset, amount.String(), movementID, "deny", reason)
		   return err
	   }
	  if h.Blocked {
		   s.noteGateLocked(phase, asset, amount.String(), movementID, "deny", "drift")
		  return ErrAlignDrift
	  }
	  intent := amount
	   covered, consumed := s.coveredLocked(asset, movementID)
	  if consumed {
		  intent = decimal.Zero
	  } else if covered.GreaterThanOrEqual(amount) {
		  intent = decimal.Zero
	   } else if covered.IsPositive() {
		   intent = amount.Sub(covered)
	   }
	   if Short(h.Lower, h.Reserved, h.Extra, intent) {
		   s.noteGateLocked(phase, asset, amount.String(), movementID, "deny", "shortfall")
		   return ErrShortfall
	  }
	   if note {
		  s.noteGateLocked(phase, asset, amount.String(), movementID, "allow", "")
	   }
	   return nil
   }

   func (s *Service) noteGate(phase, asset, amount, movementID, result, reason string) {
	   if s == nil {
		  return
	   }
	  s.mu.Lock()
	   s.noteGateLocked(phase, asset, amount, movementID, result, reason)
	  s.mu.Unlock()
   }

   func (s *Service) noteGateLocked(phase, asset, amount, movementID, result, reason string) {
	  s.gates = append(s.gates, GateLog{
		  ID: uuid.NewString(), Phase: phase, Asset: asset, Amount: amount,
		  MovementID: movementID,
		   Result:     result, Reason: reason, At: s.now(),
	  })
	   if len(s.gates) > 500 {
		  s.gates = s.gates[len(s.gates)-500:]
	  }
	  if result == "deny" {
		   noteBlock(phase, reason)
	   }
   }

// GateLogs 最近挡签记录，供巡检核对拒绝原因。
   func (s *Service) GateLogs(asset string) []GateLog {
	   if s == nil {
		  return nil
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]GateLog, 0)
	  for _, g := range s.gates {
		  if asset == "" || g.Asset == asset {
			  out = append(out, g)
		   }
	   }
	  return out
   }
