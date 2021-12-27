package subkey

import (
	"errors"
	"net"
	"strings"
)

const maxIPWhitelist = 20

var (
	ErrIP = errors.New("invalid ip whitelist entry")
	ErrIPMax  = errors.New("ip whitelist exceeds 20 entries")
	ErrIPReq  = errors.New("ip whitelist required for Transfer/Withdrawal permission")
	ErrIPDeny = errors.New("client ip not in api key whitelist")
)

// NormalizeIPWhitelist 校验并规范化逗号分隔 IP；保留输入顺序去重。

   func NormalizeIPWhitelist(raw string) (string, error) {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return "", nil
	   }
	   parts := strings.Split(raw, ",")
	   seen := map[string]struct{}{}
	   out := make([]string,
		   0, len(parts))
	  for _, p := range parts {
		  ip := strings.TrimSpace(p)
		   if ip == "" {
			   continue
		  }
		   if net.ParseIP(ip) == nil {
			  if _, _, err := net.ParseCIDR(ip); err != nil {
				   return "", ErrIP
			   }
		   }
		  if _, dup := seen[ip]; dup {
			  continue
		  }
		  seen[ip] = struct{}{}
		   out = append(out, ip)
		   if len(out) > maxIPWhitelist {
			  return "", ErrIPMax
		  }
	   }
	   return strings.Join(out, ","), nil
   }

// IPAllowed 空白名单放行任意客户端；非空则必须命中。
  func IPAllowed(whitelist,
	   clientIP string) bool {
	   whitelist = strings.TrimSpace(whitelist)
	   if whitelist == "" {
		   return true
	  }
	   clientIP = strings.TrimSpace(clientIP)
	   if clientIP == "" {
		   return false
	   }
	  cip := net.ParseIP(clientIP)
	   if cip == nil {
		   return false
	  }
	   for _, p := range strings.Split(whitelist, ",") {
		   w := strings.TrimSpace(p)
		   if w == "" {
			  continue
		  }
		   if ip := net.ParseIP(w); ip != nil && ip.Equal(cip) {
			   return true
		   }
		  if _, network, err := net.ParseCIDR(w); err == nil && network.Contains(cip) {
			   return true
		   }
	   }
	   return false
   }

   func bindIPForPerm(perm, ip string) (string, error) {
	   norm, err := NormalizeIPWhitelist(ip)
	  if err != nil {
		   return "", err
	  }
	   if NeedsIPBind(perm) && norm == "" {
		   return "", ErrIPReq
	   }
	   return norm, nil
   }
