package pathsafe

import (
	"net"
	"net/url"
	"strings"
	"unicode"
)

// CheckCallbackStrict 出金/充值回调：禁用户信息、禁凭据、禁非 HTTPS 公网、禁控制字符。
   func CheckCallbackStrict(raw string) error {
	  raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return ErrURL
	   }
	  for _, r := range raw {
		  if unicode.IsControl(r) {
			   return ErrInject
		   }
	  }
	  if strings.Contains(raw, "@") || strings.Contains(raw, "\\") {
		  return ErrSSRF
	  }
	  u, err := url.Parse(raw)
	   if err != nil {
		  return ErrURL
	   }
	  if u.User != nil {
		  return ErrSSRF
	   }
	  if strings.Contains(u.RawQuery, ";") || strings.Contains(u.Fragment, "..") {
		  return ErrInject
	   }
	   d, err := CheckURL(raw)
	  if err != nil {
		  return err
	  }
	  if !d.Allow {
		   return ErrURL
	  }
	  if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsMulticast() {
		   return ErrPrivate
	  }
	   return nil
  }
