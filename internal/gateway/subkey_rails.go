package gateway

import (
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/subkey"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func registerSubKeyRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	   store := subkey.New(nil)
	  if rt != nil && rt.SubKeys != nil {
		   store = rt.SubKeys
	  }
	   opt := callerid.FromRuntime(false, "", "", "")
	   if rt != nil && rt.Cfg != nil {
		   opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	  v1.GET("/assets/sub-keys", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  if _, err := validate.UserID(uid); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   f := subkey.ListFilter{
			  Owner: uid, SubName: c.Query("subName"), APIKey: c.Query("apiKey"),
			   State: c.Query("state"), IncludeRev: c.Query("includeRevoked") == "1",
		  }
		   c.JSON(200, gin.H{
			   "user_id": uid, "items": store.ListFiltered(f),
			   "summary": store.OwnerSummary(uid), "host": config.HostAccountBook,
		  })
	  })
	   v1.GET("/assets/sub-keys/permission-summary", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  apiKey := strings.TrimSpace(c.Query("apiKey"))
		   if apiKey == "" {
			  c.JSON(400, gin.H{"error": "apiKey required"})
			   return
		   }
		  sum, err := store.SummarizeKey(uid, apiKey)
		   if err != nil {
			   c.JSON(404, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "summary": sum, "host": config.HostAccountBook})
	   })
	   v1.GET("/assets/sub-keys/audit", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   page, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
		  pageSize, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("pageSize", "30")))
		   q := subkey.AuditQuery{
			  Owner: uid, SubName: c.Query("subName"), APIKey: c.Query("apiKey"),
			  Action: c.Query("action"), Page: page, PageSize: pageSize,
		   }
		   c.JSON(200, gin.H{"user_id": uid, "page": store.ListAuditPage(q), "host": config.HostAccountBook})
	  })
	   v1.GET("/assets/sub-keys/quota", func(c *gin.Context) {
		   uid,
			  err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{
			   "user_id": uid, "quota": store.QuotaFor(uid, c.Query("subName")),
			  "host": config.HostAccountBook,
		   })
	  })
	  v1.GET("/assets/sub-keys/summary", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{
			  "user_id": uid, "summary": store.OwnerSummary(uid),
			   "catalog": subkey.PermissionCatalogView(), "host": config.HostAccountBook,
		  })
	  })
  }
