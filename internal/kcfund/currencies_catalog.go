// 功能：币种精度与上市表读取，供仍留在 Go 的保证金币种接口使用。

// 作者：Cason
// 日期：2021.08.01

package kcfund

import (
	"context"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

  func (a *API) currencyCodes(ctx context.Context) []string {
	  seen := map[string]struct{}{}
	   if a != nil && a.rt != nil && a.rt.Cfg != nil {
		   for chain, spec := range a.rt.Cfg.Chains {
			  chain = strings.ToUpper(strings.TrimSpace(chain))
			  if ticker, okN := nativeTicker(chain); okN {
				  seen[ticker] = struct{}{}
			  }
			  for cur := range spec.Tokens {
				   cur = strings.ToUpper(strings.TrimSpace(cur))
				   if cur != "" {
					  seen[cur] = struct{}{}
				   }
			   }
		   }
	  }
	  for sym := range a.listingBySymbol(ctx) {
		  if sym != "" {
			  seen[sym] = struct{}{}
		  }
	  }
	   out := make([]string, 0, len(seen))
	  for c := range seen {
		  out = append(out, c)
	   }
	  sort.Strings(out)
	   return out
  }

  func nativeTicker(chain string) (string, bool) {
	  switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "ETH", "ARBITRUM", "OPTIMISM", "BASE":
		  return "ETH", true
	  case "BTC":
		  return "BTC", true
	  case "SOL":
		  return "SOL", true
	  case "TRON":
		  return "TRX", true
	  case "TON":
		  return "TON", true
	  case "POLYGON":
		  return "POL", true
	  case "XRP", "XLM", "ATOM", "EOS":
		  return strings.ToUpper(strings.TrimSpace(chain)), true
	  default:
		  return "", false
	  }
  }

  func (a *API) listingBySymbol(ctx context.Context) map[string]sqlrepo.ListingAsset {
	  out := map[string]sqlrepo.ListingAsset{}
	  if a == nil || a.rt == nil || a.rt.Mem || a.rt.SQL == nil || ctx == nil {
		  return out
	  }
	  rows, err := sqlrepo.NewListing(a.rt.SQL).ListAssets(ctx)
	  if err != nil {
		  return out
	  }
	  for _, row := range rows {
		  sym := strings.ToUpper(strings.TrimSpace(row.Symbol))
		  if sym != "" {
			  out[sym] = row
		  }
	  }
	  return out
  }

  func (a *API) precisionOf(code string) int {
	  code = strings.ToUpper(strings.TrimSpace(code))
	  if a == nil {
		  return 0
	  }
	  if row, okL := a.listingBySymbol(context.Background())[code]; okL && row.Decimals > 0 {
		  return row.Decimals
	  }
	  if a.rt != nil && a.rt.Cfg != nil {
		  for _, spec := range a.rt.Cfg.Chains {
			  if tok, okT := spec.Tokens[code]; okT && tok.Decimals > 0 {
				  return tok.Decimals
			  }
		  }
	  }
	  return 0
  }
