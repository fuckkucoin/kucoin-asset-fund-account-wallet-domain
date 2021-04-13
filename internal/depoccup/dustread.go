package depoccup

import "account-assets.kucoin.net/funding/pkg/money"

// DustReader 只读地板与用户尘余额。实现方不得在读路径上转账。

type DustReader interface {
	Floor(asset string) (floor string, enabled bool, ok bool)
}

type DustFact struct {
	UserID string
	Asset  string
	Amount  string
	Floor   string
	Repeats int
}

  func (s *Service) BindDust(r DustReader) {
	   if s == nil {
		   return
	  }
	   s.mu.Lock()
	  s.dust = r
	  s.mu.Unlock()
   }

   func (s *Service) dustFloor(asset string, explicit string) (string, bool) {
	  if explicit != "" {
		   if _, err := money.Parse(explicit); err != nil {
			  return "", false
		   }
		  return explicit, true
	  }
	  if s != nil && s.dust != nil {
		  floor,
			   enabled, ok := s.dust.Floor(asset)
		   if ok && enabled && floor != "" {
			  return floor, true
		   }
	  }
	  if f, ok := builtinDustFloor(asset); ok {
		  return f, true
	  }
	  return "", false
   }

  func builtinDustFloor(asset string) (string, bool) {
	   switch asset {
	  case "BTC":
		  return "0.0001", true
	   case "ETH":
		  return "0.001", true
	   case "USDT", "USDC", "DAI":
		   return "1", true
	  case "SOL":
		  return "0.01", true
	  case "TRX":
		  return "10", true
	   case "DOGE":
		   return "5", true
	   case "SHIB", "PEPE":
		   return "1000000", true
	   default:
		  return "", false
	   }
  }

// BelowFloor 金额不高于地板时为真。地板缺失则不判尘。

  func BelowFloor(amount, floor string) (bool, error) {
	   if floor == "" {
		  return false, nil
	  }
	   amt, err := money.Parse(amount)
	   if err != nil {
		   return false, ErrAmount
	   }
	  fl, err := money.Parse(floor)
	  if err != nil {
		   return false, ErrAmount
	   }
	   return amt.Cmp(fl) <= 0, nil
  }

   func (s *Service) dustSignals(user, asset, amount string, repeats int) ([]SignalKind, error) {
	   floor, ok := s.dustFloor(asset, "")
	  if !ok {
		  return nil, nil
	   }
	   low, err := BelowFloor(amount, floor)
	   if err != nil || !low {
		   return nil, err
	  }
	  out := []SignalKind{SigDustFloor}
	  if repeats >= 3 {
		  out = append(out, SigDustBurst)
	  }
	  _ = user
	   return out, nil
  }
