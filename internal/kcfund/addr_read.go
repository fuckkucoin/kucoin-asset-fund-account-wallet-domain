package kcfund

import (
	   "strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/address"
	    "account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/destallow"
	    "github.com/gin-gonic/gin"
)

   func (a *API) bookSvc() *address.Service {
	   if a == nil || a.rt == nil {
	    	  return address.New(nil, nil)
	  }
	      svc := address.New(a.rt.Ledger,
		  a.rt.Wallet)
	   if !a.rt.Mem && a.rt.SQL != nil {
	   	  svc.BindSQL(a.rt.SQL)
	    }
	     var shared *addrpool.SharedMemoPool
	  if a.rt.SharedMemo != nil {
	  	  shared, _ = a.rt.SharedMemo.(*addrpool.SharedMemoPool)
	  }
	   if shared != nil {
	   	   svc.BindSharedMemo(shared, nil)
	   }
	   if a.rt.DepositAttr != nil {
	  	  if attr, ok := a.rt.DepositAttr.(*deposit.Attributor); ok {
			  svc.BindSharedMemo(svc.SharedPool(),
				   deposit.AsMemoBinder(attr))
		  } else if binder, ok := a.rt.DepositAttr.(addrpool.MemoBinder); ok {
			   svc.BindSharedMemo(svc.SharedPool(), binder)
		  }
	   }
	     if a.rt.KV != nil {
		  svc.BindKV(a.rt.KV)
	    	  svc.BindRecycle(addrpool.NewRecycleBook(a.rt.KV, 24*time.Hour))
	   }
	   return svc
   }

  func (a *API) listWithdrawAddresses(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	      if !okU {
		  return
	   }
	  svc := a.bookSvc()
	  chain := strings.TrimSpace(bffParam(c, "chain"))
	       pageRaw := strings.TrimSpace(bffParam(c, "currentPage", "page"))
	   sizeRaw := strings.TrimSpace(bffParam(c, "pageSize"))
	     addr := strings.TrimSpace(bffParam(c, "address"))
	  cur := strings.TrimSpace(bffParam(c, "currency"))
	     if cur == "" {
		  cur = strings.TrimSpace(bffParam(c, "asset"))
	      }
	   if pageRaw == "" && sizeRaw == "" && chain == "" && addr == "" && cur == "" {
		  list, err := svc.ListBook(c.Request.Context(), uid)
		   if err != nil {
	   		  fail(c, 400, err.Error())
	  		  return
		   }
	  	   ok(c, list)
		  return
	       }
	       page, _ := strconv.Atoi(pageRaw)
	  size, _ := strconv.Atoi(sizeRaw)
	   got, err := svc.ListBookPage(c.Request.Context(), address.BookQuery{
		   UserID: uid, Chain: chain, Address: addr, Currency: cur, Asset: cur,
	    	  Tag: strings.TrimSpace(bffParam(c, "tag")), Page: page, PageSize: size,
	     })
	  if err != nil {
		   fail(c, 400, err.Error())
		   return
	  }
	      ok(c, gin.H{
		   "items": got.Entries, "currentPage": got.Page, "pageSize": got.PageSize,
		   "totalNum": got.Total, "totalPage": got.TotalPage, "currency": got.Currency,
	    })
  }

  func (a *API) getWithdrawAddress(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	      if !okU {
	    	  return
	      }
	  view, err := a.bookSvc().InspectBook(c.Request.Context(), uid, bffParam(c, "chain"), bffParam(c, "address"))
	   if err != nil {
	    	  fail(c, 400, err.Error())
		   return
	      }
	    if !view.Found {
	   	   fail(c, 404, "address book entry not found")
		   return
	      }
	   ok(c, view)
  }

  func (a *API) checkWithdrawDest(c *gin.Context) {
	       uid, okU := a.requireUser(c)
	       if !okU {
		   return
	      }
	   chain := strings.TrimSpace(bffParam(c, "chain"))
	   addr := strings.TrimSpace(bffParam(c, "address"))
	   var dest *destallow.Book
	   if a.rt != nil {
		   dest = a.rt.DestAllow
	    }
	   view, err := a.bookSvc().InspectAllow(c.Request.Context(), uid, chain, addr, dest, a.surf())
	   if err != nil {
	    	  fail(c, 400, err.Error())
	  	   return
	   }
	   ok(c, withdrawAddressCheckJSON(chain, view))
  }

  func (a *API) getWhitelistPref(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	     if !okU {
	  	  return
	   }
	  enabled := a.surf().WhitelistOnly(c.Request.Context(), uid)
	   ok(c, gin.H{"user_id": uid, "enabled": enabled, "whitelist_only": enabled})
   }

  func (a *API) listAssignedDeposit(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	    if !okU {
		  return
	  }
	  chain := strings.ToUpper(strings.TrimSpace(c.Query("chain")))
	  asset := strings.TrimSpace(c.Query("currency"))
	   if asset == "" {
	   	  asset = strings.TrimSpace(c.Query("asset"))
	   }
	     svc := a.bookSvc()
	     pageRaw := strings.TrimSpace(c.Query("currentPage"))
	   sizeRaw := strings.TrimSpace(c.Query("pageSize"))
	  if pageRaw != "" || sizeRaw != "" {
	   	   page, _ := strconv.Atoi(pageRaw)
		   size, _ := strconv.Atoi(sizeRaw)
	  	   items,
			  total, pageOut, sizeOut, pages := svc.InspectDepositPage(uid, chain, asset, page, size)
	    	   ok(c, gin.H{"items": items, "currentPage": pageOut, "pageSize": sizeOut, "totalNum": total, "totalPage": pages})
	    	   return
	    }
	     if chain != "" {
	    	  ok(c, svc.InspectDeposit(uid, chain, asset, ""))
	   	  return
	       }
	   ok(c, svc.InspectDepositList(uid, "", asset, ""))
  }

  func (a *API) getAssignedDeposit(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	   if !okU {
	   	  return
	       }
	  chain := strings.TrimSpace(c.Query("chain"))
	   addr := strings.TrimSpace(c.Query("address"))
	  if addr != "" && chain == "" {
	  	  view := a.bookSvc().InspectDepositByAddress(addr)
		  if view.Found && view.UserID != uid {
			   fail(c, 404, "deposit address not found")
	   		  return
		   }
	   	   ok(c, view)
	  	   return
	      }
	      if chain == "" {
	   	   fail(c, 400, "chain required")
	    	  return
	  }
	   asset := strings.TrimSpace(c.Query("currency"))
	   if asset == "" {
		   asset = strings.TrimSpace(c.Query("asset"))
	  }
	   ok(c, a.bookSvc().InspectDeposit(uid, chain, asset, ""))
   }

   func (a *API) listDestAllow(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	  if !okU {
	    	  return
	   }
	      var book *destallow.Book
	  if a.rt != nil {
		  book = a.rt.DestAllow
	       }
	   page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
	      size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
	  items, total, pageOut, sizeOut, pages := book.ListPage(uid, c.Query("chain"), page, size, c.Query("label"))
	      ok(c, gin.H{
		   "items": book.DecorateEntries(items), "currentPage": pageOut, "pageSize": sizeOut,
		  "totalNum": total, "totalPage": pages,
	  })
  }

   func (a *API) exportDestAllow(c *gin.Context) {
	       uid, okU := a.requireUser(c)
	     if !okU {
	    	  return
	      }
	  var book *destallow.Book
	      if a.rt != nil {
		  book = a.rt.DestAllow
	   }
	     if book == nil {
	    	   ok(c, gin.H{"items": []destallow.EntryView{}})
	    	  return
	  }
	      ok(c, gin.H{"items": book.ExportFilter(uid, c.Query("chain"), c.Query("label"))})
  }

  func (a *API) listWithdrawAudit(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	     if !okU {
	    	  return
	    }
	    limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	       list, err := a.bookSvc().ListBookAudit(c.Request.Context(), uid, c.Query("chain"), c.Query("address"), limit)
	  if err != nil {
	    	   fail(c, 400, err.Error())
		  return
	    }
	     ok(c, gin.H{"audits": list})
   }

  func (a *API) exportWithdrawAddresses(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	  if !okU {
	    	  return
	      }
	   body, ctype, err := a.bookSvc().ExportBook(c.Request.Context(), address.BookQuery{
	   	   UserID: uid, Chain: c.Query("chain"), Address: c.Query("address"),
	    	  Currency: c.Query("currency"), Asset: firstNonEmpty(c.Query("asset"), c.Query("currency")),
		  Tag: c.Query("tag"),
	   }, c.Query("format"))
	     if err != nil {
		   fail(c, 400, err.Error())
		  return
	   }
	    c.Data(200, ctype, body)
   }

  func (a *API) listAddressDups(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		   return
	  }
	      dups, err := a.bookSvc().DetectCrossChainDups(c.Request.Context(), uid)
	  if err != nil {
		   fail(c, 400, err.Error())
		   return
	   }
	   ok(c, gin.H{"cross_chain_dups": dups})
  }

  func (a *API) getAddressQuota(c *gin.Context) {
	    uid, okU := a.requireUser(c)
	  if !okU {
	    	   return
	  }
	      view,
		  err := a.bookSvc().InspectQuota(c.Request.Context(), uid)
	      if err != nil {
	   	   fail(c, 400, err.Error())
	    	   return
	  }
	   ok(c, view)
   }
