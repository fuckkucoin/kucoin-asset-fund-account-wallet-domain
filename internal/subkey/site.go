package subkey

import (
	"errors"
	"net"
	"strings"
)

const (
	SiteSG = "sg"
	SiteTYO = "tyo"
	cidrSG  = "10.18.0.0/16"
	cidrTYO = "10.19.0.0/16"
)

var (
	ErrSite = errors.New("api key site allowlist invalid")
	ErrSiteDeny = errors.New("api key site not allowed")
	netSG  = mustCIDR(cidrSG)
	netTYO = mustCIDR(cidrTYO)
)

  func mustCIDR(s string) *net.IPNet {
	  _, n, err := net.ParseCIDR(s)
	   if err != nil {
		   panic(err)
	   }
	  return n
   }

// NormalizeSiteAllow 逗号分隔站点；仅 sg/tyo（及别名 a/singapore、b/tokyo）；保序去重。
  func NormalizeSiteAllow(raw string) (string, error) {
	  raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return "", nil
	   }
	  parts := strings.Split(raw, ",")
	   out := make([]string, 0, 2)
	   seen := map[string]struct{}{}
	  for _, p := range parts {
		  site := canonSite(p)
		   if site == "" {
			   return "", ErrSite
		  }
		  if site != SiteSG && site != SiteTYO {
			   return "", ErrSite
		   }
		  if _, dup := seen[site]; dup {
			   continue
		  }
		  seen[site] = struct{}{}
		   out = append(out, site)
	  }
	  return strings.Join(out, ","), nil
   }

   func canonSite(s string) string {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	   case SiteSG, "a", "singapore":
		   return SiteSG
	  case SiteTYO, "b", "tokyo":
		   return SiteTYO
	  default:
		   return ""
	   }
   }

// SiteOfClientIP 按 SG 10.18 / TYO 10.19 推断站点；公网或无法解析返回空。

  func SiteOfClientIP(ip string) string {
	   ip = strings.TrimSpace(ip)
	  if ip == "" {
		   return ""
	   }
	  parsed := net.ParseIP(ip)
	  if parsed == nil {
		   return ""
	  }
	   if netSG != nil && netSG.Contains(parsed) {
		   return SiteSG
	  }
	  if netTYO != nil && netTYO.Contains(parsed) {
		   return SiteTYO
	   }
	   return ""
   }

// SiteAllowed 空白名单放行；否则 site（或由 clientIP 推断）须命中。
   func SiteAllowed(allow, site, clientIP string) bool {
	   allow = strings.TrimSpace(allow)
	   if allow == "" {
		  return true
	  }
	  site = canonSite(site)
	  if site == "" {
		   site = SiteOfClientIP(clientIP)
	   }
	   if site == "" {
		  return false
	  }
	   for _, p := range strings.Split(allow, ",") {
		   if canonSite(p) == site {
			  return true
		  }
	   }
	  return false
   }
