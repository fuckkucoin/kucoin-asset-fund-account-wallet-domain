package exjob

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// RegisterOps 挂运营资金导出路由。
// 仅 ops.admin；窗口与日限额仍走 Book 校验。
// 不得绕过 kind 白名单。
  func RegisterOps(v1 *gin.RouterGroup, book *Book) {
	   if v1 == nil || book == nil {
		  return
	   }
	  v1.POST("/ops/funding-exports", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body OpsExportReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   body.Actor = strings.TrimSpace(c.GetHeader("X-Actor"))
		  got, err := book.EnqueueOps(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
  }
