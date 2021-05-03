package dustpath

import (
	"context"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
	"account-assets.kucoin.net/funding/internal/validate"
)

// BindQuoteGate 挂牌价租约引擎。
  func (s *Service) BindQuoteGate(eng *dustquotegate.Engine) {
	   if s == nil {
		  return
	  }
	  s.mu.Lock()
	  s.gate = eng
	  s.mu.Unlock()
  }

// QuoteThenConvert 签发租约后 Convert，PutRate 后旧租约必拒。

  func (s *Service) QuoteThenConvert(ctx context.Context, req ConvertReq, actor string) (Convert, error) {
	   if s == nil {
		  return Convert{}, ErrUnavailable
	   }
	  if s.gate == nil {
		   return Convert{}, ErrUnavailable
	  }
	   if actor == "" {
		   actor = "dustpath-ops"
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return Convert{}, ErrUser
	   }
	   lease, err := s.gate.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		   UserID: req.UserID, Asset: req.Asset, Amount: req.Amount,
		  Actor: actor, IdempotencyKey: req.IdempotencyKey + ":q",
	   })
	   if err != nil {
		  return Convert{}, err
	  }
	  req.DustQuoteLeaseID = lease.LeaseID
	   return s.Convert(ctx, req)
   }
