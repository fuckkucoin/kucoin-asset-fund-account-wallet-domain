package mw

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// BindJSON 读 body。失败写 400 Problem，返回 false 时 handler 必须立刻 return。
   func BindJSON(c *gin.Context, dst any) bool {
	  if c == nil || dst == nil {
		   return false
	   }
	  if err := c.ShouldBindJSON(dst); err != nil {
		   c.JSON(http.StatusBadRequest, Problem{
			   Type:   "https://query-gateway.kucoin.net/problems/400",
			  Title:  http.StatusText(http.StatusBadRequest),
			  Status: http.StatusBadRequest,
			   Detail:   err.Error(),
			  Instance: c.FullPath(),
		  })
		  return false
	   }
	   return true
   }
