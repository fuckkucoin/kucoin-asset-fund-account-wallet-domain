// 现货账和钱包账按资产汇总。
// 作者: Likeo
// 日期: 2020.11.24
package wallet

import (
	"sort"
	"github.com/shopspring/decimal"
)

// UnifiedBalance 钱包页汇总：现货账 + 钱包账 + 各链原生。链上原生不混进托管可用。

type UnifiedBalance struct {
	Asset string `json:"asset"`
	SubjectFunding string `json:"subject_funding"`
	SubjectWallet  string `json:"subject_wallet"`
	SpotAvailable  string `json:"spot_available"`
	SpotFrozen       string `json:"spot_frozen"`
	SpotTotal        string `json:"spot_total"`
	FundingAvailable string `json:"funding_available"`
	FundingFrozen    string `json:"funding_frozen"`
	FundingTotal     string `json:"funding_total"`
	TradingAvailable string `json:"trading_available"`
	TradingFrozen    string `json:"trading_frozen"`
	MarginAvailable string `json:"margin_available"`
	MarginFrozen string `json:"margin_frozen"`
	IsolatedAvailable string `json:"isolated_available"`
	IsolatedFrozen   string `json:"isolated_frozen"`
	FuturesAvailable string `json:"futures_available"`
	FuturesFrozen    string `json:"futures_frozen"`
	EarnAvailable string `json:"earn_available"`
	EarnFrozen      string `json:"earn_frozen"`
	WalletAvailable string `json:"wallet_available"`
	WalletFrozen string            `json:"wallet_frozen"`
	WalletTotal  string            `json:"wallet_total"`
	ChainNatives map[string]string `json:"chain_natives,omitempty"`
	DegradedChains []string `json:"degraded_chains,omitempty"`
}

// UnifyBalances 现货账 + 钱包账 + 各链原生按资产汇总，给钱包页一起看。链上原生不混进托管可用。

  func UnifyBalances(rows []BookBalance) []UnifiedBalance {
	   by := map[string]*UnifiedBalance{}
	  for _, r := range rows {
		  u := by[r.Asset]
		  if u == nil {
			   u = &UnifiedBalance{
				  Asset: r.Asset, SubjectFunding: "funding", SubjectWallet: "wallet",
				   SpotAvailable: "0", SpotFrozen: "0", SpotTotal: "0",
				  FundingAvailable: "0", FundingFrozen: "0", FundingTotal: "0",
				   TradingAvailable: "0", TradingFrozen: "0",
				   MarginAvailable: "0", MarginFrozen: "0",
				   IsolatedAvailable: "0", IsolatedFrozen: "0",
				  FuturesAvailable: "0", FuturesFrozen: "0",
				  EarnAvailable: "0", EarnFrozen: "0",
				  WalletAvailable: "0", WalletFrozen: "0", WalletTotal: "0",
				  ChainNatives: map[string]string{},
			   }
			  by[r.Asset] = u
		  }
		  switch r.Book {
		   case "spot":
			  u.SpotAvailable = r.Available
			  u.SpotFrozen = r.Frozen
			   u.SpotTotal = addDec(r.Available, r.Frozen)
			   // KuCoin MAIN/Funding = 托管资金账 + 钱包账。
			   u.FundingAvailable = addDec(u.FundingAvailable, r.Available)
			   u.FundingFrozen = addDec(u.FundingFrozen, r.Frozen)
			  u.FundingTotal = addDec(u.FundingTotal, u.SpotTotal)
		   case "wallet":
			  u.WalletAvailable = r.Available
			   u.WalletFrozen = r.Frozen
			   u.WalletTotal = addDec(r.Available, r.Frozen)
			  u.FundingAvailable = addDec(u.FundingAvailable, r.Available)
			  u.FundingFrozen = addDec(u.FundingFrozen, r.Frozen)
			  u.FundingTotal = addDec(u.FundingTotal,
				   u.WalletTotal)
		  case "trading":
			   u.TradingAvailable = r.Available
			   u.TradingFrozen = r.Frozen
		   case "margin":
			  u.MarginAvailable = r.Available
			  u.MarginFrozen = r.Frozen
		  case "isolated":
			  u.IsolatedAvailable = r.Available
			   u.IsolatedFrozen = r.Frozen
		  case "futures":
			  u.FuturesAvailable = r.Available
			  u.FuturesFrozen = r.Frozen
		  case "earn":
			   u.EarnAvailable = r.Available
			  u.EarnFrozen = r.Frozen
		  }
		   if r.Chain != "" && r.ChainNative != "" {
			   u.ChainNatives[r.Chain] = r.ChainNative
		   }
		   if r.Degraded && r.Chain != "" {
			  u.DegradedChains = append(u.DegradedChains, r.Chain)
		  }
	   }
	  out := make([]UnifiedBalance, 0, len(by))
	  for _, u := range by {
		   if len(u.ChainNatives) == 0 {
			  u.ChainNatives = nil
		  }
		   out = append(out, *u)
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].Asset < out[j].Asset })
	   return out
   }

  func addDec(a, b string) string {
	  x, e1 := decimal.NewFromString(a)
	   y, e2 := decimal.NewFromString(b)
	   if e1 != nil || e2 != nil {
		   return a
	   }
	   return x.Add(y).String()
  }
