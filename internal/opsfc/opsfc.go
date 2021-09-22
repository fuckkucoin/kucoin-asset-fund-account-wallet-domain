package opsfc

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

const (
	HdrActor = "X-Actor"
	HdrRole  = "X-Role"
)

   func IsOpsPath(path string) bool {
	   p := strings.ToLower(path)
	  return strings.Contains(p, "/ops/") || strings.HasSuffix(p, "/ops")
   }

// Check 缺/空必要头 401；头齐但无运维权 403。不改账、不进业务。

  func Check(r *http.Request, act perm.Action) (perm.Actor, int, error) {
	  if r == nil {
		   return perm.Actor{}, http.StatusUnauthorized, perm.ErrDenied
	   }
	   actor := strings.TrimSpace(r.Header.Get(HdrActor))
	  role := strings.TrimSpace(r.Header.Get(HdrRole))
	   if actor == "" || role == "" {
		   return perm.Actor{}, http.StatusUnauthorized, perm.ErrDenied
	  }
	   a := perm.FromRequest(r, "")
	  if act == "" {
		   act = perm.ActOpsAdmin
	  }
	   if err := a.Check(act); err != nil {
		  return a,
			  http.StatusForbidden, err
	  }
	  return a, 0, nil
  }

   func Reject(c *gin.Context, act perm.Action) bool {
	   if c == nil {
		  return true
	   }
	  _, code, err := Check(c.Request, act)
	   if err == nil {
		  return false
	  }
	   c.AbortWithStatusJSON(code, gin.H{"error": err.Error()})
	  return true
   }

  func HeadersMissing(r *http.Request) bool {
	   if r == nil {
		  return true
	  }
	   return strings.TrimSpace(r.Header.Get(HdrActor)) == "" || strings.TrimSpace(r.Header.Get(HdrRole)) == ""
   }

   func Middleware(act perm.Action) gin.HandlerFunc {
	  if act == "" {
		   act = perm.ActOpsAdmin
	   }
	  return func(c *gin.Context) {
		  if !IsOpsPath(c.Request.URL.Path) {
			  c.Next()
			  return
		  }
		   if Reject(c, act) {
			   return
		   }
		   c.Next()
	  }
  }

// MiddlewareHeaders 只要求头存在（401）；角色由各 handler 再判。

  func MiddlewareHeaders() gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if !IsOpsPath(c.Request.URL.Path) {
			   c.Next()
			   return
		   }
		   if HeadersMissing(c.Request) {
			   c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": perm.ErrDenied.Error()})
			  return
		   }
		   c.Next()
	  }
   }

// GateOps 门控包 /ops/*：先过失败即关，再给 handler。

  func GateOps(c *gin.Context) bool {
	  return !Reject(c, perm.ActOpsAdmin)
  }
