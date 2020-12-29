// 部分成交回调与冻结完整性检查留在网关（撮合 → ledger holdcat），不得经 convert 服务绕开幂等。

// Author: Bram
// Date: 2020.12.28

package kcfund

import (
	    "bytes"
	    "io"
	"strings"
	    "github.com/gin-gonic/gin"
)

// forwardMeshConvert 将公开 /api/* convert 路径原样转发至 convert.kucoin.net。
// 成功时直接写响应并返回 true；传输层失败返回 false，由调用方执行进程内 cvfund。
   func (a *API) forwardMeshConvert(c *gin.Context, method, apiPath string, body []byte) bool {
	   if a.tryProductOrchestration(c, method, apiPath, body) {
	  	   return true
	          }
	      if a.abortProductLocalFallback(c) {
	   	  return true
	     }
	       g := a.mesh()
	   if g == nil || c == nil || c.Request == nil {
	    	  return false
	      }
	    if body == nil && c.Request.Body != nil {
	   	  raw, err := io.ReadAll(c.Request.Body)
		  if err != nil {
			  return false
	    	   }
	  	   body = raw
	      	  c.Request.Body = io.NopCloser(bytes.NewReader(body))
	   }
	         full := apiPath
	   q := strings.TrimSpace(c.Request.URL.RawQuery)
	      if q != "" {
	    	   full += "?" + q
	   }
	       code, raw, err := g.Convert(c.Request.Context(), method, full, body, a.meshOpts(c))
	       if err != nil {
	       	   a.markMeshDown(c, err, apiPath)
	  	  return true
	     }
	     c.Data(code, "application/json", raw)
	  return true
  }
