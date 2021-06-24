package fundpath

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/hopcompgate"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func (s *Service) Hop(ctx context.Context, req HopReq) (string, error) {
	  if s == nil {
		  return "", ErrHop
	   }
	  if s.Halted() {
		  return "", ErrHalt
	  }
	   uid,
		   err := validate.UserID(req.UserID)
	  if err != nil {
		   return "", ErrUser
	  }
	  as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return "", ErrAsset
	   }
	  amt, err := money.Parse(req.Amount)
	   if err != nil {
		   return "", ErrAmount
	  }
	   idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		   return "", ErrIdem
	  }
	   from, err := account.ParseBook(req.FromBook)
	  if err != nil {
		   return "", ErrHop
	  }
	  to, err := account.ParseBook(req.ToBook)
	   if err != nil {
		  return "", ErrHop
	   }
	   if s.acc == nil {
		   return "", ErrHop
	   }
	   // 资金域统一熔断闸：只读判定，跳闸时拒绝新 hop，不改科目冻结与已成交划转。
	   if err := s.fundCircuit(ctx, uid, as, string(from), string(to)); err != nil {
		  return "", err
	  }
	  // 同片 hop 补偿重入检查：若同 idem 已有未闭合的补偿仓，拒绝 hop 重入。
	   if s.compGate != nil {
		   if rerr := s.compGate.CheckReentry(ctx, idem); rerr != nil {
			  return "", fmt.Errorf("%w: %v", ErrHop, rerr)
		   }
	   }
	   if _, err := s.acc.CheckHop(uid, from, to); err != nil {
		   return "", err
	   }
	   if s.xfer != nil {
		  rec, err := s.xfer.Open(ctx, uid, string(from), string(to), as, amt.String(), idem)
		   if err != nil {
			   return "", err
		  }
		  if rec.TransferID != "" {
			   return rec.TransferID, nil
		   }
	  }
	  if s.shard != nil {
		  plan, err := s.shard.SettleHop(uid, string(from), string(to), as, amt.String(), idem)
		   if err != nil {
			   return "", err
		  }
		   if s.xfer != nil {
			   rec, gerr := s.xfer.Get(ctx, idem)
			   if gerr != nil {
				   return "", gerr
			   }
			  if _, aerr := s.xfer.AnnotateRoute(ctx, idem, plan.SrcShard, plan.DstShard, plan.Version, plan.IntentID, plan.SameShard, rec.Fence.Epoch, rec.Fence.Token); aerr != nil {
				  return "", aerr
			   }
		   }
		  if !plan.SameShard {
			  if plan.IntentID == "" {
				  return "", ErrHop
			  }
			  return plan.IntentID, nil
		   }
	   }
	  move := account.MoveReq{
		   UserID: uid, From: from, To: to, Asset: as, Amount: amt.Dec(), IdempotencyKey: idem,
	  }
	  tr, err := s.acc.Move(ctx, move)
	  if err != nil {
		  return "", err
	   }
	   if s.xfer != nil && tr != nil {
		  rec, gerr := s.xfer.Get(ctx, idem)
		   if gerr != nil {
			   return "", s.compensateHop(ctx, move, tr.ID, gerr)
		  }
		  if _, berr := s.xfer.BindTransfer(ctx, idem, tr.ID, rec.Fence.Epoch, rec.Fence.Token); berr != nil {
			  return "", s.compensateHop(ctx, move, tr.ID, berr)
		  }
	   }
	  return tr.ID, nil
   }

// compensateHop 在 BindTransfer 失败时执行补偿，并将补偿结果写入 hopcompgate（若已绑定）。
// SameShard 路径：本函数只在 same-shard Move 后调用；跨片路径由 shard 层自行处理。
   func (s *Service) compensateHop(ctx context.Context, move account.MoveReq, moveID string, cause error) error {
	  if s.acc == nil {
		  return fmt.Errorf("%w: %v", ErrHop, cause)
	   }
	   // 若已绑定补偿闸，先建仓（幂等）
	  if s.compGate != nil && strings.TrimSpace(moveID) != "" {
		  _, _ = s.compGate.OpenCase(ctx, hopcompgate.OpenCaseReq{
			  HopIdem: move.IdempotencyKey,
			   UserID: move.UserID,
			   Asset:    move.Asset,
			  Amount:   move.Amount.String(),
			   FromBook: string(move.From),
			   ToBook: string(move.To),
			   MoveID:    moveID,
			  BindErr:   cause.Error(),
			  SameShard: true,
			  Actor:     "fundpath.compensateHop",
		  })
	  }
	  if _, err := s.acc.CompensateMove(ctx, move); err != nil {
		   return fmt.Errorf("%w: bind %v compensate %v", ErrHop, cause, err)
	  }
	  return cause
   }
