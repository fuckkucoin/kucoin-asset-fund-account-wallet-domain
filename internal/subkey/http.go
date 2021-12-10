// 功能：account-book 内部子户 Key REST：/v1/sub-keys/*。
// 作者：TianpengLan
// 日期：2021.11.24

package subkey

import (
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
)

// HTTPMount 挂载 account-book 子户 Key 路由（避免 import app 循环依赖）。
type HTTPMount struct {
	Store *Store
	Opt callerid.Opt
}

// RegisterHTTP 挂载 /v1/sub-keys/*。

   func RegisterHTTP(v1 *gin.RouterGroup, m HTTPMount) {
	  store := m.Store
	   if store == nil {
		  store = New(nil)
	   }
	  opt := m.Opt
	   registerSubKeyRoutes(v1, store, opt)
   }

   func registerSubKeyRoutes(v1 *gin.RouterGroup,
	  store *Store, opt callerid.Opt) {
	  v1.GET("/sub-keys", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   f := ListFilter{
			   Owner: uid, SubName: c.Query("subName"), APIKey: c.Query("apiKey"),
			   State: c.Query("state"), IncludeRev: c.Query("includeRevoked") == "1",
		  }
		   c.JSON(200, gin.H{"items": store.ListFiltered(f), "host": config.HostAccountBook})
	   })
	   v1.GET("/sub-keys/permission-summary", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  sum, err := store.SummarizeKey(uid, c.Query("apiKey"))
		  if err != nil {
			  c.JSON(404, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"summary": sum, "host": config.HostAccountBook})
	  })
	  v1.GET("/sub-keys/audit", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   page, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
		  pageSize, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("pageSize", "50")))
		   q := AuditQuery{Owner: uid, SubName: c.Query("subName"), APIKey: c.Query("apiKey"), Action: c.Query("action"), Page: page, PageSize: pageSize}
		  c.JSON(200, gin.H{"page": store.ListAuditPage(q), "host": config.HostAccountBook})
	  })
	   v1.GET("/sub-keys/quota", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"quota": store.QuotaFor(uid, c.Query("subName")), "host": config.HostAccountBook})
	   })
	   v1.GET("/sub-keys/summary", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"summary": store.OwnerSummary(uid), "host": config.HostAccountBook})
	   })
	   v1.GET("/sub-keys/health", func(c *gin.Context) {
		   c.JSON(200, gin.H{"health": store.ReadHealth(), "host": config.HostAccountBook})
	   })
	  v1.POST("/sub-keys", func(c *gin.Context) {
		  var body CreateReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   body.Owner = uid
		   body.ClientIP = NormalizeClientIP(c.Request.RemoteAddr, c.GetHeader("X-Forwarded-For"))
		   created, err := store.Create(body)
		  if err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"data": created})
	   })
	  v1.PATCH("/sub-keys", func(c *gin.Context) {
		   var body ModifyReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   body.Owner = uid
		   body.ClientIP = NormalizeClientIP(c.Request.RemoteAddr, c.GetHeader("X-Forwarded-For"))
		   rec, err := store.ModifyReq(body)
		   if err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"data": rec})
	  })
	   v1.POST("/sub-keys/rotate", func(c *gin.Context) {
		   var body RotateReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   body.Owner = uid
		   body.ClientIP = NormalizeClientIP(c.Request.RemoteAddr, c.GetHeader("X-Forwarded-For"))
		   rot, err := store.Rotate(body.Owner, body.SubName, body.APIKey, body.Passphrase, body.ClientIP)
		   created := Created{}
		   if err == nil {
			   created = Created{Record: rot.Record,
				  APISecret: rot.APISecret, Passphrase: body.Passphrase}
		  }
		   if err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"data": created})
	   })
	   v1.POST("/sub-keys/revoke", func(c *gin.Context) {
		  var body RevokeReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   uid,
			   err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   body.Owner = uid
		  body.ClientIP = NormalizeClientIP(c.Request.RemoteAddr, c.GetHeader("X-Forwarded-For"))
		  rec, err := store.Revoke(body.Owner, body.SubName, body.APIKey, body.Passphrase, body.ClientIP)
		   if err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"data": rec})
	   })
	  v1.DELETE("/sub-keys", func(c *gin.Context) {
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   rec, err := store.Delete(uid, c.Query("subName"), c.Query("apiKey"), c.Query("passphrase"), NormalizeClientIP(c.Request.RemoteAddr, c.GetHeader("X-Forwarded-For")))
		  if err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"data": rec})
	  })
  }
