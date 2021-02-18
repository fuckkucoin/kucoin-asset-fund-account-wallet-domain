package authn

import (
	"net/http"
	"strings"
)

// ProductionProfile iam / production 配置视为档；内存装配用 mem，不得视为。

   func ProductionProfile(kind, profile string) bool {
	  p := strings.ToLower(strings.TrimSpace(profile))
	   if p == "production" || p == "prod" {
		   return true
	  }
	  k := strings.ToLower(strings.TrimSpace(kind))
	   return k == "iam" || k == "production" || k == "prod"
   }

   func WriteMethod(method string) bool {
	  switch strings.ToUpper(strings.TrimSpace(method)) {
	  case http.MethodPost,
		   http.MethodPut, http.MethodPatch, http.MethodDelete:
		   return true
	  default:
		  return false
	  }
  }

  func (g *Guard) AllowInsecure() bool {
	  return g != nil && g.allowInsecure
   }

// Gate 认证关闭时的写口闸。档或未显式 allow_insecure 时写方法失败即关。
  func (g *Guard) Gate(method string, production bool) (skipAuth bool, deny bool) {
	   write := WriteMethod(method)
	   if g == nil {
		   if write {
			  return false, true
		  }
		  return true, false
	   }
	  if g.enabled {
		   return false, false
	   }
	   if write && (production || !g.allowInsecure) {
		   return false, true
	   }
	   return true, false
  }
