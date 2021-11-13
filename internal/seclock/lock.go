package seclock

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// 支持中心 900006118646 列明的临时停出金触发（资金侧原因码）。身份模块删除 Passkey 须传 ReasonPasskeyDelete。

const (
	ReasonPasskeyDelete   = "passkey_delete"
	ReasonGoogle2FA       = "google_2fa"
	ReasonTradingPassword = "trading_password"
	ReasonPhone = "phone"
	ReasonEmail    = "email"
	ReasonUnfreeze = "unfreeze"
	ReasonLoginPassword = "login_password"
	ReasonSecurityChange = "security_change"
)

// NormalizeReason 归一公开触发到资金侧稳定原因码；未知原文保留小写蛇形。

   func NormalizeReason(reason string) string {
	  r := strings.ToLower(strings.TrimSpace(reason))
	   r = strings.ReplaceAll(r, "-", "_")
	   r = strings.ReplaceAll(r, " ", "_")
	   switch r {
	  case "", "security", "security_change", "securitychange":
		   return ReasonSecurityChange
	   case "passkey_delete", "delete_passkey", "deleting_passkey", "passkey":
		   return ReasonPasskeyDelete
	   case "google_2fa", "2fa", "totp", "ga", "google_authenticator":
		  return ReasonGoogle2FA
	   case "trading_password", "trade_password", "tradepw", "pin", "trading_password_change", "trading_password_reset":
		   return ReasonTradingPassword
	   case "phone", "phone_number", "mobile", "unlink_phone", "change_phone":
		  return ReasonPhone
	  case "email", "unlink_email", "change_email":
		  return ReasonEmail
	  case "unfreeze", "account_unfreeze", "unfreezing":
		   return ReasonUnfreeze
	  case "login_password", "password_reset", "forget_password", "reset_password":
		   return ReasonLoginPassword
	  default:
		  return r
	  }
   }

type Service struct {
	led *ledger.Service
}

  func New(led *ledger.Service) *Service { return &Service{led: led} }

// Mark 记录安全变更并启动出金冷却。
// 原因码归一后写入账本。
// ledger 不可用或用户 ID 非法时不得静默成功。

  func (s *Service) Mark(ctx context.Context, userID, reason string) error {
	  if s == nil || s.led == nil {
		  return ErrUnavailable
	  }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return fmt.Errorf("%w: user_id", ledger.ErrInvalidAddress)
	  }
	   reason = NormalizeReason(reason)
	  return s.led.MarkSecurityChange(ctx, userID, reason)
   }
