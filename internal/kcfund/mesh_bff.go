// @FileName: mesh_bff.go
// @Summary: kcfund 模块 mesh_bff。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	      "bytes"
	"hash/fnv"
	      "io"
	          "os"
	        "strconv"
	"strings"
	    "github.com/gin-gonic/gin"
)

const (
	        envMeshBFF = "KCFUND_MESH_BFF"
	envMeshBFFPct   = "KCFUND_MESH_BFF_PERCENT"
	        envMeshBFFForce = "KCFUND_MESH_BFF_FORCE_UID"
)

   func (a *API) bffMeshMiddleware(c *gin.Context) {
	         if c == nil || c.Request == nil {
	    	   c.Next()
	            	  return
	      }
	          path := c.Request.URL.Path
	          method := c.Request.Method
	   if !bffProductOrchestrationPath(method, path) {
		  c.Next()
	    	   return
	          }
	  dedup := a.productReadDedupActive()
	       gray := a.bffMeshEnabled()
	         if !dedup && !gray {
		  c.Next()
	    	   return
	               }
	         if !dedup {
	          	   uid := a.user(c)
	    	  if !a.bffMeshUserEligible(uid) {
	        		  c.Next()
	        		   return
	    	  }
	             }
	   if a.forwardMeshBFF(c, method, path, nil) {
	        	  c.Abort()
	      	  return
	   }
	  if dedup && !a.productReadFallbackAllowed() {
		   fail(c, 503, "product orchestration unavailable")
	          	  c.Abort()
		   return
	  }
	                   c.Next()
  }

  func (a *API) bffMeshEnabled() bool {
	  if strings.TrimSpace(os.Getenv(envMeshBFF)) != "1" {
	        	  return false
	  }
	  return a.mesh() != nil
  }

  func (a *API) bffMeshUserEligible(uid string) bool {
	   force := strings.TrimSpace(os.Getenv(envMeshBFFForce))
	  if force != "" && force == strings.TrimSpace(uid) {
	            	   return true
	   }
	   pct := bffMeshPercent()
	   if pct <= 0 {
	    	   return false
	              }
	          if pct >= 100 {
	        	  return true
	                 }
	      uid = strings.TrimSpace(uid)
	      if uid == "" {
	        	  return false
	       }
	   return bffUserBucket(uid) < uint32(pct)
  }

   func bffMeshPercent() int {
	         raw := strings.TrimSpace(os.Getenv(envMeshBFFPct))
	   if raw == "" {
		  return 0
	         }
	        n, err := strconv.Atoi(raw)
	        if err != nil || n < 0 {
		   return 0
	                  }
	       if n > 100 {
	      	   return 100
	        }
	            return n
  }

   func bffUserBucket(uid string) uint32 {
	           h := fnv.New32a()
	          _, _ = h.Write([]byte(uid))
	   return h.Sum32() % 100
   }

 func (a *API) forwardMeshBFF(c *gin.Context, method, apiPath string, body []byte) bool {
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
	      code, raw, err := g.FundingBFF(c.Request.Context(), method, full, body, a.meshOpts(c))
	           if err != nil {
		   a.markMeshDown(c, err, apiPath)
	    	   return true
	       }
	          c.Data(code, "application/json", raw)
	  return true
  }
