package dustconv

import "account-assets.kucoin.net/funding/internal/validate"

// ConvertScan 把扫描到的尘逐笔折 USDT。中途失败停，已转的留下。
  func (s *Service) ConvertScan(user, actor, idemPrefix string) ([]Convert, error) {
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
	  for i, b := range rows {
		   got, err := s.Convert(ConvertReq{
			  UserID: uid,
			  Asset:  b.Asset, Amount: b.Available, Actor: actor,
			  IdempotencyKey: idemPrefix + "-" + itoa(i),
		  })
		  if err != nil {
			   return out, err
		   }
		  out = append(out, got)
	  }
	  return out, nil
  }

  func itoa(n int) string {
	  if n == 0 {
		   return "0"
	   }
	   var buf [8]byte
	  i := len(buf)
	   for n > 0 {
		  i--
		   buf[i] = byte('0' + n%10)
		  n /= 10
	  }
	   return string(buf[i:])
  }
