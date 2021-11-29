// 子账户 API Key 路径前缀白名单。空表示不限；非空时请求 path 须匹配任一前缀。

// Author: Axel
// Date: 2021.11.28

package subkey

import (
	"errors"
	"strings"
)

const maxPathAllow = 16

var (
	ErrPath     = errors.New("api key path allowlist invalid")
	ErrPathDeny = errors.New("api key path not allowed")
)

// NormalizePathAllow 逗号分隔前缀；去空、保序去重；每条以 / 开头且 ≤128 rune；最多 16 条。
  func NormalizePathAllow(raw string) (string, error) {
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return "", nil
	   }
	   parts := strings.Split(raw, ",")
	  out := make([]string, 0, len(parts))
	   seen := map[string]struct{}{}
	  for _, p := range parts {
		   p = strings.TrimSpace(p)
		  if p == "" {
			   continue
		   }
		   if !strings.HasPrefix(p, "/") {
			  return "", ErrPath
		   }
		  if strings.ContainsAny(p, " \t\r\n") {
			   return "", ErrPath
		  }
		   if len([]rune(p)) > 128 {
			   return "", ErrPath
		  }
		   if _, ok := seen[p]; ok {
			   continue
		  }
		  seen[p] = struct{}{}
		   out = append(out, p)
		  if len(out) > maxPathAllow {
			   return "", ErrPath
		  }
	  }
	  return strings.Join(out, ","), nil
   }

// PathAllowed 空白名单放行；否则 path（去 query）须等于或以前缀匹配任一条。

  func PathAllowed(allow, path string) bool {
	  allow = strings.TrimSpace(allow)
	   if allow == "" {
		  return true
	   }
	   path = strings.TrimSpace(path)
	  if i := strings.IndexByte(path, '?'); i >= 0 {
		  path = path[:i]
	   }
	   if path == "" {
		   return false
	   }
	   for _, p := range strings.Split(allow, ",") {
		  p = strings.TrimSpace(p)
		  if p == "" {
			  continue
		   }
		  if path == p {
			   return true
		  }
		  if strings.HasPrefix(path, p) {
			  if len(path) == len(p) {
				  return true
			  }
			   next := path[len(p)]
			  if next == '/' {
				   return true
			   }
		  }
	  }
	   return false
  }
