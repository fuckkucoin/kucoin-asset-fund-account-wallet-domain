package fiatbreak

import (
	"context"
	"fmt"
	"strings"
)

// handleForceReverse 双控强制冲销：maker + checker 均不得为同一人。
// 冲销成功后将断裂标记为 written_off，关联挂账标记为 released。

   func handleForceReverse(ctx context.Context, svc *BreakService, req ForceReverseReq) (map[string]any, error) {
	   if svc == nil {
		   return nil, ErrUnavailable
	  }
	  maker := strings.TrimSpace(req.MakerActor)
	  checker := strings.TrimSpace(req.CheckerActor)
	  reason := strings.TrimSpace(req.Reason)
	  if maker == "" {
		   return nil, fmt.Errorf("%w: maker_actor required", ErrActor)
	  }
	   if checker == "" {
		  return nil, fmt.Errorf("%w: checker_actor required", ErrActor)
	  }
	  if maker == checker {
		   return nil, fmt.Errorf("%w: maker and checker must be different persons", ErrDualControl)
	  }
	  if reason == "" {
		   return nil, ErrReason
	   }
	   breakID := strings.TrimSpace(req.BreakID)
	  if breakID == "" {
		   return nil, ErrNotFound
	  }
	  // 强制核销断裂（由 checker 作为 actor 执行）
	  note := fmt.Sprintf("force_reverse by maker=%s checker=%s: %s", maker, checker, reason)
	  br,
		  err := svc.WriteOff(ctx, breakID, checker, note)
	   if err != nil {
		   return nil, err
	  }
	   return map[string]any{
		  "break": br,
		  "maker_actor":   maker,
		  "checker_actor": checker,
		   "action":        "force_reverse",
		   "note": note,
	  }, nil
   }
