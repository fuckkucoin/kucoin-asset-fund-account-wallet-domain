package mw

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

const ctxActor = "aa_actor"

  func ActorFrom(c *gin.Context) perm.Actor {
	  if c == nil {
		  return perm.Actor{}
	   }
	   if v, ok := c.Get(ctxActor); ok {
		  if a, ok := v.(perm.Actor); ok {
			   return a
		   }
	   }
	  a := perm.FromRequest(c.Request, "")
	  c.Set(ctxActor, a)
	  return a
   }

// AttachActor 把请求头里的 actor 塞进 context，后面 handler 不必再解一次。

   func AttachActor() gin.HandlerFunc {
	  return func(c *gin.Context) {
		  _ = ActorFrom(c)
		   c.Next()
	   }
   }
