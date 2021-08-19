package kcfund

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/ledger"
)

   func bookToKC(subject, book string) string {
	   s := strings.ToLower(strings.TrimSpace(subject))
	  b := strings.ToLower(strings.TrimSpace(book))
	  switch {
	  case s == "funding" || b == "spot" || b == "wallet" || b == "funding":
		   return "main"
	   case s == "trading" || b == "trading":
		   return "trade"
	   case s == "isolated" || b == "isolated":
		   return "isolated"
	  case s == "margin" || b == "margin":
		   return "margin"
	  case s == "futures" || b == "futures" || b == "contract":
		  return "contract"
	   case s == "earn" || b == "earn":
		   return "earn"
	  default:
		   return b
	  }
   }

  func kcToBook(typ string) (account.Book, bool) {
	   switch strings.ToUpper(strings.TrimSpace(typ)) {
	   case "", "MAIN", "FUNDING", "UNIFIED", "UTA", "UNIFIED_TRADE":
		   // UTA 公开划转额度 accountType 含 UNIFIED；统一账户可划出额落资金科目。
		  return account.BookFunding, true
	  case "TRADE", "TRADING", "TRADE_HF":
		  return account.BookTrading, true
	   case "MARGIN", "MARGIN_HF", "CROSS", "MARGIN_V2":
		   return account.BookMargin,
			  true
	   case "ISOLATED", "ISOLATED_V2":
		  return account.BookIsolated, true
	   case "CONTRACT", "FUTURES":
		   return account.BookFutures, true
	   case "EARN":
		   return account.BookEarn, true
	   default:
		  return "", false
	   }
  }

   func matchType(want, got string) bool {
	   want = strings.ToLower(strings.TrimSpace(want))
	  got = strings.ToLower(strings.TrimSpace(got))
	  if want == "" {
		   return true
	   }
	   if want == "trade_hf" && got == "trade" {
		  return true
	  }
	  if want == "margin_hf" && got == "margin" {
		   return true
	  }
	  if want == "margin_v2" && got == "margin" {
		  return true
	  }
	  if want == "cross" && got == "margin" {
		  return true
	   }
	  if want == "isolated_v2" && got == "isolated" {
		   return true
	   }
	  return want == got
   }

   func convertAccount(typ string) string {
	   switch strings.ToUpper(strings.TrimSpace(typ)) {
	  case "TRADE", "TRADING", "TRADE_HF":
		  return "trading"
	   case "BOTH":
		   return "both"
	  default:
		  return "funding"
	   }
   }

   func ledgerBook(b account.Book) string {
	  switch b {
	   case account.BookTrading:
		   return string(ledger.AccountTrading)
	   case account.BookMargin:
		   return string(ledger.AccountMargin)
	   case account.BookIsolated:
		  return string(ledger.AccountIsolated)
	  case account.BookFutures:
		  return string(ledger.AccountFutures)
	  case account.BookEarn:
		  return string(ledger.AccountEarn)
	   default:
		  return string(ledger.AccountSpot)
	  }
   }
