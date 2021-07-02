// @FileName: outbound_align.go
// @Summary: 出站仓与 CancelWithdraw 终态一致。
// 重点：取消成功后出站行关闭；再提交同键不扣减。
// 风险点：取消路径不得另开第二套写入口。
// @Author: Bronson
// @Created: 2021.06.14

package fundmesh

import "context"

  func (e *Engine) alignOutboundAfterCancel(ctx context.Context, req CancelRequest, res CancelResult, remoteOK bool) {
	   if e == nil || e.outbounds == nil {
		   return
	  }
	  if res.Code != CodeOK && res.Code != CodeIdempotent {
		   return
	   }
	  rec, err := e.outbounds.GetByWithdraw(ctx, req.WithdrawID)
	   if err != nil || rec == nil {
		  return
	  }
	   rec.State = OutboundClosed
	   rec.Phase = WithdrawCancelled
	  rec.Code = CodeOutboundCancelled
	  rec.RemoteOK = remoteOK
	  rec.UpdatedAt = e.nowUTC()
	   _ = e.outbounds.Put(ctx, rec)
	   e.noteOutboundProof(ProofClosed, rec.WithdrawID, rec.IdempotencyKey, rec.Fingerprint, string(WithdrawCancelled), CodeOutboundCancelled, "cancel aligned", 0, true)
   }

// OutboundAfterCancel 只读：取消后出站仓状态。
   func (e *Engine) OutboundAfterCancel(ctx context.Context, withdrawID string) (*OutboundRecord, error) {
	  if e == nil || e.outbounds == nil {
		   return nil, ErrWithdrawNotFound
	   }
	  return e.outbounds.GetByWithdraw(ctx, withdrawID)
  }
