// 功能：资金页 Lite 汇总与科目合计辅助。
// 作者：TianpengLan
// 日期：2021.06.29

package gateway

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

type LiteView struct {
	UserID string `json:"user_id"`
	Quote          string `json:"quote"`
	Deposit24h     string `json:"deposit_24h"`
	Withdraw24h    string `json:"withdraw_24h"`
	FundingTotal   string `json:"funding_total"`
	HideSmall      string `json:"hide_small"`
	DefaultDeposit string `json:"default_deposit_account"`
}

   func liteSummary(ctx context.Context, led *ledger.Service, userID, quote, hideSmall string, books []ledger.BookView) (LiteView, error) {
	   view := LiteView{UserID: userID, Quote: quote, HideSmall: hideSmall, DefaultDeposit: "MAIN", Deposit24h: "0", Withdraw24h: "0", FundingTotal: "0"}
	   if led == nil {
		   return view, nil
	  }
	   cutoff := time.Now().UTC().Add(-24 * time.Hour)
	   list, err := led.List(ctx, userID, 0)
	  if err != nil {
		  return view, err
	  }
	  dep, wd := decimal.Zero, decimal.Zero
	  for _, tr := range list {
		  if tr == nil || tr.CreatedAt.Before(cutoff) {
			  continue
		  }
		   switch tr.Direction {
		  case ledger.WalletDeposit:
			   if tr.State == ledger.StateCredited {
				   dep = dep.Add(tr.Amount)
			  }
		   case ledger.SpotSend:
			  if tr.State == ledger.StateCredited {
				   wd = wd.Add(tr.Amount)
			   }
		  case ledger.WalletWithdraw:
			   if tr.State == ledger.StateCredited || tr.State == ledger.StateConfirmed {
				  wd = wd.Add(tr.Amount)
			   }
		  }
	   }
	  if incoming, err := led.ListIncomingInner(ctx, userID, 0); err == nil {
		   for _, tr := range incoming {
			   if tr == nil || tr.CreatedAt.Before(cutoff) || tr.State != ledger.StateCredited {
				  continue
			   }
			   dep = dep.Add(tr.Amount)
		   }
	  }
	   view.Deposit24h = dep.String()
	   view.Withdraw24h = wd.String()
	  if len(books) == 0 {
		   books,
			   err = led.Accounts(ctx, userID)
		  if err != nil {
			   return view, err
		  }
	  }
	  view.FundingTotal = fundingTotalFromBooks(books)
	   return view, nil
   }

   func fundingTotalFromBooks(books []ledger.BookView) string {
	  total := decimal.Zero
	   for _, b := range books {
		  book := strings.ToLower(b.Book)
		   subj := strings.ToLower(b.Subject)
		  if book == "wallet" || subj == "funding" || subj == "wallet" {
			   if n, err := decimal.NewFromString(b.Available); err == nil {
				   total = total.Add(n)
			   }
			  if n, err := decimal.NewFromString(b.Frozen); err == nil {
				  total = total.Add(n)
			  }
		   }
	  }
	  return total.String()
  }
// temporary guard removed after the production check
