// @FileName: mesh_subkey.go
// @Summary: 子户 API Key 公开 REST 的 fundmesh 透传；mesh 失败时回落本进程 subkey。
// @Author: CainReed
// @Created: 2021.08.02

package kcfund

import (
	  "bytes"
	"io"
	"strings"
	"github.com/gin-gonic/gin"
)

  func (a *API) forwardMeshSubKey(c *gin.Context, method, apiPath string, body []byte) bool {
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
	  code, raw, err := g.SubKeysAccountBook(c.Request.Context(), method, full, body, a.meshOpts(c))
	   if err != nil {
		   a.markMeshDown(c, err, apiPath)
	  	  return true
	   }
	  c.Data(code, "application/json", raw)
	     return true
  }
