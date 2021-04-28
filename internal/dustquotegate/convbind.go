package dustquotegate

import "context"

// ConvBind 实现 dustconv.LeaseGate。
type ConvBind struct {
	eng *Engine
}

// NewConvBind 构造。

   func NewConvBind(eng *Engine) *ConvBind { return &ConvBind{eng: eng} }

  func (b *ConvBind) AssertBeforeConvert(ctx context.Context, leaseID, user, asset, amount, actor string) error {
	  if b == nil || b.eng == nil {
		  return ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   _, err := b.eng.AssertLease(ctx, BindReq{
		   LeaseID: leaseID, UserID: user, Asset: asset, Amount: amount, Actor: actor,
	   })
	   return err
   }

   func (b *ConvBind) SettleAfterConvert(ctx context.Context, leaseID, convertID, usdt, actor string) error {
	  if b == nil || b.eng == nil {
		  return ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  _, err := b.eng.SettleLease(ctx, SettleReq{
		   LeaseID: leaseID, ConvertID: convertID, USDT: usdt, Actor: actor,
	   })
	   return err
   }

   func (b *ConvBind) FailClosed(ctx context.Context, leaseID, actor, detail string) error {
	  if b == nil || b.eng == nil {
		   return ErrUnavailable
	  }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   return b.eng.FailClosedConserve(ctx,
		   leaseID, actor, detail)
  }
