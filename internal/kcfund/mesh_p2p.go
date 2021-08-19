// @FileName: mesh_p2p.go
// @Summary: P2P 占用读面公开 REST 的 fundmesh 透传；mesh 失败时回落本进程 p2pgate.Funds。
// @Author: CainReed
// @Created: 2021.08.01

package kcfund

import (
	"bytes"
	"io"
	"net/url"
	"strings"
	  "github.com/gin-gonic/gin"
)

  func (a *API) forwardMeshP2P(c *gin.Context, method, apiPath string, body []byte) bool {
	  if a.tryProductOrchestration(c, method, apiPath, body) {
		   return true
	   }
	  if a.abortProductLocalFallback(c) {
		  return true
	      if body == nil && c.Request.Body != nil {
		   raw, err := io.ReadAll(c.Request.Body)
	    	   if err != nil {
	   		   return false
		   }
		  body = raw
	    	  c.Request.Body = io.NopCloser(bytes.NewReader(body))
	     }
	    full := apiPath
	   q := stripCallerQuery(c.Request.URL.RawQuery)
	   if q != "" {
		   full += "?" + q
	  }
	     code, raw, err := g.P2PFunding(c.Request.Context(), method, full, body, a.meshOpts(c))
	      if err != nil {
	  	   a.markMeshDown(c, err, apiPath)
		   return true
	  }
	   c.Data(code, "application/json", raw)
	  return true
  }

// stripCallerQuery 去掉查询串里的用户标识。下游只认 CallOpts 的 X-User-Id。
  func stripCallerQuery(raw string) string {
	     raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return ""
	   }
	  vals, err := url.ParseQuery(raw)
	    if err != nil {
		  return ""
	  }
	  for _, k := range []string{"user_id", "userId", "uid", "user"} {
	  	  vals.Del(k)
	     }
	     return vals.Encode()
  }
