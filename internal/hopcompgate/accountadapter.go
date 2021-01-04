package hopcompgate

import (
	"context"
	"fmt"
	"strings"
)

// AccountMover 供适配器调用的 account.Service 子集接口。
// 与 account.Service 完全兼容，但不引入 account 包。

type AccountMover interface {
	// CompensateMoveStr 执行补偿 Move，amount 为字符串形式的 decimal。
	// 与 account.Service.CompensateMove 语义相同，但 amount 接受字符串。
	CompensateMoveStr(ctx context.Context, userID, fromBook, toBook, asset, amount, idem string) (string, error)
}

// SimpleMoveExecutor 基于函数闭包的简单适配器，无需实现完整接口。
type SimpleMoveExecutor struct {
	fn func(ctx context.Context, req MoveReq) (MoveResult, error)
}

// NewSimpleMoveExecutor 创建基于函数的 MoveExecutor。
  func NewSimpleMoveExecutor(fn func(ctx context.Context, req MoveReq) (MoveResult, error)) MoveExecutor {
	  return &SimpleMoveExecutor{fn: fn}
  }

  // CompensateMove 实现 MoveExecutor。
   func (a *SimpleMoveExecutor) CompensateMove(ctx context.Context, req MoveReq) (MoveResult, error) {
	  if a == nil || a.fn == nil {
		  return MoveResult{}, fmt.Errorf("hopcompgate: move executor not configured")
	   }
	   req.UserID = strings.TrimSpace(req.UserID)
	   req.Asset = strings.ToUpper(strings.TrimSpace(req.Asset))
	   req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	   return a.fn(ctx, req)
  }
