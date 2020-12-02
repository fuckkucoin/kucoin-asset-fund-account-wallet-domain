package cvfund

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"github.com/shopspring/decimal"
)

// OccIntegrityChecker 路径占用完整性校验接口（由 cvoccup.Service 实现）。
// 注入后 FreezeIntegrity 同时校验 PathOcc 层，确保占用令牌与 holdcat 一致。
type OccIntegrityChecker interface {
	// CheckIntegrity 校验路径占用令牌一致性；openBreach=true 时对漂移开 BreachEvent。
	CheckIntegrity(ctx context.Context, occID string, openBreach bool) error
	// GetOccByOrder 按订单 ID 取路径占用（用于关联校验）。
	GetOccByOrder(ctx context.Context, orderID string) (interface {
		GetHoldToken() string
		GetStatus() string
	}, error)
}

// FreezeIntegrity 核对限价单 holdcat 与剩余未成交是否一致（强制闸）。
// OPEN/PARTIAL 须有 HoldID 且 holdcat 行存在；FILLED/CANCELLED/EXPIRED 不得残留 hold。
// 分类账与账本漂移时以账本为准；本检查只返回错误，不得静默补铸。
// 若注入了 OccIntegrityChecker，同步校验路径级占用令牌。

  func (b *Book) FreezeIntegrity(ctx context.Context, userID, orderID string) error {
	  o, err := b.loadLimit(ctx, userID, orderID, "")
	  if err != nil {
		  return err
	   }
	   src,
		   err := decimal.NewFromString(o.FromSize)
	  if err != nil {
		   return err
	  }
	  filled := decimal.Zero
	   if strings.TrimSpace(o.FilledSize) != "" {
		  filled, _ = decimal.NewFromString(o.FilledSize)
	   }
	  remain := src.Sub(filled)
	  switch o.Status {
	   case "OPEN", "PARTIAL":
		  // 强制闸：OPEN/PARTIAL 必须有 HoldID；无则直接返回错误（不静默）
		  if o.HoldID == "" {
			   return fmt.Errorf("integrity gate: convert limit missing hold token order=%s status=%s", o.ID, o.Status)
		  }
		  if b.kv == nil {
			   return fmt.Errorf("integrity gate: kv unavailable order=%s", o.ID)
		  }
		  rows, err := holdcat.New(b.kv).List(ctx, userID, true)
		  if err != nil {
			  return err
		  }
		  found := false
		   for _, r := range rows {
			   if r.ID == o.HoldID || (r.Reason == holdcat.ReasonConvertLimit && strings.Contains(r.Ref, o.ClientOrderID)) {
				  found = true
				   amt, _ := decimal.NewFromString(r.Amount)
				  if remain.IsPositive() && amt.IsPositive() && amt.LessThan(remain) {
					   return fmt.Errorf("integrity gate: convert limit hold underflow order=%s hold=%s remain=%s", o.ID, amt.String(), remain.String())
				   }
				  break
			  }
		  }
		  if !found {
			   return fmt.Errorf("integrity gate: convert limit hold not found order=%s status=%s", o.ID, o.Status)
		  }
		   // 路径级占用令牌校验（若注入 OccChecker）
		  if b.occChecker != nil {
			   if err := b.occChecker.CheckOccByOrder(ctx, o.ID, true); err != nil {
				  return fmt.Errorf("integrity gate: occ token mismatch order=%s: %w", o.ID, err)
			  }
		  }
	  case "FILLED", "CANCELLED", "EXPIRED":
		   // 终态：不得残留 hold；若有则视为穿仓
		  if o.HoldID != "" && b.kv != nil {
			   rows, _ := holdcat.New(b.kv).List(ctx, userID, true)
			  for _, r := range rows {
				  if r.ID == o.HoldID {
					  return fmt.Errorf("integrity gate: hold leaked order=%s status=%s hold=%s", o.ID, o.Status, o.HoldID)
				  }
			   }
		  }
		   // 路径级占用令牌：终态不得有残留占用
		   if b.occChecker != nil {
			   if err := b.occChecker.CheckOccByOrder(ctx, o.ID, true); err != nil {
				   // 终态的占用残留错误也须上报
				  return fmt.Errorf("integrity gate: occ residue at terminal order=%s status=%s: %w", o.ID, o.Status, err)
			  }
		  }
	   }
	   return nil
   }

// OccCheckerAdapter 路径占用校验适配器（轻量接口供 cvfund 使用）。
// 避免 cvfund 直接依赖 cvoccup 包（循环依赖防护）。

type OccCheckerAdapter interface {
	// CheckOccByOrder 按订单 ID 校验路径占用令牌；openBreach=true 时开 BreachEvent。
	CheckOccByOrder(ctx context.Context, orderID string, openBreach bool) error
}

// BindOccChecker 注入路径占用校验器（可选；未注入则退化到 holdcat-only 校验）。
  func (b *Book) BindOccChecker(checker OccCheckerAdapter) *Book {
	  if b != nil {
		   b.occChecker = checker
	  }
	   return b
   }
