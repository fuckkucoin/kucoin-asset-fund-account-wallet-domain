// GET /v1/assets/overview 的 quote、hide_small 以 fundpref.Book 为准；查询参数显式传入时覆盖偏好。
// 损坏载荷返回错误，不用工厂默认冒充已读到的偏好。

// Author: Axel
// Date: 2021.06.29

package gateway

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

type displayOpts struct {
	Quote     string
	HideSmall string
	Hide      bool
	Pref      fundpref.Preference
	FromPref  bool
}

// resolveDisplayOpts 合并 fundpref、FundsPageConfig 与查询串。
// 偏好载荷损坏或存储失败时返回错误，不用工厂默认冒充已读到的偏好。

   func resolveDisplayOpts(ctx context.Context, rt *app.Runtime, userID, quoteQ, hideQ string) (displayOpts, error) {
	  out := displayOpts{Quote: "USDT", HideSmall: "1"}
	  if rt != nil && rt.Cfg != nil {
		  if rt.Cfg.Funds.QuoteCurrency != "" {
			   out.Quote = rt.Cfg.Funds.QuoteCurrency
		   }
		  if rt.Cfg.Funds.HideSmall != "" {
			  out.HideSmall = rt.Cfg.Funds.HideSmall
		   }
	   }
	   if strings.TrimSpace(userID) != "" {
		  p, err := prefBook(rt).Get(ctx, userID)
		   if err != nil {
			   return out, err
		  }
		   out.Pref = p
		  out.FromPref = true
		  if p.DisplayCurrency != "" {
			  out.Quote = p.DisplayCurrency
		   }
		   if p.HideZero {
			  out.Hide = true
		  }
	  }
	  if strings.TrimSpace(quoteQ) != "" {
		  out.Quote = strings.ToUpper(strings.TrimSpace(quoteQ))
	  }
	  switch strings.ToLower(strings.TrimSpace(hideQ)) {
	  case "1", "true", "yes":
		   out.Hide = true
	   case "0", "false", "no":
		  out.Hide = false
	   }
	  return out, nil
   }
