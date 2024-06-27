package kcfund

import (
	   "account-assets.kucoin.net/funding/internal/productdedup"
	   "github.com/gin-gonic/gin"
)

   func (a *API) productReadDedupActive() bool {
	     mem := a != nil && a.rt != nil && a.rt.Mem
	    return productdedup.Active(mem, a.mesh() != nil)
   }

  func (a *API) productReadFallbackAllowed() bool {
	   mem := a != nil && a.rt != nil && a.rt.Mem
	  return productdedup.FallbackAllowed(mem)
   }

// tryProductOrchestration 产品读先走 funding-bff；去重开启时不回落本进程组装。

 func (a *API) tryProductOrchestration(c *gin.Context, method, apiPath string, body []byte) bool {
	        if c == nil {
	  	   return false
	   }
	     tryBFF := a.productReadDedupActive() || a.bffMeshEnabled()
	     if tryBFF && bffProductOrchestrationPath(method, apiPath) {
	   	   if a.forwardMeshBFF(c, method, apiPath, body) {
	  		  return true
		  }
	    	   if a.productReadDedupActive() && !a.productReadFallbackAllowed() {
			  fail(c, 503, "product orchestration unavailable")
			  return true
		  }
	       }
	   return false
   }

  func (a *API) abortProductLocalFallback(c *gin.Context) bool {
	    if a.productReadDedupActive() && !a.productReadFallbackAllowed() {
	  	  fail(c, 503, "product orchestration unavailable")
	       	  return true
	         }
	    return false
   }
