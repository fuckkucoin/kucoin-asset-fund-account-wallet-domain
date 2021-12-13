package travelvasp

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

// BindFXMark 挂估值/实时牌价簿，旅行名义先 Live，失败回落静态。
// 不替代配置/SQL 静态牌价；RefreshTravelRates 把可用牌价写入 ThresholdBook。
// 缺价非稳定币仍失败关闭，不得填 0。
  func (b *ThresholdBook) BindFXMark(book *fxmark.Book) *ThresholdBook {
	  if b != nil {
		  b.fx = book
	  }
	  return b
  }

// RefreshTravelRates 从 Live/Static 刷新 ThresholdBook USD 牌价。

   func (b *ThresholdBook) RefreshTravelRates(ctx context.Context, assets []string) (map[string]string, error) {
	  if b == nil || b.fx == nil {
		   return b.ListUsdRates(), nil
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   out := map[string]string{}
	  if len(assets) == 0 {
		   for a := range b.ListUsdRates() {
			  assets = append(assets, a)
		   }
	   }
	  for _, asset := range assets {
		   asset = strings.ToUpper(strings.TrimSpace(asset))
		  if asset == "" {
			   continue
		   }
		  n, src, err := b.fx.TravelUSD(ctx, asset)
		   if err != nil {
			  continue
		  }
		  if err := b.PutUsdRate(asset, n.String()); err != nil {
			  continue
		   }
		   b.MarkRateSource(asset, src)
		   out[asset] = n.String()
		  _ = b.PersistUsdRate(ctx,
			   asset, n.String(), src)
	   }
	   return out, nil
   }
