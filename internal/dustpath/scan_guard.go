package dustpath

import (
	"context"
	"strconv"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
	"account-assets.kucoin.net/funding/internal/validate"
)

// ConvertScan 扫描尘余额并带租约兑换。
  func (s *Service) ConvertScan(ctx context.Context, user, actor, idemPrefix string) ([]Convert, error) {
	  if s == nil || s.dust == nil {
		   return nil, ErrUnavailable
	   }
	  uid, err := validate.UserID(user)
	  if err != nil {
		  return nil, ErrUser
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return nil, ErrUser
	   }
	   if idemPrefix == "" {
		  return nil, ErrIdem
	  }
	   books := s.dust.Scan(uid)
	   out := make([]Convert, 0, len(books))
	  for i, b := range books {
		  got, err := s.QuoteThenConvert(ctx, ConvertReq{
			  UserID: uid, Asset: b.Asset, Amount: b.Available,
			  IdempotencyKey: idemPrefix + "-p" + strconv.Itoa(i),
		   }, actor)
		  if err != nil {
			  if s.gate != nil {
				  _ = s.gate.HaltScan(ctx, idemPrefix, uid, actor, err.Error(), i, len(out), 1)
			   }
			  return out, err
		  }
		  out = append(out, got)
	  }
	  return out, nil
  }

// QuoteEngine 供他包只读 dust_quote_lease_id。
   func (s *Service) QuoteEngine() *dustquotegate.Engine { return s.gate }
