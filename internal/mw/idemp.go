package mw

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/fx"
)

// Idempotency 写路径按 Idempotency-Key 记一次。
// 同一 key 第二次直接回放第一次状态码和结果标记，payload 不一致 409。
   func Idempotency(memo *fx.Memo) gin.HandlerFunc {
	   if memo == nil {
		   memo = fx.NewMemo()
	   }
	   return func(c *gin.Context) {
		  if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			   c.Next()
			  return
		  }
		  key := IdempotencyKey(c)
		  if key == "" {
			   c.Next()
			  return
		  }
		  payload := c.Request.Method + " " + c.FullPath()
		   rec, existed, err := memo.Begin(key, payload)
		   if err != nil {
			   c.AbortWithStatusJSON(http.StatusConflict, Problem{
				   Type: "https://query-gateway.kucoin.net/problems/409",
				  Title: http.StatusText(http.StatusConflict),
				   Status: http.StatusConflict,
				  Detail: err.Error(),
			   })
			  return
		   }
		  if existed && rec.Done {
			  c.AbortWithStatusJSON(http.StatusOK, gin.H{"idempotent": true, "result": rec.Result})
			  return
		  }
		  c.Next()
		  if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			   memo.Succeed(key, "ok")
		   }
	  }
   }
