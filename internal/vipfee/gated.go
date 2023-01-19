package vipfee

import (
	"context"
	"account-assets.kucoin.net/funding/internal/viptiergate"
)

// GatedPin 带世代门控的 Pin 入口。
// gate 为 nil 时等价于 Service.Pin（向后兼容）。
// 返回 (Pin, newGeneration, TierLease, error)；调用方可将 TierLease.ID 作为
// tier_lease_id 供他包只读消费。
   func (s *Service) GatedPin(
	   ctx context.Context,
	   gate *viptiergate.Service,
	   req PinReq,
   ) (Pin, int64, *viptiergate.TierLease, error) {
	  if gate == nil {
		  p, err := s.Pin(req)
		  return p, 0, nil, err
	   }
	  // 1. 检查是否有阻塞性漂移案。
	  blocked, reason, err := gate.IsSettlementBlocked(ctx, req.UserID)
	   if err != nil {
		   return Pin{}, 0, nil, err
	   }
	   if blocked {
		  return Pin{}, 0, nil, viptiergate.ErrDriftBlocked
	   }
	   _ = reason
	   // 2. 读取当前世代（CAS 前置条件）。
	   expectedGen := s.CurrentGen(req.UserID)
	  // 3. 原有 Pin 落账。
	  p, err := s.Pin(req)
	   if err != nil {
		   return Pin{}, 0, nil, err
	   }
	   // 4. CAS 世代递增 + 作废旧 QuoteBind + 创建 TierLease。
	  fromTier := TierStandard
	   // 解析 ToTier：Pin 指定的目标档位。
	  toTier,
		   terr := ParseTier(req.Tier)
	  if terr != nil {
		  toTier = TierStandard
	  }
	   lease, newGen, gerr := gate.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID: p.UserID,
		   FromTier:    viptiergate.Tier(fromTier),
		   ToTier:      viptiergate.Tier(toTier),
		  TriggerKind: "pin",
		   TriggerRef:  p.UserID + "|pin|" + p.At.UTC().Format("20060102150405"),
		   ExpectedGen: expectedGen,
	  })
	  if gerr != nil {
		  // GatedUpgrade 失败（ErrStaleGen 或 ErrDriftBlocked）；
		  // 业务 Pin 已落账但世代未递增。此场景由 worker 下次扫描补偿世代。
		   return p, 0, nil, gerr
	   }
	   // 同步内存世代（与 gate 数据库世代保持一致）。
	   _,
		  _ = s.BumpGen(p.UserID, expectedGen, toTier)
	  return p, newGen, lease, nil
  }

// GatedClearPin 带世代门控的 ClearPin 入口。
// gate 为 nil 时等价于 Service.ClearPin（向后兼容）。
// 若存在在途冻额返回 ErrInflightFreeze；若存在在途 Quote 返回 ErrInflightQuote。

  func (s *Service) GatedClearPin(
	   ctx context.Context,
	   gate *viptiergate.Service,
	   userID, actor string,
  ) (int64, *viptiergate.TierLease, error) {
	  if gate == nil {
		   return 0, nil, s.ClearPin(userID, actor)
	   }
	  // 1. 读取当前世代（CAS 前置条件）。
	  expectedGen := s.CurrentGen(userID)
	  // 2. 降档互斥检查 + CAS 世代递增（在 GatedDowngrade 内完成）。
	  // 注意：ClearPin 是降档到 standard；fromTier 由调用方 PinOf 读取。
	  currentPin, err := s.PinOf(userID)
	   fromTier := TierStandard
	  if err == nil {
		   fromTier = currentPin.Tier
	  }
	  lease, newGen, gerr := gate.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID: userID,
		  FromTier: viptiergate.Tier(fromTier),
		  ToTier:   viptiergate.Tier(TierStandard),
		   TriggerKind: "clear_pin",
		  TriggerRef:  userID + "|clear_pin|" + actor,
		  ExpectedGen: expectedGen,
	   })
	   if gerr != nil {
		   // 互斥检查失败（冻额/Quote），返回对应错误，ClearPin 不执行。
		   return 0, nil, gerr
	  }
	   // 3. 通过互斥检查，执行原有 ClearPin 落账。
	   if err := s.ClearPin(userID, actor); err != nil {
		   // 业务 ClearPin 失败；世代已递增但 Pin 未清除。
		  // worker 下次扫描时会发现租约 bound 但 pin 仍存在，开立 DriftCase 处理。
		   return newGen, lease, err
	  }
	  // 同步内存世代。
	  _, _ = s.BumpGen(userID, expectedGen, TierStandard)
	   return newGen, lease, nil
   }

// GatedRecordVolume 带世代门控的 RecordVolume 入口。
// 仅当本次记量触发档位抬升（量过 VIP/VIP+ 阈值）时才调用 gate.GatedUpgrade。
// gate 为 nil 时等价于 Service.RecordVolume（向后兼容）。
  func (s *Service) GatedRecordVolume(
	  ctx context.Context,
	   gate *viptiergate.Service,
	   req VolumeReq,
  ) (Fill, int64, *viptiergate.TierLease, error) {
	   // 记录前先读取当前档位。
	  beforeEff, _ := s.Resolve(ResolveReq{UserID: req.UserID, Asset: req.Asset})
	  beforeTier := beforeEff.Tier
	  fill, err := s.RecordVolume(req)
	   if err != nil {
		  return Fill{}, 0, nil, err
	   }
	   if gate == nil {
		   return fill, 0, nil, nil
	   }
	   // 记录后再读取档位。
	  afterEff, aerr := s.Resolve(ResolveReq{UserID: req.UserID, Asset: req.Asset})
	   if aerr != nil {
		  return fill, 0, nil, nil
	   }
	  afterTier := afterEff.Tier
	   // 仅在档位发生抬升时触发门控。
	  if !viptiergate.IsUpgrade(viptiergate.Tier(beforeTier), viptiergate.Tier(afterTier)) {
		   return fill, 0, nil, nil
	  }
	  // 检查阻塞性漂移案。
	  blocked, _, berr := gate.IsSettlementBlocked(ctx, req.UserID)
	  if berr != nil || blocked {
		  return fill, 0, nil, nil // 不阻断量记录，仅跳过世代推进
	   }
	   expectedGen := s.CurrentGen(req.UserID)
	   lease, newGen, gerr := gate.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID:   fill.UserID,
		   FromTier: viptiergate.Tier(beforeTier),
		  ToTier: viptiergate.Tier(afterTier),
		  TriggerKind: "volume",
		   TriggerRef:  fill.ID,
		   ExpectedGen: expectedGen,
	  })
	  if gerr != nil {
		  return fill, 0, nil, nil // 世代竞争失败不阻断 Fill 返回
	  }
	  _, _ = s.BumpGen(fill.UserID, expectedGen, afterTier)
	   return fill, newGen, lease, nil
   }
