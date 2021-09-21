package mw

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/gin-gonic/gin"
)

const HeaderRequestID = "X-Request-ID"

// RequestID 透传或补一个请求号，写回响应头。不参与鉴权。
   func RequestID() gin.HandlerFunc {
	   return func(c *gin.Context) {
		   id := c.GetHeader(HeaderRequestID)
		   if id == "" {
			  id = newRequestID()
		  }
		   c.Writer.Header().Set(HeaderRequestID, id)
		  c.Set("request_id", id)
		  c.Next()
	   }
  }

  func RequestIDFrom(c *gin.Context) string {
	   if c == nil {
		  return ""
	  }
	  if v, ok := c.Get("request_id"); ok {
		  if s, ok := v.(string); ok {
			   return s
		  }
	   }
	   return c.GetHeader(HeaderRequestID)
  }

   func newRequestID() string {
	  var b [16]byte
	   if _, err := rand.Read(b[:]); err != nil {
		  return "req-fallback"
	   }
	  return hex.EncodeToString(b[:])
   }
