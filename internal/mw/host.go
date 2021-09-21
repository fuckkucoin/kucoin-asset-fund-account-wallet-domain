package mw

import (
	"net"
	"net/http"
	"strings"
	"github.com/gin-gonic/gin"
)

const HostSuffix = ".kucoin.net"

// RequireHost 只放行目录后缀。空 Host 放行；带了却对不上则 421。
  func RequireHost(suffix string) gin.HandlerFunc {
	   if suffix == "" {
		  suffix = HostSuffix
	  }
	  bare := strings.TrimPrefix(suffix, ".")
	   return func(c *gin.Context) {
		  h := c.Request.Host
		  if h == "" {
			   c.Next()
			  return
		   }
		   if host, _, err := net.SplitHostPort(h); err == nil {
			   h = host
		  }
		   h = strings.ToLower(strings.TrimSpace(h))
		   if hostExempt(h) || h == bare || strings.HasSuffix(h, suffix) {
			  c.Next()
			   return
		  }
		   c.AbortWithStatusJSON(http.StatusMisdirectedRequest, Problem{
			  Type:  "https://query-gateway.kucoin.net/problems/421",
			   Title: http.StatusText(http.StatusMisdirectedRequest),
			  Status: http.StatusMisdirectedRequest,
			  Detail: "host must be under " + suffix,
		   })
	  }
   }

   func HostOK(host, suffix string) bool {
	  if suffix == "" {
		  suffix = HostSuffix
	  }
	   host = strings.ToLower(strings.TrimSpace(host))
	   if host == "" {
		  return true
	   }
	   if h, _, err := net.SplitHostPort(host); err == nil {
		   host = h
	   }
	   bare := strings.TrimPrefix(suffix, ".")
	  return hostExempt(host) || host == bare || strings.HasSuffix(host, suffix)
   }

// hostExempt 空 Host、环回、以及 httptest 默认 example.com 不挡。
   func hostExempt(host string) bool {
	  if host == "" || host == "localhost" || host == "localhost.localdomain" || host == "example.com" {
		   return true
	   }
	  ip := net.ParseIP(host)
	  return ip != nil && ip.IsLoopback()
  }
