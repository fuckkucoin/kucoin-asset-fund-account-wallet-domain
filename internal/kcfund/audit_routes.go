package kcfund

import (
	"strings"

	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/subkey"
	    "github.com/gin-gonic/gin"
)

  func (a *API) transferInOpen(c *gin.Context, currency string) bool {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	  if currency == "" || c == nil {
	   	return false
	  }
	for _, code := range a.currencyCodes(c.Request.Context()) {
		if code == currency {
	      		return true
		}
	}
	return false
}

func (a *API) getMarginConfig(c *gin.Context) {
	codes := a.utaBook().Borrowable()
	items := make([]gin.H, 0, len(codes))
	for _, code := range codes {
		lim, err := a.utaBook().RequireInterestLimits(code)
	   	if err != nil {
			fail(c, 503, err.Error())
			return
		}
	      	items = append(items, gin.H{
	   		"currency": code, "borrowLimit": lim.BorrowLimitTotal,
	   		"borrowLimitHold": lim.BorrowLimitHold, "interestFreeLimit": lim.InterestFreeBorrowLimit,
	   		"rateHourly": lim.CurrentRateHourly, "rateDaily": lim.CurrentRateDaily,
	   	})
	      }
	ok(c, items)
}

  func (a *API) getMarginSymbols(c *gin.Context) {
	   ok(c, gin.H{"items": a.isoBook().BookedSymbols()})
}

func (a *API) getIsolatedSymbols(c *gin.Context) {
	    ok(c, gin.H{"items": a.isoBook().BookedSymbols()})
}

  func (a *API) utaCollateralCatalog(c *gin.Context) {
	ok(c, a.utaBook().CollateralCatalogView())
}

func (a *API) listMasterAPIKeys(c *gin.Context) {
	uid, okU := a.requireUser(c)
	    if !okU {
		return
	    }
	  store := a.keys()
	if store == nil {
	  	ok(c, []subkey.Record{})
		return
	}
	  rows := store.List(uid, "", "")
	   out := make([]subkey.Record, 0)
	for _, r := range rows {
	  	if r.IsMaster || strings.TrimSpace(r.SubName) == "" {
			out = append(out, r)
		}
	    }
	   ok(c, out)
}

  func (a *API) listConvertCurrencies(c *gin.Context) {
	if a == nil || a.rt == nil || a.rt.SQL == nil || a.rt.SQL.DB == nil {
	    	if a != nil && a.rt != nil && a.rt.Mem {
			ok(c, convertCurrenciesJSON([]sqlrepo.ListingAsset{}))
			return
		}
	   	fail(c, 503, "listing unavailable")
		return
	    }
	rows, err := sqlrepo.NewListing(a.rt.SQL).ListAssets(c.Request.Context())
	  if err != nil {
	   	fail(c, 503, err.Error())
	    	return
	    }
	    if rows == nil {
	    	rows = []sqlrepo.ListingAsset{}
	}
	ok(c, convertCurrenciesJSON(rows))
}
