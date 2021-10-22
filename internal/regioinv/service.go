package regioinv

import (
	"context"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Service 是库存域业务层。Store 注入。
type Service struct {
	st    Store
	clock func() time.Time
}

// New 构造 Service，Store 不能为 nil。
   func New(st Store) *Service {
	   return &Service{st: st, clock: func() time.Time { return time.Now().UTC() }}
  }

   func (svc *Service) SetClock(fn func() time.Time) {
	  if fn != nil {
		  svc.clock = fn
	  }
  }

  func (svc *Service) now() time.Time {
	   return svc.clock()
  }

// RegisterRegion 登记区域，幂等：code 已存在时返回已有行。
   func (svc *Service) RegisterRegion(ctx context.Context, req RegisterReq) (Region, error) {
	   code, cat, err := BindCatalog(req.Code, req.CatalogCode)
	  if err != nil {
		   return Region{}, err
	  }
	   if actor := strings.TrimSpace(req.Actor); actor == "" {
		  return Region{}, ErrActor
	  }
	   existing, err := svc.st.FindRegion(ctx, code)
	  if err == nil {
		  return existing, nil
	  }
	  if !errors.Is(err, ErrNotFound) {
		  return Region{}, err
	  }
	  now := svc.now()
	  r := Region{
		  ID:          newID("reg"),
		  Code:        code,
		  CatalogCode: cat,
		  Name:        strings.TrimSpace(req.Name),
		   Role:        DefaultRole(cat, req.Role),
		   Firewall:    FWOpen,
		   Ready:       true,
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := svc.st.SaveRegion(ctx, r); err != nil {
		  return Region{}, err
	  }
	   return r, nil
   }

// GetRegion 按代码读取。
   func (svc *Service) GetRegion(ctx context.Context, code string) (Region, error) {
	  return svc.st.FindRegion(ctx, code)
  }

// ListRegions 全部区。
  func (svc *Service) ListRegions(ctx context.Context) ([]Region, error) {
	  return svc.st.ListRegions(ctx)
   }

// ObserveInv 接收一条来源读数，汇聚后写入库存快照。
   func (svc *Service) ObserveInv(ctx context.Context, req ObserveReq) (RegionInv, error) {
	   regionID := req.RegionID
	  if regionID == "" {
		   r, err := svc.st.FindRegion(ctx, req.RegionCode)
		  if err != nil {
			  return RegionInv{}, err
		  }
		   regionID = r.ID
	  }
	   tier, err := NormTier(req.Tier)
	   if err != nil {
		  return RegionInv{}, err
	   }
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   chain := strings.ToLower(strings.TrimSpace(req.Chain))
	  avail, err := money.ParseBook(req.Available)
	  if err != nil {
		   return RegionInv{}, ErrInvalid
	  }
	   froz, err := money.ParseBook(req.Frozen)
	  if err != nil {
		  return RegionInv{}, ErrInvalid
	   }
	   now := svc.now()
	   // 取现有 seq
	  var seq int64 = 1
	  if old, err := svc.st.FindInv(ctx, regionID, tier, asset, chain); err == nil {
		  seq = old.Seq + 1
	   }
	   obs := Observation{
		  ID: newID("ob"),
		  RegionID: regionID,
		  Tier:     tier,
		  Asset:    asset,
		  Chain:    chain,
		   Source:    strings.TrimSpace(req.Source),
		   Available: avail.String(),
		  Frozen:    froz.String(),
		  Seq: seq,
		  ObservedAt: now,
	   }
	   if err := svc.st.SaveObs(ctx, obs); err != nil {
		   return RegionInv{}, err
	  }
	   return Aggregate(ctx, svc.st, regionID, tier, asset, chain, now)
  }

// Snapshot 返回某区的全部库存快照。
   func (svc *Service) Snapshot(ctx context.Context, regionCode string) ([]RegionInv, error) {
	  r, err := svc.st.FindRegion(ctx, regionCode)
	  if err != nil {
		   return nil, err
	  }
	   return svc.st.ListInv(ctx, r.ID)
  }

// SetMark 写入或更新水位。Low ≤ Target ≤ High。
   func (svc *Service) SetMark(ctx context.Context, req MarkReq) (Watermark, error) {
	   if strings.TrimSpace(req.Actor) == "" {
		  return Watermark{}, ErrActor
	  }
	  regionID := req.RegionID
	  if regionID == "" {
		  r, err := svc.st.FindRegion(ctx, req.RegionCode)
		  if err != nil {
			   return Watermark{}, err
		   }
		   regionID = r.ID
	  }
	   tier, err := NormTier(req.Tier)
	   if err != nil {
		   return Watermark{}, err
	   }
	   low, err := money.ParseBook(req.Low)
	  if err != nil {
		  return Watermark{}, ErrInvalid
	   }
	  tgt, err := money.ParseBook(req.Target)
	  if err != nil {
		   return Watermark{}, ErrInvalid
	   }
	  high, err := money.ParseBook(req.High)
	  if err != nil {
		   return Watermark{}, ErrInvalid
	   }
	  if low.Cmp(tgt) > 0 || tgt.Cmp(high) > 0 {
		  return Watermark{}, ErrInvalid
	   }
	  enabled := true
	  if req.Enabled != nil {
		  enabled = *req.Enabled
	  }
	   now := svc.now()
	   existing, err := svc.st.FindMark(ctx, regionID, tier, req.Asset, req.Chain)
	  wm := Watermark{
		  RegionID: regionID,
		  Tier:   tier,
		   Asset:  strings.ToUpper(req.Asset),
		   Chain:  strings.ToLower(req.Chain),
		  Low:    low.String(),
		  Target: tgt.String(),
		   High:   high.String(),
		   MinLeg: "0",
		  MaxLeg: "0",
		  DailyCap: "0",
		   Used:        "0",
		   WindowStart: req.WindowStart,
		  WindowEnd:   req.WindowEnd,
		   MaxLegs:     req.MaxLegs,
		   Enabled:     enabled,
		  Actor: req.Actor,
		  UpdatedAt: now,
	   }
	   if err == nil {
		  wm.ID = existing.ID
		   wm.Used = existing.Used
		  wm.UsedDay = existing.UsedDay
	   } else {
		   wm.ID = newID("wm")
	  }
	  if req.MinLeg != "" {
		  if ml, err := money.ParseBook(req.MinLeg); err == nil {
			   wm.MinLeg = ml.String()
		   }
	  }
	   if req.MaxLeg != "" {
		   if ml, err := money.ParseBook(req.MaxLeg); err == nil {
			   wm.MaxLeg = ml.String()
		   }
	  }
	   if req.DailyCap != "" {
		  if cap, err := money.ParseBook(req.DailyCap); err == nil {
			   wm.DailyCap = cap.String()
		  }
	  }
	  if wm.MaxLegs == 0 {
		   wm.MaxLegs = 8
	   }
	  if err := svc.st.SaveMark(ctx, wm); err != nil {
		  return Watermark{}, err
	   }
	  return wm, nil
  }

// SetPolicy 写入隔离策略，区不存在时拒绝。
  func (svc *Service) SetPolicy(ctx context.Context, req PolicyReq) (IsoPolicy, error) {
	   if strings.TrimSpace(req.Actor) == "" {
		   return IsoPolicy{}, ErrActor
	   }
	   regionID := req.RegionID
	   if regionID == "" {
		  r, err := svc.st.FindRegion(ctx, req.RegionCode)
		  if err != nil {
			   return IsoPolicy{}, err
		   }
		   regionID = r.ID
	   }
	   existing,
		   _ := svc.st.FindPolicy(ctx, regionID)
	   pol := existing
	   if pol.RegionID == "" {
		  pol.ID = newID("pol")
		   pol.RegionID = regionID
		   pol.BlockCrossHot = true
		   pol.BlockWrongRebal = true
		  pol.FailTo = FWBlocked
	  }
	   if req.BlockCrossHot != nil {
		  pol.BlockCrossHot = *req.BlockCrossHot
	  }
	  if req.BlockWrongRebal != nil {
		  pol.BlockWrongRebal = *req.BlockWrongRebal
	   }
	   if req.DriftBPS > 0 {
		   pol.DriftBPS = req.DriftBPS
	  }
	  if req.FailTo != "" {
		  if fw, err := NormFirewall(req.FailTo); err == nil {
			   pol.FailTo = fw
		  }
	   }
	  pol.Actor = req.Actor
	  pol.UpdatedAt = svc.now()
	  if err := svc.st.SavePolicy(ctx, pol); err != nil {
		  return IsoPolicy{}, err
	  }
	  return pol, nil
   }

// WatermarkBoard 生成全部区的水位看板。
   func (svc *Service) WatermarkBoard(ctx context.Context) (Board, error) {
	  regions, err := svc.st.ListRegions(ctx)
	  if err != nil {
		  return Board{}, err
	  }
	   now := svc.now()
	   var rows []BoardRow
	   for _, reg := range regions {
		   marks, err := svc.st.ListMarks(ctx, reg.ID)
		  if err != nil {
			  continue
		   }
		   for _, m := range marks {
			   inv, _ := svc.st.FindInv(ctx, reg.ID, m.Tier, m.Asset, m.Chain)
			   avail := "0"
			   frozen := "0"
			  if inv.ID != "" {
				  avail = inv.Available
				  frozen = inv.Frozen
			   }
			  br := ""
			  dbps := 0
			   if a, err := bookOf(avail); err == nil {
				   if low, err := bookOf(m.Low); err == nil && a.Cmp(low) < 0 {
					   br = BreachBelow
				  }
				  if high, err := bookOf(m.High); err == nil && a.Cmp(high) > 0 {
					   br = BreachAbove
				   }
				  if tgt, err := bookOf(m.Target); err == nil {
					  dbps, _ = DriftBPS(avail, tgt.String())
				   }
			  }
			  rows = append(rows, BoardRow{
				   RegionCode: reg.Code,
				  Tier:      m.Tier,
				  Asset:     m.Asset,
				   Chain:     m.Chain,
				  Available: avail,
				  Frozen:    frozen,
				   Low:    m.Low,
				  Target: m.Target,
				   High:   m.High,
				  Firewall: reg.Firewall,
				   Halted: reg.Halted,
				  Breach: br,
				  DriftBPS: dbps,
			   })
		  }
	  }
	  return Board{Rows: rows, At: now}, nil
  }

// InspectBreaches 返回当前越界清单。
   func (svc *Service) InspectBreaches(ctx context.Context) ([]Breach, error) {
	   regions, err := svc.st.ListRegions(ctx)
	   if err != nil {
		  return nil, err
	  }
	  var out []Breach
	  for _, reg := range regions {
		  marks,
			  err := svc.st.ListMarks(ctx, reg.ID)
		  if err != nil {
			   continue
		   }
		  for _, m := range marks {
			   inv, err := svc.st.FindInv(ctx, reg.ID, m.Tier, m.Asset, m.Chain)
			  if err != nil {
				   continue
			  }
			   a, err := bookOf(inv.Available)
			   if err != nil {
				  continue
			  }
			  low, _ := bookOf(m.Low)
			  high, _ := bookOf(m.High)
			   tgt, _ := bookOf(m.Target)
			   dbps, _ := DriftBPS(inv.Available, tgt.String())
			  if a.Cmp(low) < 0 {
				  out = append(out, Breach{
					   RegionID: reg.ID, RegionCode: reg.Code,
					  Tier: m.Tier, Asset: m.Asset, Chain: m.Chain,
					   Kind: BreachBelow, Available: inv.Available, Bound: low.String(),
					  DriftBPS: dbps,
				   })
			  } else if a.Cmp(high) > 0 {
				   out = append(out, Breach{
					  RegionID: reg.ID,
					   RegionCode: reg.Code,
					   Tier: m.Tier, Asset: m.Asset, Chain: m.Chain,
					   Kind: BreachAbove, Available: inv.Available, Bound: high.String(),
					  DriftBPS: dbps,
				   })
			  }
		  }
	   }
	  return out, nil
   }

// MoveQuoteRegions 为跨区搬运出报价（不落计划）。
// 返回按水位裁剪的可搬量和双方快照。
  func (svc *Service) MoveQuoteRegions(ctx context.Context, t Transfer, now time.Time) (MoveQuote, error) {
	  if now.IsZero() {
		   now = svc.now()
	   }
	   srcR, err := svc.st.FindRegion(ctx, t.SrcRegionID)
	  if err != nil {
		   return MoveQuote{}, err
	   }
	  dstR, err := svc.st.FindRegion(ctx, t.DstRegionID)
	   if err != nil {
		  return MoveQuote{}, err
	  }
	  srcInv, err := svc.st.FindInv(ctx, t.SrcRegionID, t.Tier, t.Asset, t.Chain)
	  if err != nil {
		   return MoveQuote{}, ErrNotFound
	  }
	   dstInv, err := svc.st.FindInv(ctx, t.DstRegionID, t.Tier, t.Asset, t.Chain)
	  if err != nil {
		   dstInv = RegionInv{RegionID: t.DstRegionID, Tier: t.Tier, Asset: t.Asset, Chain: t.Chain, Available: "0"}
	  }
	  srcMark, err := svc.st.FindMark(ctx, t.SrcRegionID, t.Tier, t.Asset, t.Chain)
	   if err != nil {
		   return MoveQuote{}, ErrWatermark
	   }
	  dstMark, err := svc.st.FindMark(ctx, t.DstRegionID, t.Tier, t.Asset, t.Chain)
	  if err != nil {
		   return MoveQuote{}, ErrWatermark
	   }
	  // 可搬量 = min(src_avail - src_target, dst_target - dst_avail)
	  srcAvail, _ := bookOf(srcInv.Available)
	  srcTarget,
		  _ := bookOf(srcMark.Target)
	  dstAvail, _ := bookOf(dstInv.Available)
	   dstTarget, _ := bookOf(dstMark.Target)
	   srcSurplus,
		  err := srcAvail.Sub(srcTarget)
	  if err != nil || !srcSurplus.IsPositive() {
		  return MoveQuote{}, ErrWrongRebalance
	  }
	   dstDeficit, err := dstTarget.Sub(dstAvail)
	  if err != nil || !dstDeficit.IsPositive() {
		  return MoveQuote{}, ErrWrongRebalance
	  }
	  amt := minAmt(srcSurplus,
		   dstDeficit)
	  if req, err := money.Parse(t.Amount); err == nil && req.IsPositive() {
		  amt = minAmt(amt, req)
	   }
	  return MoveQuote{
		  Amount: amt.String(),
		  Src:    srcR,
		  Dst: dstR,
		   SrcInv: srcInv,
		  DstInv: dstInv,
		  SrcMark: srcMark,
		  DstMark: dstMark,
	  }, nil
   }
