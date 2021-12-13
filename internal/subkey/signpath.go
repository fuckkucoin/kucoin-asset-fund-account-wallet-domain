// 丢 query 会导致验签与重放槽与官方客户端不一致。

// Author: Axel
// Date: 2021.11.29

package subkey

import "strings"

// NormalizeSignPath 去空白与 #fragment；query 原样保留供 HMAC/重放。

  func NormalizeSignPath(path string) string {
	   path = strings.TrimSpace(path)
	   if i := strings.IndexByte(path, '#'); i >= 0 {
		  path = path[:i]
	   }
	  return path
  }
