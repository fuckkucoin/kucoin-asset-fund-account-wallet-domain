package fundintgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/fundint"
)

// Engine 实现 fundint.LeaseEngine。

type Engine struct {
	Svc *Service
}

// BindFundint 把引擎挂到 fundint.Service。

   func BindFundint(fi *fundint.Service, svc *Service) {
	  if fi == nil || svc == nil {
		   return
	  }
	   fi.BindLeaseEngine(Engine{Svc: svc})
   }

// CheckPass 签发租约。

   func (e Engine) CheckPass(ctx context.Context, req fundint.Req) (string, error) {
	  if e.Svc == nil {
		  return "", ErrUnavailable
	   }
	  lease,
		  err := e.Svc.CheckWithLease(ctx, fromFundintReq(req))
	   if err != nil {
		   return "", err
	   }
	   return lease.LeaseID, nil
   }

// AssertSend 复验并 SpotSend。fundint.SendResolved 在 AssertSend 成功后仍会再调 ledger.Create；
// 为避免双记，本路径仅复验+绑定，不调用 Sender。SpotSend 仍由 fundint 主链发起。
   func (e Engine) AssertSend(ctx context.Context, leaseID string, req fundint.Req) error {
	  if e.Svc == nil {
		   return ErrUnavailable
	  }
	   // 仅复验：把 Sender 临时置空会导致 SendAssert 返回 ErrSpotSendBlocked。
	   // 因此走专用 RebindOnly。
	   return e.Svc.RebindOnly(ctx, SendAssertReq{
		  LeaseID: leaseID,
		  UserID:  req.UserID,
		  ToUID:   req.ToUID,
		  ToEmail: req.ToEmail,
		  ToPhone:        req.ToPhone,
		  Asset:          req.Asset,
		  Amount:         req.Amount.String(),
		   IdempotencyKey: req.IdempotencyKey,
		  Remark:         req.Remark,
	   })
  }

// CloseSend 迁入 sent。
  func (e Engine) CloseSend(ctx context.Context, leaseID, transferID string) error {
	   if e.Svc == nil {
		  return ErrUnavailable
	  }
	   return e.Svc.MarkSent(ctx, leaseID, transferID)
   }

  func fromFundintReq(req fundint.Req) CheckWithLeaseReq {
	  return CheckWithLeaseReq{
		  UserID: req.UserID,
		   ToUID:   req.ToUID,
		   ToEmail: req.ToEmail,
		   ToPhone: req.ToPhone,
		  Asset: req.Asset,
		  Amount:         req.Amount.String(),
		  IdempotencyKey: "check:" + req.IdempotencyKey,
	  }
  }
