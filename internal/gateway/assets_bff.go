// @FileName: assets_bff.go
// @Summary: 资产读面 query 别名。
// @Author: Cason
// @Created: 2021.06.29

package gateway

import (
	       "strings"

	        "github.com/gin-gonic/gin"
)

  func queryFirst(c *gin.Context, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(c.Query(k)); v != "" {
		   return v
	   	}
	}
	     return ""
}

// quoteAndHideQuery 合并 quote、hide_small、hideSmall 等同义 query。
  func quoteAndHideQuery(c *gin.Context) (quote, hideSmall string) {
	  if c == nil {
	  return "", ""
	}
	      quote = queryFirst(c, "quote", "quoteCurrency", "balanceCurrency")
	hideSmall = queryFirst(c, "hide_small", "hideSmall")
	  return quote, hideSmall
}
