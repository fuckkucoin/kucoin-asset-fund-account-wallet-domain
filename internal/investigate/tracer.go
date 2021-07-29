package investigate

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/snap"
)

type Hop struct {
	FromBook string `json:"from_book"`
	ToBook   string `json:"to_book"`
	Asset    string `json:"asset"`
	Amount string `json:"amount"`
	Dir string `json:"direction"`
	ID string `json:"transfer_id"`
}

type Trace struct {
	UserID string `json:"user_id"`
	Asset string          `json:"asset"`
	Hops  []Hop           `json:"hops"`
	Books []snap.Position `json:"books"`
}

// TraceAsset 只跟内部 hop 和充提。金额必须能过 ParseBook。
   func TraceAsset(userID, asset string, transfers []*ledger.Transfer, views []ledger.BookView) (Trace, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return Trace{}, fmt.Errorf("asset required")
	  }
	  out := Trace{UserID: userID,
		  Asset: asset}
	   for _, tr := range transfers {
		   if tr == nil || !strings.EqualFold(tr.Asset, asset) {
			  continue
		   }
		   if _, err := money.ParseBook(tr.Amount.String()); err != nil {
			   return Trace{}, err
		  }
		  from, to := booksOf(tr.Direction)
		  out.Hops = append(out.Hops, Hop{
			   FromBook: from,
			  ToBook: to, Asset: asset,
			  Amount: tr.Amount.String(), Dir: string(tr.Direction), ID: tr.ID,
		   })
	   }
	  for _, v := range views {
		  if !strings.EqualFold(v.Asset, asset) {
			  continue
		   }
		   p, err := posOf(v)
		  if err != nil {
			  return Trace{}, err
		  }
		  out.Books = append(out.Books, p)
	  }
	   return out, nil
  }

  func posOf(v ledger.BookView) (snap.Position, error) {
	   avail, err := money.ParseBook(orZero(v.Available))
	  if err != nil {
		  return snap.Position{}, err
	  }
	   fr, err := money.ParseBook(orZero(v.Frozen))
	   if err != nil {
		   return snap.Position{}, err
	   }
	  tot, err := avail.Add(fr)
	   if err != nil {
		   return snap.Position{}, err
	  }
	  return snap.Position{
		   Book: v.Book, Subject: v.Subject, Asset: v.Asset,
		  Available: avail.String(),
		   Frozen: fr.String(), Total: tot.String(),
	   }, nil
   }

   func booksOf(d ledger.Direction) (from, to string) {
	  switch d {
	   case ledger.EarnSubscribe:
		  return "funding", "earn"
	   case ledger.EarnRedeem:
		   return "earn", "funding"
	  case ledger.FundingToTrading:
		  return "funding", "trading"
	   case ledger.TradingToFunding:
		   return "trading", "funding"
	   case ledger.FundingToMargin:
		  return "funding", "margin"
	   case ledger.MarginToFunding:
		  return "margin", "funding"
	  case ledger.FundingToFutures:
		   return "funding", "futures"
	   case ledger.FuturesToFunding:
		  return "futures", "funding"
	   case ledger.SpotToWallet:
		  return "funding", "wallet"
	  case ledger.WalletToSpot:
		  return "wallet", "funding"
	  case ledger.WalletWithdraw:
		  return "wallet", "chain"
	  case ledger.WalletDeposit:
		   return "chain", "wallet"
	   default:
		  return string(d), ""
	   }
   }

  func orZero(s string) string {
	  if strings.TrimSpace(s) == "" {
		   return "0"
	  }
	   return s
  }
