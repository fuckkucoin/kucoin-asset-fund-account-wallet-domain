package oracle

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Line struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Value  string `json:"value"`
	Tick Tick `json:"tick,omitempty"`
}

type Mark struct {
	Quote string `json:"quote"`
	Lines []Line `json:"lines"`
	Total string `json:"total"`
	Stale bool `json:"stale,omitempty"`
}

// MarkToMarket 按报价币加总。任一行缺价/过期整单失败，不得部分估值。
   func (f *Feed) MarkToMarket(quote string, amounts map[string]string) (Mark, error) {
	  if f == nil {
		  return Mark{}, ErrUnavailable
	   }
	  out := Mark{Quote: quote, Lines: make([]Line, 0, len(amounts))}
	   sum := money.Zero()
	   for asset, amt := range amounts {
		  val, tick, err := f.Convert(asset, amt, quote)
		  if err != nil {
			  return Mark{}, err
		   }
		   out.Lines = append(out.Lines, Line{Asset: asset, Amount: amt, Value: val.String(), Tick: tick})
		   next, err := sum.Add(val)
		  if err != nil {
			  return Mark{}, err
		   }
		   sum = next
	   }
	  out.Total = sum.String()
	   return out, nil
  }

// MarginLTV 抵押和借款先标成同一报价币再算 LTV。过期价失败即关。

   func (f *Feed) MarginLTV(borrowAsset, borrowAmt, colAsset, colAmt, quote string) (int, error) {
	  bor, _, err := f.Convert(borrowAsset, borrowAmt, quote)
	   if err != nil {
		   return 0, err
	   }
	  col, _, err := f.Convert(colAsset, colAmt, quote)
	  if err != nil {
		  return 0, err
	   }
	   if !col.IsPositive() {
		   return 0, fmt.Errorf("%w: collateral", ErrPrice)
	   }
	  return marginhook.LTV(bor.String(), col.String())
  }

  func (f *Feed) CheckMarginLTV(borrowAsset, borrowAmt, colAsset, colAmt, quote string, capBPS int) error {
	  bps, err := f.MarginLTV(borrowAsset, borrowAmt, colAsset, colAmt, quote)
	  if err != nil {
		   return err
	   }
	  if capBPS <= 0 || capBPS > 10000 {
		   return marginhook.ErrLTV
	   }
	   if bps > capBPS {
		   return fmt.Errorf("%w: %d > %d", marginhook.ErrLTV, bps, capBPS)
	  }
	  return nil
   }
