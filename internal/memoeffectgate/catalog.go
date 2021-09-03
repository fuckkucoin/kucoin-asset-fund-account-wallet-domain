package memoeffectgate

import "strings"

type ChainRule struct {
	Chain string
	NeedMemo   bool
	UTXO       bool
	MaxOutputs int
	TagName    string
	Confirms   int
}

var chainCatalog = []ChainRule{
	{Chain: "BTC", NeedMemo: false, UTXO: true, MaxOutputs: 256, TagName: "", Confirms: 3},
	{Chain: "LTC", NeedMemo: false, UTXO: true, MaxOutputs: 128, TagName: "", Confirms: 6},
	{Chain: "BCH", NeedMemo: false, UTXO: true, MaxOutputs: 128, TagName: "", Confirms: 6},
	{Chain: "DOGE", NeedMemo: false, UTXO: true, MaxOutputs: 64, TagName: "", Confirms: 20},
	{Chain: "ETH", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 12},
	{Chain: "TRX", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 20},
	{Chain: "XRP", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "destination_tag", Confirms: 1},
	{Chain: "XLM", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "memo", Confirms: 1},
	{Chain: "EOS", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "memo", Confirms: 1},
	{Chain: "ATOM", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "memo", Confirms: 8},
	{Chain: "TON", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "comment", Confirms: 8},
	{Chain: "HBAR", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "memo", Confirms: 1},
	{Chain: "ALGO", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "note", Confirms: 1},
	{Chain: "KAVA", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "memo", Confirms: 8},
	{Chain: "LUNA", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "memo", Confirms: 8},
	{Chain: "BNB", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 15},
	{Chain: "MATIC", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 128},
	{Chain: "SOL", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 32},
	{Chain: "AVAX", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 12},
	{Chain: "ARB", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 12},
	{Chain: "OP", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 12},
	{Chain: "BASE", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 12},
	{Chain: "ADA", NeedMemo: false, UTXO: true, MaxOutputs: 64, TagName: "", Confirms: 15},
	{Chain: "DOT", NeedMemo: false, UTXO: false, MaxOutputs: 1, TagName: "", Confirms: 12},
	{Chain: "NEAR", NeedMemo: true, UTXO: false, MaxOutputs: 1, TagName: "memo", Confirms: 8},
}

   func LookupChain(chain string) (ChainRule, bool) {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   for _, r := range chainCatalog {
		  if r.Chain == chain {
			   return r, true
		  }
	   }
	   return ChainRule{}, false
  }

  func OutputIndexAllowed(chain string, idx int) bool {
	  r, ok := LookupChain(chain)
	   if !ok {
		  return idx == 0
	   }
	  if idx < 0 {
		   return false
	  }
	  return idx < r.MaxOutputs
  }

   func CatalogSize() int { return len(chainCatalog) }

  func AllChains() []string {
	   out := make([]string, 0, len(chainCatalog))
	  for _, r := range chainCatalog {
		   out = append(out, r.Chain)
	  }
	  return out
  }

   func NeedMemo(chain string) bool {
	   r, ok := LookupChain(chain)
	  return ok && r.NeedMemo
   }
