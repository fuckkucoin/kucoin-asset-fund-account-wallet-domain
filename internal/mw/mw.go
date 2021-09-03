package mw

import (
	"context"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func WriteTimeout(d time.Duration) gin.HandlerFunc {
	   if d <= 0 {
		   d = 8 * time.Second
	  }
	  return func(c *gin.Context) {
		   if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			   c.Next()
			  return
		   }
		  ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		   defer cancel()
		  c.Request = c.Request.WithContext(ctx)
		  c.Next()
	   }
   }

// RequireActor 写路径必须带 X-Actor。缺省用户身份不得碰冻账/审批。

   func RequireActor(acts ...perm.Action) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			   c.Next()
			   return
		   }
		   actor := perm.FromRequest(c.Request, "")
		  if strings.TrimSpace(actor.ID) == "" {
			  c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "X-Actor required"})
			   return
		   }
		   for _, a := range acts {
			   if err := actor.Check(a); err != nil {
				   c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
				  return
			   }
		  }
		   c.Next()
	  }
  }

  func IdempotencyKey(c *gin.Context) string {
	  if c == nil {
		   return ""
	  }
	  if k := strings.TrimSpace(c.GetHeader("Idempotency-Key")); k != "" {
		  return k
	   }
	   return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
  }
