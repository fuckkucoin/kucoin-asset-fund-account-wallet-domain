// 功能：出金提现域，覆盖管线、广播、RBF 与费用抬升。
// 作者：Gunner
// 日期：2020.11.03

package withdraw

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/vipfee"
)

// BindVIP 挂上档位解析。Request 仍按全额冻；报价走 QuoteWithVIP。

   func (s *Service) BindVIP(v *vipfee.Service) *Service {
	  if s != nil {
		  s.vip = v
	  }
	   return s
   }

type FeeQuote struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Gross  string `json:"gross"`
	Fee string `json:"fee"`
	Net string `json:"net"`
	BPS int    `json:"bps"`
	Tier string `json:"tier,omitempty"`
}

// QuoteWithVIP 出金切费。未绑解析器时 BPS=0，fee=0 net=gross。

   func (s *Service) QuoteWithVIP(userID, asset, gross string) (FeeQuote, error) {
	  if s == nil {
		  return FeeQuote{}, fmt.Errorf("withdraw service unavailable")
	   }
	  amt, err := money.Parse(gross)
	  if err != nil {
		  return FeeQuote{}, err
	  }
	   if s.vip == nil {
		  return FeeQuote{UserID: userID, Asset: asset, Gross: amt.String(), Fee: "0", Net: amt.String()}, nil
	   }
	   q, err := s.vip.QuoteWithdraw(userID, asset, amt.String())
	   if err != nil {
		  return FeeQuote{}, err
	  }
	   if err := money.Conserve(mustAmt(q.Fee).MustAdd(mustAmt(q.Net)), amt); err != nil {
		   return FeeQuote{}, err
	  }
	  return FeeQuote{
		   UserID: q.UserID, Asset: q.Asset, Gross: q.Gross,
		   Fee: q.Fee, Net: q.Net, BPS: q.BPS, Tier: string(q.Tier),
	  }, nil
   }

   func mustAmt(s string) money.Amount {
	   a, err := money.ParseBook(s)
	   if err != nil {
		  return money.Zero()
	   }
	  return a
   }
