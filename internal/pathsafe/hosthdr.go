package pathsafe

import (
	"net"
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/internal/config"
)

// CheckHostHeader 请求 Host。禁控制字符、用户信息、裸 IP；只放目录主机或公网主机名。

   func CheckHostHeader(raw string) error {
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return ErrURL
	  }
	   for _, r := range raw {
		   if unicode.IsControl(r) || r == '@' || r == '\\' || r == '/' {
			  return ErrInject
		  }
	  }
	   if strings.Contains(raw, "://") {
		  return ErrInject
	  }
	  host := raw
	   if h, _, err := splitHostPortSafe(raw); err == nil {
		   host = h
	  }
	  host = strings.ToLower(strings.TrimSpace(host))
	  if host == "" {
		   return ErrURL
	   }
	   if ip := net.ParseIP(host); ip != nil {
		   return ErrPrivate
	  }
	  if config.CatalogHost(host) {
		  return nil
	   }
	   if config.PublicHost(host) {
		  return nil
	   }
	   return ErrURL
   }

   func splitHostPortSafe(raw string) (string, string, error) {
	  if !strings.Contains(raw, ":") {
		  return raw, "", nil
	   }
	   host, port, err := net.SplitHostPort(raw)
	   return host, port, err
  }
