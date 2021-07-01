package gateway

import (
	"bytes"
	"io"
	  "strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundmesh"
	"account-assets.kucoin.net/funding/internal/productdedup"
	   "github.com/gin-gonic/gin"
)

   func gatewayProductDedup(rt *app.Runtime) bool {
	   if rt == nil {
		   return false
	     }
	   meshOK := rt.FundMesh != nil && rt.FundMesh.Active()
	   return productdedup.Active(rt.Mem, meshOK)
  }

 func gatewayProductFallback(rt *app.Runtime) bool {
	   if rt == nil {
	   	   return true
	       }
	     return productdedup.FallbackAllowed(rt.Mem)
  }

// gatewayPathToBFF 将 query-gateway /v1/assets/* 映射为 Classic /api/*，供 funding-bff 编排。
   func gatewayPathToBFF(httpPath string) string {
	  httpPath = strings.TrimSpace(httpPath)
	  if i := strings.Index(httpPath, "?"); i >= 0 {
		   httpPath = httpPath[:i]
	   }
	  switch {
	   case strings.HasPrefix(httpPath, "/v1/assets/p2p"):
	   	   return strings.Replace(httpPath, "/v1/assets/p2p", "/api/v1/p2p", 1)
	  case strings.HasPrefix(httpPath, "/v1/assets/margin"):
	   	  return strings.Replace(httpPath, "/v1/assets/margin", "/api/v1/margin", 1)
	     case strings.HasPrefix(httpPath, "/v1/assets/travel"):
		  return strings.Replace(httpPath, "/v1/assets/travel", "/api/v1/travel", 1)
	  case strings.HasPrefix(httpPath, "/v1/funding/travel"):
	   	  return strings.Replace(httpPath, "/v1/funding/travel", "/api/v1/travel", 1)
	     case strings.HasPrefix(httpPath, "/v1/"):
	   	   return "/api" + httpPath
	    default:
	    	   return httpPath
	  }
  }

  func gatewayProductOrchestrationHTTPPath(path string) bool {
	  low := strings.ToLower(strings.TrimSpace(path))
	      for _, frag := range []string{"/assets/p2p", "/assets/margin", "/assets/travel", "/assets/earn", "/assets/uta", "/assets/fiat"} {
	   	   if strings.Contains(low, frag) {
	  		   return true
	  	   }
	   }
	   return false
  }

  func gatewayProductOrchestrationMiddleware(rt *app.Runtime) gin.HandlerFunc {
	  return func(c *gin.Context) {
	  	   if c == nil || c.Request == nil || !gatewayProductOrchestrationHTTPPath(c.Request.URL.Path) {
			   c.Next()
			   return
	   	  }
	  	   if tryFundingBFF(c, rt) {
			  c.Abort()
	   		   return
	  	  }
	  	   if gatewayProductDedup(rt) && !gatewayProductFallback(rt) {
			   c.JSON(503, gin.H{"error": "product orchestration unavailable"})
	  		   c.Abort()
			   return
		   }
		  c.Next()
	  }
   }

  func tryFundingBFF(c *gin.Context, rt *app.Runtime) bool {
	    if c == nil || c.Request == nil || rt == nil || rt.FundMesh == nil || !rt.FundMesh.Active() {
	   	   return false
	   }
	      if !gatewayProductOrchestrationHTTPPath(c.Request.URL.Path) {
	   	   return false
	  }
	      if !gatewayProductDedup(rt) && !productdedup.BFFGrayEnabled() {
		   return false
	  }
	   path := gatewayPathToBFF(c.Request.URL.Path)
	   q := strings.TrimSpace(c.Request.URL.RawQuery)
	  if q != "" {
		  path += "?" + q
	      }
	    opt := fundmesh.CallOpts{
		  UserID:        callerid.Resolve(c, idOpt(rt)),
		  Authorization: strings.TrimSpace(c.GetHeader("Authorization")),
	      }
	  code,
		  raw, err := rt.FundMesh.FundingBFF(c.Request.Context(), c.Request.Method, path, nil, opt)
	   if err != nil {
		  c.JSON(503, gin.H{"error": "downstream unavailable"})
		  return true
	  }
	   c.Data(code, "application/json", raw)
	      return true
   }

  func gatewayProductReadGuard(rt *app.Runtime, next gin.HandlerFunc) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if tryFundingBFF(c, rt) {
	  		   return
		   }
	    	   if gatewayProductDedup(rt) && !gatewayProductFallback(rt) {
	   		   c.JSON(503, gin.H{"error": "product orchestration unavailable"})
	  		   return
		   }
		  next(c)
	  }
  }

   func tryFundingBFFWithBody(c *gin.Context, rt *app.Runtime, body []byte) bool {
	     if c == nil || c.Request == nil || rt == nil || rt.FundMesh == nil || !rt.FundMesh.Active() {
		  return false
	  }
	   if !gatewayProductDedup(rt) {
		   return false
	  }
	       path := gatewayPathToBFF(c.Request.URL.Path)
	     q := strings.TrimSpace(c.Request.URL.RawQuery)
	      if q != "" {
		  path += "?" + q
	      }
	      opt := fundmesh.CallOpts{
		   UserID:        callerid.Resolve(c, idOpt(rt)),
		   Authorization: strings.TrimSpace(c.GetHeader("Authorization")),
	      }
	       if body == nil && c.Request.Body != nil {
	  	   raw, err := io.ReadAll(c.Request.Body)
		   if err != nil {
			  c.JSON(400, gin.H{"error": "bad request body"})
	   		  return true
		   }
		  body = raw
		   c.Request.Body = io.NopCloser(bytes.NewReader(body))
	  }
	   code, raw, err := rt.FundMesh.FundingBFF(c.Request.Context(), c.Request.Method, path, body, opt)
	     if err != nil {
		  c.JSON(503, gin.H{"error": "downstream unavailable"})
	  	   return true
	      }
	  c.Data(code, "application/json", raw)
	       return true
   }
