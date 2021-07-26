package hotreserve

import (
	"context"
	"github.com/shopspring/decimal"
)

type ExplainLine struct {
	Label string `json:"label"`
	Amount string `json:"amount"`
	Note string `json:"note,omitempty"`
}

type Explain struct {
	Asset string `json:"asset"`
	Version  int64  `json:"version"`
	Point    string `json:"point"`
	Lower    string `json:"lower"`
	Reserved string `json:"reserved"`
	Extra string `json:"extra"`
	Operational string `json:"operational"`
	Blocked     bool   `json:"blocked"`
	BlockReason string `json:"block_reason,omitempty"`
	EventID     string `json:"event_id,omitempty"`
	Lines []ExplainLine `json:"lines"`
}

// Explain 给出挡签使用的运营可用。
// 做什么：运营可用 = 保守下界 − 未兑付预留 − 快照尚未计入的增量。
// 没有新鲜快照时失败，不返回 0。
// 不变量：与 Admit 使用同一 headLocked。

  func (s *Service) Explain(asset string) (Explain, error) {
	  if s == nil {
		   return Explain{}, ErrUnavailable
	   }
	   as, err := normAsset(asset)
	  if err != nil {
		  return Explain{}, err
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   h, err := s.headLocked(as)
	   if err != nil {
		   return Explain{}, err
	  }
	  op := operationalOf(h.Lower, h.Reserved, h.Extra)
	  reason := ""
	   if h.Blocked {
		   if tk := s.drifts[s.driftOpen[as]]; tk != nil {
			  reason = tk.Kind
		  }
	   }
	  ex := Explain{
		  Asset: as, Version: h.Snap.Version, Point: h.Snap.Point, Lower: h.Snap.Lower,
		  Reserved: h.Reserved.String(), Extra: h.Extra.String(), Operational: op.String(),
		   Blocked: h.Blocked, BlockReason: reason, EventID: h.Snap.EventID,
		   Lines: []ExplainLine{
			  {Label: "hot_available", Amount: h.Snap.HotAvail, Note: "热库存可用，只读"},
			  {Label: "inflow_kept", Amount: h.Snap.InflowKept, Note: "入金期望按折价保留"},
			  {Label: "outflow_stressed", Amount: h.Snap.OutflowStressed, Note: "出金期望加压"},
			   {Label: "sweep_hold", Amount: h.Snap.SweepHold, Note: "已登记归集计划占用"},
			  {Label: "in_transit", Amount: h.Snap.InTransit, Note: "快照时的广播跟踪"},
			  {Label: "buffer", Amount: h.Snap.Buffer, Note: "额外缓冲"},
			   {Label: "lower", Amount: h.Snap.Lower, Note: "保守下界"},
			   {Label: "reserved", Amount: h.Reserved.String(), Note: "open 与 earmarked 合计"},
			  {Label: "extra", Amount: h.Extra.String(), Note: "快照之后新增的广播或归集占用"},
			   {Label: "operational", Amount: op.String(), Note: "下界减预留减增量"},
		   },
	   }
	  return ex, nil
   }

// RecordRealization 用实际剩余热可用记下点估偏差。偏差 = 点估 − 实际。

  func (s *Service) RecordRealization(ctx context.Context, asset, actual string) (HotForecast, error) {
	   if s == nil {
		   return HotForecast{}, ErrUnavailable
	   }
	   as, err := normAsset(asset)
	  if err != nil {
		   return HotForecast{}, err
	   }
	  got, err := parseDecAllowNeg(actual)
	   if err != nil {
		  return HotForecast{}, err
	   }
	   if got.IsNegative() {
		  return HotForecast{}, ErrAmount
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   row := s.forecast[as]
	   if row == nil {
		  return HotForecast{}, ErrForecastMissing
	   }
	  point, err := parseDecAllowNeg(row.Point)
	  if err != nil {
		  return HotForecast{}, err
	   }
	  row.Realized = got.String()
	   row.Bias = point.Sub(got).String()
	  if err := s.persistForecast(ctx, row); err != nil {
		   return HotForecast{}, err
	   }
	  noteBias(as, row.Bias)
	  cp := *row
	  return cp, nil
  }

   func operationalOf(lower, reserved, extra decimal.Decimal) decimal.Decimal {
	  return lower.Sub(reserved).Sub(extra)
  }
