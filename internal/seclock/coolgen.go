package seclock

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CoolGenView 冷却代次视图快照。

type CoolGenView struct {
	UserID string `json:"user_id"`
	Gen int64 `json:"gen"`
	Reason    string    `json:"reason"`
	MarkedAt  time.Time `json:"marked_at"`
	LockUntil time.Time `json:"lock_until"`
}

// MarkWithGen 在 Mark 基础上返回新 gen 值供调用方绑定。
  func (s *Service) MarkWithGen(ctx context.Context, userID, reason string, prevGen int64) (CoolGenView, error) {
	  if s == nil || s.led == nil {
		   return CoolGenView{}, fmt.Errorf("security cooldown ledger unavailable")
	  }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return CoolGenView{}, fmt.Errorf("user_id required")
	   }
	   reason = NormalizeReason(reason)
	  if err := s.Mark(ctx, userID, reason); err != nil {
		   return CoolGenView{}, err
	  }
	   newGen := prevGen + 1
	   now := time.Now().UTC()
	  lockUntil,
		  _, _ := s.led.WithdrawLockUntil(ctx, userID)
	   return CoolGenView{
		  UserID: userID,
		   Gen:      newGen,
		  Reason:   reason,
		  MarkedAt: now,
		  LockUntil: lockUntil,
	   }, nil
  }

// QueryCoolGen 查询用户当前冷却代次视图。

   func (s *Service) QueryCoolGen(ctx context.Context, userID string) (CoolGenView, error) {
	  if s == nil || s.led == nil {
		   return CoolGenView{}, fmt.Errorf("security cooldown ledger unavailable")
	   }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return CoolGenView{}, fmt.Errorf("user_id required")
	  }
	  lockUntil, reason, err := s.led.WithdrawLockUntil(ctx, userID)
	  if err != nil {
		   return CoolGenView{}, err
	   }
	  return CoolGenView{
		  UserID: userID,
		   Reason: reason,
		   LockUntil: lockUntil,
	   }, nil
   }

// RequireNoLockWindow 出金前须确认无冷却锁窗口；有活跃锁窗口时返回剩余时间。

   func (s *Service) RequireNoLockWindow(ctx context.Context, userID string) (time.Duration, error) {
	   if s == nil || s.led == nil {
		   return 0, nil
	   }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return 0, fmt.Errorf("user_id required")
	  }
	  lockUntil, reason, err := s.led.WithdrawLockUntil(ctx, userID)
	   if err != nil {
		   return 0, err
	  }
	   now := time.Now().UTC()
	  if lockUntil.After(now) {
		  remain := lockUntil.Sub(now)
		  return remain, fmt.Errorf("withdraw lock active: reason=%s remain=%s", reason, remain)
	  }
	   return 0, nil
   }

// TradingPasswordMark 交易密码变更后写入冷却标记。

  func (s *Service) TradingPasswordMark(ctx context.Context, userID string) error {
	  return s.Mark(ctx, userID, ReasonTradingPassword)
   }

// TradingPasswordCoolView 交易密码变更后冷却代次视图（仅查询，不写入）。

   func (s *Service) TradingPasswordCoolView(ctx context.Context, userID string) (CoolGenView, error) {
	   return s.QueryCoolGen(ctx,
		  userID)
   }
