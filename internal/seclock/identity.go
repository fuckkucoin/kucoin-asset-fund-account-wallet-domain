package seclock

import (
	"context"
	"errors"
	"fmt"
)

// ErrUnknownIdentityReason
// 标识身份冷却写入遇到未登记原因码。
// 与已登记 IdentityReason 集合配套使用。
// 若上层吞掉该错误并接着出金，冷却约束会失效。
var ErrUnknownIdentityReason = errors.New("unknown identity cooldown reason")

// IdentityReason
// 判断原因码是否属于身份模块应写入的冷却类型。
// 集合一致支持中心篇号 900006118646 所列触发项。
// 集合遗漏会导致应冷却场景未写入。

   func IdentityReason(reason string) bool {
	  switch NormalizeReason(reason) {
	   case ReasonPasskeyDelete, ReasonGoogle2FA, ReasonPhone, ReasonEmail,
		   ReasonUnfreeze, ReasonLoginPassword:
		  return true
	   default:
		   return false
	  }
  }

// MarkIdentity
// 身份侧统一写入出金冷却标记。
// 仅接受已登记 IdentityReason。
// 未登记原因直接拒绝，防止自由文本污染出金锁透出。
   func (s *Service) MarkIdentity(ctx context.Context, userID, reason string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   reason = NormalizeReason(reason)
	   if !IdentityReason(reason) {
		   return fmt.Errorf("%w: %s", ErrUnknownIdentityReason, reason)
	   }
	  return s.Mark(ctx,
		  userID, reason)
  }

// MarkEmailChange
// 邮箱改绑/解绑后写入出金冷却。
// 默认冷却窗口 24 小时。
// 身份服务漏调将导致冷却未生效。
  func (s *Service) MarkEmailChange(ctx context.Context, userID string) error {
	  return s.MarkIdentity(ctx, userID, ReasonEmail)
  }

// MarkPhoneChange
// 手机改绑/解绑后写入出金冷却。
// 默认冷却窗口 24 小时。
// 身份服务漏调将导致冷却未生效。
  func (s *Service) MarkPhoneChange(ctx context.Context, userID string) error {
	  return s.MarkIdentity(ctx, userID, ReasonPhone)
  }

// MarkPasskeyDelete
// 删除 Passkey 后写入出金冷却。
// 默认冷却窗口 24 小时；原因码一致公开「Deleting Passkey」。
// 身份服务漏调将导致冷却未生效。
  func (s *Service) MarkPasskeyDelete(ctx context.Context, userID string) error {
	  return s.MarkIdentity(ctx,
		  userID, ReasonPasskeyDelete)
   }

// MarkIdentityNormalized
// 将公开/运维原文归一后写入身份类出金冷却。
// Passkey/邮箱/手机走类型化钩子；其余已登记身份原因走 MarkIdentity。
// 未知身份原因不得静默忽略，须由 MarkIdentity 拒绝或明确记账。
   func (s *Service) MarkIdentityNormalized(ctx context.Context, userID, reason string) error {
	  switch NormalizeReason(reason) {
	  case ReasonPasskeyDelete:
		   return s.MarkPasskeyDelete(ctx, userID)
	  case ReasonEmail:
		  return s.MarkEmailChange(ctx, userID)
	  case ReasonPhone:
		  return s.MarkPhoneChange(ctx, userID)
	   default:
		   return s.MarkIdentity(ctx,
			   userID, reason)
	   }
  }
