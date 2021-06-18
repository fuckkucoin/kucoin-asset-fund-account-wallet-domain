package fundpref

import (
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

const maxAssetOrder = 32

var marketKeys = map[string]struct{}{
	"symbol": {}, "pair": {}, "ticker": {}, "kline": {}, "klines": {},
	"depth": {}, "match": {}, "matchmaking": {}, "orderbook": {},
	"market": {}, "last_tick": {}, "lastTick": {}, "tick": {},
}

// Validate 规范化并核对已归并的偏好。损坏或非法即关。

   func Validate(p *Preference) error {
	   if p == nil {
		  return ErrCorrupt
	  }
	  uid, err := validate.UserID(p.UserID)
	   if err != nil {
		  return ErrUser
	  }
	   p.UserID = uid
	  book,
		  err := ParseDefaultAccount(p.DefaultAccount)
	   if err != nil {
		   return ErrAccount
	  }
	   p.DefaultAccount = book
	   cur, err := validate.Asset(p.DisplayCurrency)
	  if err != nil {
		  return ErrCurrency
	   }
	  p.DisplayCurrency = cur
	   order, err := normalizeOrder(p.AssetOrder)
	   if err != nil {
		  return err
	   }
	   p.AssetOrder = order
	   return nil
   }

   func normalizeOrder(in []string) ([]string, error) {
	  if len(in) == 0 {
		   return []string{}, nil
	  }
	  if len(in) > maxAssetOrder {
		   return nil, ErrOrder
	   }
	  seen := make(map[string]struct{}, len(in))
	  out := make([]string, 0, len(in))
	  for _, raw := range in {
		   a, err := validate.Asset(raw)
		  if err != nil {
			   return nil, ErrOrder
		   }
		   if _, ok := seen[a]; ok {
			  return nil, ErrOrder
		   }
		  seen[a] = struct{}{}
		   out = append(out, a)
	  }
	   return out, nil
   }

// RejectMarket 写体带行情/撮合键则失败即关。
   func RejectMarket(raw []byte) error {
	  if len(raw) == 0 {
		  return nil
	   }
	   var bag map[string]json.RawMessage
	   if err := json.Unmarshal(raw, &bag); err != nil {
		   return ErrCorrupt
	   }
	   for k := range bag {
		   if _, bad := marketKeys[strings.TrimSpace(k)]; bad {
			  return ErrMarket
		   }
	   }
	   return nil
   }

  func mergeWrite(base Preference, req WriteReq) Preference {
	  out := base
	   if v := firstNonEmpty(req.DefaultAccount, req.DefaultAccount2); v != "" {
		  out.DefaultAccount = v
	  }
	  if v := firstNonEmpty(firstNonEmpty(req.DisplayCurrency, req.Currency), req.QuoteCurrency); v != "" {
		   out.DisplayCurrency = v
	   }
	   if req.HideZero != nil {
		   out.HideZero = *req.HideZero
	  } else if req.HideSmall != nil {
		   out.HideZero = *req.HideSmall
	   } else if req.HideSmallAlt != nil {
		  out.HideZero = *req.HideSmallAlt
	  }
	  if req.AssetOrder != nil {
		   out.AssetOrder = append([]string(nil), req.AssetOrder...)
	  } else if req.AssetOrder2 != nil {
		   out.AssetOrder = append([]string(nil), req.AssetOrder2...)
	  } else if req.CurrencyOrder != nil {
		   out.AssetOrder = append([]string(nil), req.CurrencyOrder...)
	  }
	   return out
  }

  func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		  return a
	   }
	  return strings.TrimSpace(b)
   }

// Defaults 未落库用户的工厂默认。不是损坏回落。
   func Defaults(userID string) Preference {
	  return Preference{
		  UserID: strings.TrimSpace(userID),
		   DefaultAccount:  AccountFunding,
		  DisplayCurrency: "USDT",
		  HideZero:        false,
		  AssetOrder: []string{},
	  }
  }
