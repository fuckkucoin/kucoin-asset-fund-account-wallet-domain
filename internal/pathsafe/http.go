package pathsafe

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, _ any) {
	  v1.POST("/pathsafe/check", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Path string `json:"path"`
			  Root string `json:"root"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Root != "" {
			   got, err := JoinRoot(body.Root, body.Path)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			  c.JSON(200, gin.H{"ok": true, "path": got, "host": config.HostIAMGateway})
			   return
		  }
		  got, err := Check(body.Path)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true, "path": got})
	  })
	  v1.POST("/pathsafe/url", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   URL string `json:"url"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := CheckURL(body.URL)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
   }
