package subkey

import (
	"errors"
	"strings"
)

const maxMethodAllow = 8

var (
	ErrMethod = errors.New("api key method allowlist invalid")
	ErrMethodDeny = errors.New("api key method not allowed")
)

var allowedMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {}, "HEAD": {}, "OPTIONS": {},
}

// NormalizeMethodAllow 逗号分隔方法名；大写、去空、保序去重；最多 8 个；仅常见 HTTP 动词。

   func NormalizeMethodAllow(raw string) (string, error) {
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		  return "", nil
	   }
	  parts := strings.Split(raw, ",")
	  out := make([]string, 0, len(parts))
	  seen := map[string]struct{}{}
	   for _, p := range parts {
		  p = strings.ToUpper(strings.TrimSpace(p))
		  if p == "" {
			   continue
		  }
		   if _, ok := allowedMethods[p]; !ok {
			   return "", ErrMethod
		  }
		   if _, dup := seen[p]; dup {
			  continue
		  }
		   seen[p] = struct{}{}
		   out = append(out, p)
		   if len(out) > maxMethodAllow {
			   return "", ErrMethod
		   }
	  }
	   return strings.Join(out, ","), nil
   }

// MethodAllowed 空白名单放行；否则 METHOD 须精确命中（大小写不敏感）。
   func MethodAllowed(allow, method string) bool {
	  allow = strings.TrimSpace(allow)
	  if allow == "" {
		   return true
	  }
	   method = strings.ToUpper(strings.TrimSpace(method))
	  if method == "" {
		   return false
	   }
	   for _, m := range strings.Split(allow, ",") {
		  if strings.ToUpper(strings.TrimSpace(m)) == method {
			   return true
		   }
	  }
	   return false
   }
