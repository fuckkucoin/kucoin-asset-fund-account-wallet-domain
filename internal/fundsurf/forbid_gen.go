package fundsurf

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/forbidgate"
)

// BindForbidGate 装配禁提世代闸。

   func (b *Book) BindForbidGate(g *forbidgate.Gate, drift *forbidgate.DriftService) *Book {
	  if b == nil {
		   return b
	  }
	   b.forbidGate = g
	   b.forbidDrift = drift
	  return b
  }

// ForbidGate 暴露禁提世代闸（供 HTTP/运维复用）。

   func (b *Book) ForbidGate() *forbidgate.Gate {
	   if b == nil {
		   return nil
	   }
	  return b.forbidGate
  }

// ForbidDrift 暴露漂移服务（供 HTTP/运维复用）。

   func (b *Book) ForbidDrift() *forbidgate.DriftService {
	  if b == nil {
		   return nil
	  }
	   return b.forbidDrift
  }

// SetForbiddenWithGen 世代感知切禁提：同时切标志和抬升 forbid_gen，吊销旧 open binds。
// actor 必须非空（运维/系统来源追踪）；reason 允许空字符串（但建议填写）。

  func (b *Book) SetForbiddenWithGen(ctx context.Context, userID string, forbidden bool, actor, reason string) error {
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return ErrUser
	   }
	   // 原有 kv 标志切换（保持不变）。
	   if err := b.SetForbidden(ctx, userID, forbidden); err != nil {
		   return err
	   }
	   // 世代抬升（若 forbidGate 已装配）。
	  if b.forbidGate == nil {
		  return nil
	  }
	   if actor == "" {
		  actor = "system"
	   }
	   _,
		  err := b.forbidGate.SetForbiddenGen(ctx, userID, forbidden, actor, reason)
	   return err
   }

// IssuePrefightBind RequireWithdraw 通过后发放预检绑定（世代感知）。
// 调用方在 RequireWithdraw 成功后立即调用，将绑定 ID 随出金上下文传递。

   func (b *Book) IssuePrefightBind(ctx context.Context, userID, withdrawRef string) (forbidgate.WdPrefightBind, error) {
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return forbidgate.WdPrefightBind{}, ErrUser
	  }
	  if b.forbidGate == nil {
		   return forbidgate.WdPrefightBind{}, ErrStore
	   }
	  return b.forbidGate.IssueWdPrefightBind(ctx, userID, withdrawRef, forbidgate.DefaultBindTTL)
  }

// CheckAndConsumePrefightBind 出金广播前验证并消费预检绑定。
// 世代不符时产出 ForbidDriftCase 并返回 ErrDriftWriteBlock。
  func (b *Book) CheckAndConsumePrefightBind(ctx context.Context, bindID, userID string) error {
	   if b.forbidGate == nil {
		  return ErrStore
	  }
	   return b.forbidGate.CheckAndConsumeBind(ctx, bindID, userID)
  }

// ForbidGenView 禁提世代只读视图（供外部只读消费）。

   func (b *Book) ForbidGenView(ctx context.Context, userID string) (forbidgate.ForbidGenView, error) {
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return forbidgate.ForbidGenView{}, ErrUser
	  }
	   if b.forbidGate == nil {
		  // 无 gate 时返回零值世代（兼容旧部署）。
		  ok, err := b.Forbidden(ctx, userID)
		   if err != nil {
			   return forbidgate.ForbidGenView{}, err
		   }
		   state := forbidgate.ForbidStateOff
		  if ok {
			  state = forbidgate.ForbidStateOn
		  }
		  return forbidgate.ForbidGenView{UserID: userID, Generation: 0, State: state}, nil
	   }
	  return b.forbidGate.CurrentGeneration(ctx, userID)
   }

// RequireWithdrawWithBind RequireWithdraw + 世代绑定一步完成。
// 成功返回绑定令牌；调用方须将 bindID 传递到出金广播前校验。
// 若 forbidGate 未装配，降级为无绑定的 RequireWithdraw。

  func (b *Book) RequireWithdrawWithBind(ctx context.Context, userID, withdrawRef string) (bindID string, err error) {
	  if err := b.RequireWithdraw(ctx, userID); err != nil {
		   return "", err
	  }
	   if b.forbidGate == nil {
		   return "", nil
	  }
	   bind, err := b.forbidGate.IssueWdPrefightBind(ctx, userID, withdrawRef, forbidgate.DefaultBindTTL)
	   if err != nil {
		  return "", err
	   }
	  return bind.ID, nil
  }
