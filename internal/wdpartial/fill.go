package wdpartial

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// CreateFill 创建部分成交分录。

   func (s *Service) CreateFill(req CreateFillReq) (PartialFill, error) {
	  if s == nil {
		  return PartialFill{}, ErrUnavailable
	  }
	   if err := validateCreateFill(req); err != nil {
		  return PartialFill{}, err
	   }
	  amt, err := money.Parse(req.Amount)
	  if err != nil || !amt.IsPositive() {
		   return PartialFill{},
			  ErrAmountZero
	   }
	   now := s.now()
	   fill := PartialFill{
		   ID: uuid.NewString(),
		  PlanID: strings.TrimSpace(req.PlanID),
		   LegID:  strings.TrimSpace(req.LegID),
		  HoldID: strings.TrimSpace(req.HoldID),
		   UserID: strings.TrimSpace(req.UserID),
		   Asset:  strings.TrimSpace(req.Asset),
		   Amount: amt.String(),
		  TxHash: strings.TrimSpace(req.TxHash),
		   Chain:  strings.TrimSpace(req.Chain),
		  State:  FillPending,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if fee := strings.TrimSpace(req.FeeAmount); fee != "" {
		   if feeAmt, ferr := money.Parse(fee); ferr == nil && feeAmt.IsPositive() {
			   fill.FeeAmount = feeAmt.String()
		   }
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  s.shardFills[fill.ID] = &fill
	  s.fillsByPlan[fill.PlanID] = append(s.fillsByPlan[fill.PlanID], fill.ID)
	  return fill.Copy(), nil
  }

// ConfirmFill 链上确认成交分录。幂等：已 confirmed 直接返回。
   func (s *Service) ConfirmFill(req ConfirmFillReq) (PartialFill, error) {
	   if s == nil {
		   return PartialFill{}, ErrUnavailable
	   }
	  if strings.TrimSpace(req.FillID) == "" {
		  return PartialFill{}, ErrNoFill
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  f, ok := s.shardFills[req.FillID]
	  if !ok {
		   return PartialFill{}, ErrFillNotFound
	  }
	  if f.State == FillConfirmed {
		  return f.Copy(), nil
	   }
	   if f.State != FillPending {
		  return f.Copy(), ErrState
	  }
	  now := s.now()
	   f.State = FillConfirmed
	   if h := strings.TrimSpace(req.TxHash); h != "" {
		  f.TxHash = h
	  }
	   f.ConfirmedAt = ptrTime(now)
	   f.UpdatedAt = now
	   return f.Copy(), nil
   }

// VoidFill 作废成交分录（广播失败后）。只能作废 pending 态。
  func (s *Service) VoidFill(fillID, reason string) (PartialFill, error) {
	  if s == nil {
		   return PartialFill{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   f, ok := s.shardFills[strings.TrimSpace(fillID)]
	  if !ok {
		  return PartialFill{}, ErrFillNotFound
	  }
	   if f.State != FillPending {
		  return f.Copy(), ErrState
	  }
	   now := s.now()
	   f.State = FillVoided
	   f.UpdatedAt = now
	   _ = reason
	   return f.Copy(), nil
  }

   func validateCreateFill(r CreateFillReq) error {
	  if strings.TrimSpace(r.PlanID) == "" {
		   return ErrNoPlan
	  }
	  if strings.TrimSpace(r.LegID) == "" {
		  return ErrNoLeg
	   }
	   if strings.TrimSpace(r.HoldID) == "" {
		   return ErrNoHold
	   }
	  if strings.TrimSpace(r.UserID) == "" {
		   return ErrNoUser
	  }
	  if strings.TrimSpace(r.Asset) == "" {
		  return ErrNoAsset
	  }
	  if strings.TrimSpace(r.Chain) == "" {
		   return ErrNoAsset
	  }
	   return nil
  }

   func (s *HoldService) CreateFill(req CreateFillReq) (PartialFill, error) {
	  if s == nil {
		  return PartialFill{}, ErrUnavailable
	   }
	   if err := validateCreateFill(req); err != nil {
		   return PartialFill{}, err
	  }
	  amt, err := money.Parse(req.Amount)
	  if err != nil || !amt.IsPositive() {
		  return PartialFill{}, ErrAmountZero
	   }
	   now := s.now()
	  fill := PartialFill{
		  ID:     uuid.NewString(),
		   PlanID: strings.TrimSpace(req.PlanID),
		  LegID:  strings.TrimSpace(req.LegID),
		   HoldID: strings.TrimSpace(req.HoldID),
		  UserID: strings.TrimSpace(req.UserID),
		   Asset: strings.TrimSpace(req.Asset),
		   Amount: amt.String(),
		  TxHash: strings.TrimSpace(req.TxHash),
		  Chain:  strings.TrimSpace(req.Chain),
		  State:     FillPending,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if fee := strings.TrimSpace(req.FeeAmount); fee != "" {
		   if feeAmt, ferr := money.Parse(fee); ferr == nil && feeAmt.IsPositive() {
			   fill.FeeAmount = feeAmt.String()
		   }
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   s.fills[fill.ID] = &fill
	  s.fillsByPlan[fill.PlanID] = append(s.fillsByPlan[fill.PlanID], fill.ID)
	   return fill.Copy(), nil
   }

// ConfirmFill 链上确认成交分录。幂等：已 confirmed 直接返回。

   func (s *HoldService) ConfirmFill(req ConfirmFillReq) (PartialFill, error) {
	   if s == nil {
		  return PartialFill{},
			  ErrUnavailable
	   }
	   if strings.TrimSpace(req.FillID) == "" {
		  return PartialFill{}, ErrNoFill
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   f, ok := s.fills[req.FillID]
	   if !ok {
		   return PartialFill{},
			  ErrFillNotFound
	   }
	  if f.State == FillConfirmed {
		  return f.Copy(), nil
	  }
	  if f.State != FillPending {
		   return f.Copy(), ErrState
	   }
	   now := s.now()
	  f.State = FillConfirmed
	  if h := strings.TrimSpace(req.TxHash); h != "" {
		  f.TxHash = h
	   }
	   f.ConfirmedAt = ptrTime(now)
	  f.UpdatedAt = now
	   return f.Copy(), nil
   }

// VoidFill 作废成交分录（广播失败后）。只能作废 pending 态。

  func (s *HoldService) VoidFill(fillID, reason string) (PartialFill, error) {
	   if s == nil {
		   return PartialFill{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   f, ok := s.fills[strings.TrimSpace(fillID)]
	   if !ok {
		  return PartialFill{}, ErrFillNotFound
	  }
	   if f.State != FillPending {
		  return f.Copy(), ErrState
	  }
	  now := s.now()
	  f.State = FillVoided
	   f.UpdatedAt = now
	  return f.Copy(), nil
   }
