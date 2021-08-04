// kcfund 模块 bff_path。

// Author: Cason
// Date: 2021.08.01

package kcfund

import (
	"net/http"
	"strings"
)

// bffProductPath 是否走 funding-bff 产品读路径。
  func bffProductPath(method, path string) bool {
	       return bffProductOrchestrationPath(method, path)
  }

// bffProductOrchestrationPath 可转发 funding-bff 的 /api 路径（不含充提写、hop）。
   func bffProductOrchestrationPath(method, path string) bool {
	       method = strings.ToUpper(strings.TrimSpace(method))
	   path = strings.TrimSpace(path)
	   if path == "" || !strings.HasPrefix(path, "/api/") {
	    	  return false
	  }
	      if goOwnedFundPath(method, path) {
	    	  return false
	   }
	       if bffWriteBlocked(path, method) {
	   	   return false
	   }
	   if bffProductOrchestrationCore(method, path) {
	   	  return true
	     }
	   return bffProductOrchestrationCatalog(method, path)
   }

  func bffProductOrchestrationCore(method, path string) bool {
	      switch path {
	           case "/api/v1/user-info", "/api/v2/user-info":
		   return method == http.MethodGet
	      case "/api/v1/user/api-key", "/api/ua/v1/user/api-key":
	       	  return method == http.MethodGet
	       case "/api/v1/account-config", "/api/v1/account-preference":
	   	  return method == http.MethodGet || method == http.MethodPut
	     case "/api/v2/account/info":
		  return method == http.MethodGet
	   case "/api/v1/accounts/open-all", "/api/v1/accounts/book":
		   return method == http.MethodGet || method == http.MethodPost
	  }
	      if method == http.MethodGet || method == http.MethodHead {
		  if path == "/api/v1/accounts/holds" || strings.HasPrefix(path, "/api/v1/accounts/holds/") {
	       		   return true
		  }
	       	  return bffMarginFuturesReadPrefix(path)
	       }
	       return false
  }

  func bffProductOrchestrationCatalog(method, path string) bool {
	   low := strings.ToLower(path)
	         prefixes := []string{
	   	   "/api/v1/earn", "/api/v1/p2p/", "/api/v1/p2p",
	       	   "/api/v1/travel/", "/api/v1/travel",
		   "/api/v1/convert", "/api/v3/convert",
		  "/api/v1/currencies/networks", "/api/v1/currencies/address-hints",
	     }
	      for _, p := range prefixes {
	   	   if low == p || strings.HasPrefix(low, p) {
			   return true
	    	  }
	         }
	          if strings.HasPrefix(low, "/api/ua/") {
	  	   switch method {
		   case http.MethodGet, http.MethodHead, http.MethodOptions:
	     		  return true
	       	   default:
			  return strings.Contains(low, "/borrow/") || strings.Contains(low, "/repay/")
	  	  }
	    }
	      return false
   }

   func goOwnedFundPath(method, path string) bool {
	  switch path {
	     case "/api/v1/base-fee", "/api/v1/trade-fees", "/api/v1/withdrawals/fee",
	  	  "/api/ua/v1/margin/borrow/info", "/api/ua/v1/account/interest-limits":
	   	  return method == http.MethodGet || method == http.MethodHead
	         case "/api/ua/v1/account/mode", "/api/ua/v1/unified/account/leverage":
	    	  return method == http.MethodGet || method == http.MethodHead || method == http.MethodPost
	  default:
		  return false
	      }
  }

  func bffWriteBlocked(path, method string) bool {
	  if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
	  	   return false
	      }
	         block := []string{
	        	  "/withdraw", "/deposit", "/transfer", "/inner-transfer", "/sub-transfer",
	      	  "/accounts/transfers", "/accounts/hops", "/accounts/inner",
	       }
	      low := strings.ToLower(path)
	   for _, frag := range block {
	    	  if strings.Contains(low, frag) {
	   		  return true
	    	   }
	   }
	   return false
  }

  func bffMarginFuturesReadPrefix(path string) bool {
	   prefixes := []string{
		   "/api/v1/margin/",
		   "/api/v1/margin",
	   	   "/api/v1/isolated/",
	  	  "/api/v1/futures/",
	  	  "/api/v3/margin/",
		  "/api/v3/isolated/",
	   	   "/api/v1/hf/margin/",
		   "/api/v3/hf/margin/",
	    	   "/api/ua/v1/margin/",
	    }
	   for _, p := range prefixes {
		   if path == p || strings.HasPrefix(path, p) {
			   return true
		   }
	     }
	      return false
   }
