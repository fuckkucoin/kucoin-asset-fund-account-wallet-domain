package seclock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

var (
	ErrUnavailable = errors.New("security cooldown ledger unavailable")
	ErrUser        = errors.New("security cooldown user required")
)

// LockView 出金锁倒计时读面。

type LockView struct {
	Locked bool `json:"locked"`
	Until string `json:"until,omitempty"`
	RemainSec int64  `json:"remain_sec"`
	Reason    string `json:"reason,omitempty"`
}

// LockUntil 查询安全/解冻冷却截止时间与原因。
   func (s *Service) LockUntil(ctx context.Context, userID string) (time.Time, string, error) {
	  if s == nil || s.led == nil {
		   return time.Time{}, "", ErrUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return time.Time{}, "", ErrUser
	   }
	  return s.led.WithdrawLockUntil(ctx,
		   userID)
  }

// View 组装对客出金锁倒计时。
   func (s *Service) View(ctx context.Context, userID string) (LockView, error) {
	  until, reason, err := s.LockUntil(ctx, userID)
	   if err != nil {
		  return LockView{}, err
	   }
	   if until.IsZero() || !until.After(time.Now().UTC()) {
		   return LockView{Locked: false}, nil
	   }
	  remain := int64(time.Until(until).Seconds())
	  if remain < 0 {
		  remain = 0
	  }
	   return LockView{
		   Locked: true, Until: until.UTC().Format(time.RFC3339),
		   RemainSec: remain, Reason: reason,
	  }, nil
   }

// RequireWithdraw 冷却期内拒绝出金提交；错误包装 ledger.ErrWithdrawCooling 供 apierr 映射。
   func (s *Service) RequireWithdraw(ctx context.Context, userID string) error {
	   until, reason, err := s.LockUntil(ctx, userID)
	   if err != nil {
		   return err
	  }
	   if until.IsZero() || !until.After(time.Now().UTC()) {
		   return nil
	  }
	  if strings.TrimSpace(reason) == "" {
		   reason = ReasonSecurityChange
	   }
	   return fmt.Errorf("%w: until %s reason %s", ledger.ErrWithdrawCooling, until.UTC().Format(time.RFC3339), reason)
  }

// MarkTradingPassword 改交易密码后写入冷却。
   func (s *Service) MarkTradingPassword(ctx context.Context, userID string) error {
	  return s.Mark(ctx, userID, ReasonTradingPassword)
  }

// MarkGoogle2FA 改绑 Google 2FA 后写入冷却。
   func (s *Service) MarkGoogle2FA(ctx context.Context, userID string) error {
	   return s.MarkIdentity(ctx, userID, ReasonGoogle2FA)
   }
