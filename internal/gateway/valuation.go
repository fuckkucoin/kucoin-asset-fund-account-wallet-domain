package gateway

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

// Valued 单币种估值行（嵌入统一余额视图）。
type Valued struct {
	wallet.UnifiedBalance
	Mark  string `json:"mark,omitempty"`
	Quote string `json:"quote"`
	QuoteValue string `json:"quote_value,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
}

// PortfolioValuation 总览估值结果。
type PortfolioValuation struct {
	Quote string `json:"quote"`
	Total string `json:"total"`
	Items []Valued `json:"items"`
	Skipped    int  `json:"skipped_no_mark"`
	Incomplete bool `json:"incomplete"`
}

// valuePortfolio 按计价币汇总各科目可用+冻结；hide 为 true 时丢弃低于阈值的行且不计入 total。
  func valuePortfolio(ctx context.Context, marks *fxmark.Book, rows []wallet.UnifiedBalance, quote, hideSmall string, hide bool) PortfolioValuation {
	  quote = strings.ToUpper(strings.TrimSpace(quote))
	   if quote == "" {
		   quote = "USDT"
	  }
	  th, err := decimal.NewFromString(strings.TrimSpace(hideSmall))
	  if err != nil || th.IsNegative() {
		   th = decimal.NewFromInt(1)
	   }
	  res := PortfolioValuation{Quote: quote, Total: "0", Items: make([]Valued, 0, len(rows))}
	  if marks == nil {
		   res.Incomplete = true
		  return res
	   }
	   total := decimal.Zero
	   skipped := 0
	   for _, row := range rows {
		   v := Valued{UnifiedBalance: row, Quote: quote}
		  amt := sumDec(row.FundingTotal, row.TradingAvailable, row.MarginAvailable, row.IsolatedAvailable, row.FuturesAvailable, row.EarnAvailable)
		  if !amt.IsPositive() {
			  continue
		  }
		   rate, err := quoteRate(ctx, marks, row.Asset, quote)
		  if err != nil || !rate.IsPositive() {
			  skipped++
			   res.Incomplete = true
			  continue
		  }
		  v.Mark = rate.String()
		  qv := amt.Mul(rate)
		  v.QuoteValue = qv.String()
		   if hide && qv.LessThan(th) {
			   v.Hidden = true
			   continue
		   }
		  total = total.Add(qv)
		  res.Items = append(res.Items, v)
	  }
	  res.Total = total.String()
	   res.Skipped = skipped
	   return res
  }

  func quoteRate(ctx context.Context, marks *fxmark.Book, asset, quote string) (decimal.Decimal, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   quote = strings.ToUpper(strings.TrimSpace(quote))
	  if asset == quote {
		  return decimal.NewFromInt(1),
			  nil
	   }
	  if quote == "USDT" || quote == "USDC" || quote == "USD" {
		   return marks.USD(ctx, asset)
	   }
	  return marks.Rate(ctx, asset, quote)
  }

  func sumDec(parts ...string) decimal.Decimal {
	  n := decimal.Zero
	  for _, p := range parts {
		  if p == "" {
			   continue
		   }
		   if d, err := decimal.NewFromString(p); err == nil {
			  n = n.Add(d)
		  }
	   }
	   return n
   }

  func convertQuoteURL(rt *app.Runtime) string {
	  if rt != nil && rt.Cfg != nil {
		  return rt.Cfg.Funds.ConvertQuoteURL
	  }
	   return ""
  }

// newFxBook 绑定资金页牌价源；URL 须通过 config.AllowOutbound 白名单。
  func newFxBook(kv persist.KV, quoteURL string) *fxmark.Book {
	  b := fxmark.New(kv)
	   if strings.TrimSpace(quoteURL) != "" {
		  b.BindQuoteURL(quoteURL)
	   }
	   return b
  }
