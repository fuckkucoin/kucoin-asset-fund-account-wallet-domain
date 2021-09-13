package oracle

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Tick struct {
	Asset string `json:"asset"`
	Quote string `json:"quote"`
	Price string `json:"price"`
	At time.Time `json:"at"`
}

  func pairKey(asset, quote string) string {
	   return strings.ToUpper(strings.TrimSpace(asset)) + "/" + strings.ToUpper(strings.TrimSpace(quote))
  }

   func ParseTick(asset, quote, price string, at time.Time) (Tick, error) {
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return Tick{}, err
	  }
	   q, err := validate.Asset(quote)
	  if err != nil {
		   return Tick{}, err
	   }
	   if as == q {
		   return Tick{}, ErrPair
	  }
	   if _, err := money.Parse(price); err != nil {
		  return Tick{}, ErrPrice
	   }
	  if at.IsZero() {
		  return Tick{}, ErrPrice
	  }
	   return Tick{Asset: as, Quote: q, Price: price, At: at.UTC()}, nil
   }
