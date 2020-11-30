package address

import (
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func registerRead(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  if v1 == nil || svc == nil {
		  return
	  }
	   id := readID(rt)
	   v1.GET("/addresses/book/item", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  e, ok, err := svc.GetBook(c.Request.Context(), user, c.Query("chain"), c.Query("address"))
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   if !ok {
			  c.JSON(404, gin.H{"found": false, "error": "address book entry not found"})
			   return
		   }
		   c.JSON(200, gin.H{"found": true, "entry": svc.decorate(c.Request.Context(), e)})
	  })
	   v1.GET("/addresses/book/dups", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   dups, err := svc.DetectCrossChainDups(c.Request.Context(), user)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"user_id": user, "cross_chain_dups": dups})
	   })
	   v1.GET("/addresses/book/admin", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   uid := strings.TrimSpace(c.Query("user_id"))
		  if uid == "" {
			  c.JSON(400, gin.H{"error": "user_id required"})
			  return
		  }
		   page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
		  size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
		   view, err := svc.InspectBookAdmin(c.Request.Context(), uid, c.Query("chain"), firstNonEmpty(c.Query("asset"), c.Query("currency")), c.Query("tag"), page, size,
			   strings.EqualFold(c.Query("include_disabled"), "true") || c.Query("include_disabled") == "1")
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, view)
	  })
	  v1.GET("/addresses/book/inspect", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  view, err := svc.InspectBook(c.Request.Context(), user, c.Query("chain"), c.Query("address"))
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, view)
	  })
	   v1.GET("/addresses/book/export", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   body, ctype, err := svc.ExportBook(c.Request.Context(), BookQuery{
			   UserID: user, Chain: c.Query("chain"), Address: c.Query("address"),
			  Currency: c.Query("currency"), Asset: firstNonEmpty(c.Query("asset"), c.Query("currency")),
			   Tag: c.Query("tag"), IncludeDisabled: strings.EqualFold(c.Query("include_disabled"), "true") || c.Query("include_disabled") == "1",
		   }, c.Query("format"))
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.Data(200, ctype, body)
	   })
	  v1.PUT("/addresses/book/quota", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   UserID string `json:"user_id"`
			   Limit  int    `json:"limit"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		   if err := svc.PersistQuota(c.Request.Context(), body.UserID, body.Limit); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   view, err := svc.InspectQuota(c.Request.Context(), body.UserID)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, view)
	  })
	   v1.GET("/addresses/book/anomaly", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   view,
			  err := svc.InspectAnomaly(c.Request.Context(), user)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, view)
	  })
	   v1.GET("/addresses/book/quota", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   view, err := svc.InspectQuota(c.Request.Context(), user)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, view)
	   })
	   v1.GET("/addresses/book/count", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   n, err := svc.BookCount(c.Request.Context(), user, c.Query("chain"))
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"user_id": user, "chain": c.Query("chain"), "totalNum": n})
	   })
	  v1.GET("/addresses/assigned", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  chain := strings.ToUpper(strings.TrimSpace(c.Query("chain")))
		   asset := strings.TrimSpace(c.Query("asset"))
		  if asset == "" {
			   asset = strings.TrimSpace(c.Query("currency"))
		  }
		  if strings.TrimSpace(c.Query("address")) != "" && chain == "" {
			   view := svc.InspectDepositByAddress(c.Query("address"))
			  if view.Found && view.UserID != user {
				   c.JSON(403, gin.H{"error": "forbidden"})
				  return
			   }
			   c.JSON(200, view)
			   return
		   }
		   pageRaw := strings.TrimSpace(c.Query("currentPage"))
		  sizeRaw := strings.TrimSpace(c.Query("pageSize"))
		  if pageRaw != "" || sizeRaw != "" {
			   page, _ := strconv.Atoi(pageRaw)
			  size, _ := strconv.Atoi(sizeRaw)
			  items, total, pageOut, sizeOut, pages := svc.InspectDepositPage(user, chain, asset, page, size)
			  c.JSON(200, gin.H{"items": items, "currentPage": pageOut, "pageSize": sizeOut, "totalNum": total, "totalPage": pages})
			   return
		   }
		   if chain != "" {
			   c.JSON(200, svc.InspectDeposit(user, chain, asset, ""))
			   return
		   }
		  c.JSON(200, gin.H{"items": svc.InspectDepositList(user, "", asset, "")})
	  })
	   var pref destallow.PrefReader
	  var book *destallow.Book
	   if rt != nil && rt.FundSurf != nil {
		   pref = rt.FundSurf
	  }
	   if rt != nil {
		   book = rt.DestAllow
	  }
	   destallow.Register(v1, book, pref, id)
	   registerWrite(v1, rt, svc)
   }

  func readID(rt *app.Runtime) callerid.Opt {
	   if rt == nil || rt.Cfg == nil {
		   return callerid.Opt{}
	   }
	   return callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
   }
