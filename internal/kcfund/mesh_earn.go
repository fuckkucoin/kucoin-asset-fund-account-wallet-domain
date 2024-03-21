// 功能：理财 Earn 公开 REST 的 fundmesh 透传；mesh 失败时由 extras 回落本进程 earn 账本。
// 闪兑申购（convertType=STAKING）与运营 /v1/earn/* 不经本文件。

// 作者：Bronson
// 日期：2021.08.01

package kcfund

import (
	"bytes"
	      "io"
	"strings"
	  "github.com/gin-gonic/gin"
)

   func (a *API) forwardMeshEarn(c *gin.Context, method, apiPath string, body []byte) bool {
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
	     code, raw, err := g.Earn(c.Request.Context(), method, full, body, a.meshOpts(c))
	   if err != nil {
		   a.markMeshDown(c, err, apiPath)
		  return true
	      }
	   c.Data(code, "application/json", raw)
	     return true
  }
