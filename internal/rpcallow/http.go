package rpcallow

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   v1.POST("/rpcallow/check", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   URL string `json:"url"`
			  Chain string `json:"chain"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   var got Decision
		   var err error
		  if body.Chain != "" {
			  got, err = CheckChain(body.Chain, body.URL)
		   } else {
			   got, err = Check(body.URL)
		  }
		   if err != nil {
			  c.JSON(200, got)
			   return
		  }
		   c.JSON(200, got)
	   })
	  // 公网 RPC 目录运营面；先进程配置 CatalogFromConfig。
	  // 与 configs/app.yaml chains / FilterPublicRPCEndpoints 一致。
	  // 空目录不代表禁网；调用方仍须 CheckChain。
	   v1.GET("/rpcallow/catalog", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   cat := Catalog()
		   src := "env"
		  if rt != nil && rt.Cfg != nil {
			   cat = CatalogFromConfig(rt.Cfg)
			   src = "config"
		   }
		  c.JSON(200, gin.H{
			  "catalog": cat, "source": src, "count": len(cat),
			   "host": "query-gateway.kucoin.net",
		  })
	   })
  }
