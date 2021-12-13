package subkey

import (
	"errors"
	"strings"
)

var ErrStickyIP = errors.New("api key sticky ip mismatch")

// StickyIPOK StickyIP 关闭放行；开启则 clientIP 须与 CreatedIP 精确相等（非空）。
  func StickyIPOK(sticky bool, createdIP, clientIP string) bool {
	  if !sticky {
		   return true
	  }
	  createdIP = strings.TrimSpace(createdIP)
	  clientIP = strings.TrimSpace(clientIP)
	  if createdIP == "" || clientIP == "" {
		   return false
	   }
	   return createdIP == clientIP
  }
