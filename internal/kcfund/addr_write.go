package kcfund

import (
	"io"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/address"
	    "account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	   "account-assets.kucoin.net/funding/internal/ledger"
	"github.com/gin-gonic/gin"
)

   func (a *API) updateWithdrawAddress(c *gin.Context) {
	  var body struct {
		   Chain   string `json:"chain"`
	  	  Address string `json:"address"`
	    	  Remark string `json:"remark"`
	    	  Memo   string `json:"memo"`
		   Asset  string `json:"asset"`
	    }
	  if err := c.ShouldBindJSON(&body); err != nil {
	   	   fail(c, 400, err.Error())
		   return
	       }
	   uid, okU := a.requireUser(c)
	      if !okU {
		   return
	       }
	       e, err := a.bookSvc().UpdateBook(c.Request.Context(), address.BookWrite{
		  UserID: uid, Chain: body.Chain, Address: body.Address, Label: body.Remark, Memo: body.Memo, Asset: body.Asset, Actor: uid,
	      })
	      if err != nil {
		   fail(c, 400, err.Error())
		  return
	  }
	    a.syncBookDestAllow(e,
		   e.Disabled)
	    ok(c, e)
  }

  func (a *API) disableWithdrawAddress(c *gin.Context) {
	   var body struct {
	  	  Chain   string `json:"chain"`
		  Address string `json:"address"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
	  	  fail(c, 400, err.Error())
		   return
	    }
	     uid, okU := a.requireUser(c)
	    if !okU {
		  return
	     }
	   e, err := a.bookSvc().DisableBook(c.Request.Context(), uid, body.Chain, body.Address, uid)
	  if err != nil {
		   fail(c, 400, err.Error())
		  return
	   }
	   a.syncBookDestAllow(e, true)
	   ok(c, e)
   }

   func (a *API) restoreWithdrawAddress(c *gin.Context) {
	      var body struct {
	    	   Chain   string `json:"chain"`
	    	   Address string `json:"address"`
	   }
	     if err := c.ShouldBindJSON(&body); err != nil {
	    	  fail(c, 400, err.Error())
	   	   return
	      }
	   uid, okU := a.requireUser(c)
	   if !okU {
		  return
	      }
	     e, err := a.bookSvc().RestoreBook(c.Request.Context(), uid, body.Chain, body.Address, uid)
	   if err != nil {
		   fail(c, 400, err.Error())
	   	  return
	  }
	     a.syncBookDestAllow(e, false)
	     ok(c, e)
  }

  func (a *API) enableWithdrawAddress(c *gin.Context) {
	    var body struct {
	   	   Chain string `json:"chain"`
	   	  Address string `json:"address"`
	   }
	      if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		  return
	    }
	    uid, okU := a.requireUser(c)
	     if !okU {
	   	   return
	   }
	       e, err := a.bookSvc().EnableBook(c.Request.Context(), uid, body.Chain, body.Address, uid)
	       if err != nil {
		  fail(c, 400, err.Error())
	  	  return
	   }
	     a.syncBookDestAllow(e, false)
	      ok(c, e)
   }

   func (a *API) addDestAllow(c *gin.Context) {
	     var body struct {
	  	  Chain   string `json:"chain"`
	    	  Address string `json:"address"`
		   Label   string `json:"label"`
	   }
	     if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
		   return
	  }
	     uid, okU := a.requireUser(c)
	  if !okU {
	    	   return
	  }
	     if a.rt == nil || a.rt.DestAllow == nil {
		   fail(c, 503, "destination allowlist unavailable")
		  return
	     }
	   e, err := a.rt.DestAllow.AddExact(uid, body.Chain, body.Address, body.Label)
	       if err != nil {
	   	   fail(c, 400, err.Error())
		  return
	   }
	     ok(c, a.rt.DestAllow.ViewOf(e))
   }

   func (a *API) deleteDestAllow(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	  if !okU {
		   return
	     }
	  if a.rt == nil || a.rt.DestAllow == nil {
	    	  fail(c, 503, "destination allowlist unavailable")
		   return
	  }
	    if err := a.rt.DestAllow.RemoveExact(uid, c.Query("chain"), c.Query("address")); err != nil {
	   	   fail(c, 400, err.Error())
		  return
	     }
	  ok(c, gin.H{"ok": true})
  }

  func (a *API) setWhitelistPref(c *gin.Context) {
	       var body struct {
	    	  Enabled bool `json:"enabled"`
	       }
	   enabled := body.Enabled
	   switch err := bindJSONOrEmpty(c, &body); err {
	   case nil:
		   enabled = body.Enabled
	   case io.EOF:
	    	   if b, ok := bffParamBool(c, "enabled"); ok {
			   enabled = b
		   } else {
	  		   fail(c, 400, "enabled required")
			   return
		   }
	   default:
	   	   fail(c, 400, err.Error())
	    	   return
	   }
	   uid, okU := a.requireUser(c)
	     if !okU {
		   return
	  }
	   if err := destallow.SetPref(c.Request.Context(), a.surf(), uid, enabled); err != nil {
		   fail(c, 400, err.Error())
	   	   return
	  }
	  ok(c, gin.H{"user_id": uid, "enabled": enabled, "whitelist_only": enabled})
   }

  func (a *API) assignDepositAddress(c *gin.Context) {
	  var body struct {
		  Currency string `json:"currency"`
	   	  Chain    string `json:"chain"`
	  	  Memo     string `json:"memo"`
	  }
	       if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
		  return
	      }
	  uid, okU := a.requireUser(c)
	     if !okU {
	   	   return
	     }
	  memo := strings.TrimSpace(body.Memo)
	   if memo == "" {
		  memo = fundsurf.MemoFor(uid, body.Chain)
	     }
	     view, err := a.bookSvc().AssignDeposit(uid, body.Currency, body.Chain, memo)
	       if err != nil {
		   fail(c, 400, err.Error())
		   return
	   }
	   ok(c, view)
  }

   func (a *API) releaseDepositAddress(c *gin.Context) {
	     var body struct {
		   Chain string `json:"chain"`
	  }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		   return
	    }
	     uid, okU := a.requireUser(c)
	      if !okU {
	  	   return
	  }
	       if err := a.bookSvc().ReleaseDeposit(uid, body.Chain); err != nil {
		  fail(c, 400, err.Error())
		  return
	     }
	   ok(c, gin.H{"ok": true, "chain": strings.ToUpper(strings.TrimSpace(body.Chain))})
   }

  func (a *API) recycleDepositAddress(c *gin.Context) {
	    var body struct {
	   	   Chain string `json:"chain"`
	  	   Address string `json:"address"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		  return
	   }
	   uid, okU := a.requireUser(c)
	      if !okU {
		  return
	   }
	  got, err := a.bookSvc().RecycleDeposit(c.Request.Context(), uid, body.Chain, body.Address)
	  if err != nil {
		   fail(c, 400, err.Error())
	   	   return
	  }
	      ok(c, got)
  }

   func (a *API) addWithdrawAddress(c *gin.Context) {
	     var body struct {
	    	  Chain   string `json:"chain"`
		  Address string `json:"address"`
	   	   Remark  string `json:"remark"`
		  Memo    string `json:"memo"`
		   Asset string `json:"asset"`
	   }
	      if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
		  fail(c, 400, err.Error())
	    	  return
	   }
	      body.Chain = strings.TrimSpace(firstNonEmpty(body.Chain, bffParam(c, "chain")))
	     body.Address = strings.TrimSpace(firstNonEmpty(body.Address, bffParam(c, "address")))
	   body.Remark = strings.TrimSpace(firstNonEmpty(body.Remark, bffParam(c, "remark")))
	   body.Memo = strings.TrimSpace(firstNonEmpty(body.Memo, bffParam(c, "memo")))
	       body.Asset = strings.TrimSpace(firstNonEmpty(body.Asset, bffParam(c, "asset")))
	      if body.Chain == "" || body.Address == "" {
		  fail(c, 400, "chain and address required")
	  	  return
	   }
	   uid, okU := a.requireUser(c)
	  if !okU {
	  	   return
	   }
	   e, err := a.bookSvc().AddBookWrite(c.Request.Context(), address.BookWrite{
		  UserID: uid, Chain: body.Chain, Address: body.Address, Label: body.Remark,
	  	  Nickname: body.Remark, Memo: body.Memo, Asset: body.Asset, Actor: uid,
	    	  IdemKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	   })
	   if err != nil {
		  fail(c, 400, err.Error())
		  return
	   }
	      a.syncBookDestAllow(e, false)
	   ok(c, e)
   }

  func (a *API) deleteWithdrawAddress(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	   if !okU {
	    	  return
	   }
	  chain := bffParam(c, "chain")
	     addr := bffParam(c, "address")
	  e, err := a.bookSvc().DisableBook(c.Request.Context(), uid, chain, addr, uid)
	     if err != nil {
		   fail(c, 400, err.Error())
		   return
	     }
	       a.syncBookDestAllow(e, true)
	     ok(c, gin.H{"ok": true, "soft_deleted": true})
   }

  func (a *API) importDestAllow(c *gin.Context) {
	     var body struct {
	   	  Rows []destallow.ImportRow `json:"rows"`
	  }
	  if err := c.ShouldBindJSON(&body); err != nil {
	   	   fail(c, 400, err.Error())
		  return
	   }
	   uid, okU := a.requireUser(c)
	  if !okU {
		   return
	   }
	  if a.rt == nil || a.rt.DestAllow == nil {
		   fail(c, 503, "destination allowlist unavailable")
		   return
	   }
	     for i := range body.Rows {
	    	  if strings.TrimSpace(body.Rows[i].UserID) == "" {
	  		  body.Rows[i].UserID = uid
		   }
	  }
	   rep, err := a.rt.DestAllow.ImportExact(body.Rows, uid)
	    if err != nil {
	   	   fail(c, 400, err.Error())
		   return
	     }
	      ok(c, rep)
   }

  func (a *API) reclaimDepositAddress(c *gin.Context) {
	     var body struct {
		  Chain   string `json:"chain"`
		  Address string `json:"address"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
	  	  fail(c, 400, err.Error())
	    	   return
	    }
	     uid, okU := a.requireUser(c)
	     if !okU {
	   	  return
	      }
	   got,
		  err := a.bookSvc().ReclaimDeposit(c.Request.Context(), uid, body.Chain, body.Address)
	     if err != nil {
	   	  fail(c, 400, err.Error())
	    	  return
	   }
	  ok(c, got)
  }

  func (a *API) assignDepositFair(c *gin.Context) {
	     var body struct {
	    	   Currency string   `json:"currency"`
		  Chain    string   `json:"chain"`
		  Memo     string   `json:"memo"`
		  Chains   []string `json:"chains"`
	      }
	      if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
	   	  return
	  }
	     uid, okU := a.requireUser(c)
	   if !okU {
		   return
	   }
	       memo := strings.TrimSpace(body.Memo)
	    if memo == "" {
	    	  memo = fundsurf.MemoFor(uid, body.Chain)
	     }
	   view, err := a.bookSvc().AssignDepositFair(uid, body.Currency, body.Chain, memo, body.Chains)
	       if err != nil {
		   fail(c, 400, err.Error())
	   	  return
	  }
	  ok(c, view)
   }

   func (a *API) batchWithdrawAddresses(c *gin.Context) {
	      var body struct {
		  Rows []address.BookWrite `json:"rows"`
	    }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		   return
	   }
	  uid, okU := a.requireUser(c)
	    if !okU {
		   return
	   }
	    for i := range body.Rows {
	   	  if strings.TrimSpace(body.Rows[i].UserID) == "" {
	  		   body.Rows[i].UserID = uid
		   }
		   body.Rows[i].Actor = uid
	  }
	  rep, err := a.bookSvc().AddBookBatch(c.Request.Context(), body.Rows)
	   if err != nil {
	  	   fail(c, 400, err.Error())
		  return
	   }
	       for _, e := range rep.Accepted {
		   a.syncBookDestAllow(e.AddressBookEntry, false)
	  }
	  ok(c, rep)
  }

   func (a *API) addDestAllowTemp(c *gin.Context) {
	  var body struct {
		  Chain   string `json:"chain"`
	  	   Address string `json:"address"`
		  Label string `json:"label"`
	  	  RiskTag string `json:"risk_tag"`
		  TTLSec int64 `json:"ttl_sec"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
	  	  fail(c, 400, err.Error())
	   	  return
	    }
	   uid, okU := a.requireUser(c)
	    if !okU {
		   return
	  }
	    if a.rt == nil || a.rt.DestAllow == nil {
		   fail(c, 503, "destination allowlist unavailable")
		   return
	   }
	     e, err := a.rt.DestAllow.AddTempExact(uid, body.Chain, body.Address, body.Label, body.RiskTag, time.Duration(body.TTLSec)*time.Second)
	       if err != nil {
	  	   fail(c, 400, err.Error())
	    	  return
	     }
	   ok(c, a.rt.DestAllow.ViewOf(e))
  }

  func (a *API) precheckDestAllowBatch(c *gin.Context) {
	   var body struct {
		   Rows []destallow.PrecheckRow `json:"rows"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
	    	   return
	     }
	      uid, okU := a.requireUser(c)
	       if !okU {
	    	  return
	   }
	      var book *destallow.Book
	   if a.rt != nil {
		  book = a.rt.DestAllow
	     }
	   ok(c, gin.H{"items": destallow.PrecheckBatch(c.Request.Context(), book, a.surf(), uid, body.Rows), "dry_run": true})
  }

  func (a *API) assignDepositSticky(c *gin.Context) {
	     var body struct {
	    	  Currency string `json:"currency"`
	   	   Chain    string `json:"chain"`
	   	  Memo     string `json:"memo"`
	  }
	  if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
	  	  return
	     }
	     uid, okU := a.requireUser(c)
	      if !okU {
		  return
	  }
	   memo := strings.TrimSpace(body.Memo)
	   if memo == "" {
	    	   memo = fundsurf.MemoFor(uid, body.Chain)
	    }
	   view,
	  	   err := a.bookSvc().AssignDepositSticky(uid, body.Currency, body.Chain, memo)
	       if err != nil {
	   	  fail(c, 400, err.Error())
	    	  return
	     }
	      ok(c, view)
   }

  func (a *API) syncBookDestAllow(e ledger.AddressBookEntry, drop bool) {
	   if a == nil || a.rt == nil || a.rt.DestAllow == nil {
	   	  return
	    }
	  if drop || e.Disabled {
	   	  _ = a.rt.DestAllow.RemoveExact(e.UserID, e.Chain, e.Address)
	  	  return
	     }
	   _, _ = a.rt.DestAllow.AddExact(e.UserID, e.Chain, e.Address, e.Label)
   }
