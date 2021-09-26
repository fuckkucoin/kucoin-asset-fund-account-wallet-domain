// @FileName: http.go
// @Summary: notify 包内 http 实现。
// http.go
// 资金事件通知投递之 HTTP 接口层。
// CreateDate: 2021.09.16
// @Author: Cason
// @Created: 2021.09.02

package notify

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := NewService()
	  _ = rt
	   v1.GET("/notify/kinds", func(c *gin.Context) {
		  c.JSON(200, gin.H{"kinds": AllKinds(), "host": "notify.kucoin.net"})
	   })
	   v1.POST("/notify/enqueue", func(c *gin.Context) {
		  var body struct {
			  Key string `json:"key"`
			  Kind    string `json:"kind"`
			  Payload string `json:"payload"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if err := svc.Publish(c.Request.Context(), body.Key, body.Kind, body.Payload); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	   v1.POST("/notify/flush", func(c *gin.Context) {
		  c.JSON(200, svc.Flush(c.Request.Context()))
	  })
	  v1.GET("/notify/delivered", func(c *gin.Context) {
		   c.JSON(200, gin.H{"delivered": svc.Delivered(), "dead": svc.cons.DLQ().Len()})
	  })
  }
