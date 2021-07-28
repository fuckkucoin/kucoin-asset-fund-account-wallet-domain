package hotreserve

import (
	"context"
	"strings"
)

// ForceRelease 双人释放未广播预留。
// 做什么：把 open 或 earmarked 冲回热运营可用。
// 申请人与复核人必须是两个非 system 身份；已有广播跟踪则拒绝。
// 不变量：本操作只改预留科目，不调用账本解冻，也不改已兑付单据。

   func (s *Service) ForceRelease(ctx context.Context, req ReleaseReq) (Reservation, error) {
	   if s == nil {
		  return Reservation{}, ErrUnavailable
	   }
	   req.Requester = strings.TrimSpace(req.Requester)
	   req.Approver = strings.TrimSpace(req.Approver)
	  req.Note = clip(req.Note, 255)
	   if err := distinctActors(req.Requester, req.Approver); err != nil {
		  return Reservation{}, err
	   }
	  return s.release(ctx, req.ID, KindForce, req.Requester, req.Approver, req.Note, true)
   }

   func distinctActors(a, b string) error {
	   if a == "" || b == "" {
		   return ErrDual
	  }
	   if strings.EqualFold(a, b) || strings.EqualFold(a, "system") || strings.EqualFold(b, "system") {
		   return ErrDual
	   }
	  if len(a) > 64 || len(b) > 64 {
		  return ErrDual
	   }
	  return nil
  }
