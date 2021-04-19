package dustconv

import "account-assets.kucoin.net/funding/internal/validate"

// ConvertScanGuarded 带世代钉扎的扫描兑换。
  func (s *Service) ConvertScanGuarded(user, actor, idemPrefix string) ([]Convert, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  uid, err := validate.UserID(user)
	  if err != nil {
		  return nil, ErrUser
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return nil, ErrActor
	   }
	   if idemPrefix == "" {
		   return nil, ErrIdem
	  }
	   rows := s.Scan(uid)
	   out := make([]Convert, 0, len(rows))
	  pinned := map[string]LeaseQuote{}
	   for i, b := range rows {
		  live, err := s.LiveLeaseQuote(uid, b.Asset)
		   if err != nil {
			   return out, err
		   }
		  if prev, ok := pinned[b.Asset]; ok {
			  if err := prev.SameLeaseQuote(live); err != nil {
				   return out, err
			   }
		   } else {
			  pinned[b.Asset] = live
		   }
		  got, err := s.Convert(ConvertReq{
			   UserID: uid, Asset: b.Asset, Amount: b.Available, Actor: actor,
			  IdempotencyKey: idemPrefix + "-" + itoa(i),
		  })
		   if err != nil {
			  return out, err
		  }
		   out = append(out, got)
	  }
	  return out, nil
   }
