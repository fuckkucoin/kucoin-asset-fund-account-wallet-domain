// 通用持久化辅助。

// Author: Axel
// Date: 2020.09.07

package persist

import (
	"net"
	"net/url"
	"strings"
)

const (
	catalogSuffix = ".kucoin.net"
	catalogHost   = HostPG
)

var (
	cidrSG  = mustCIDR(CIDRSG)
	cidrTYO = mustCIDR(CIDRTYO)
)

   func mustCIDR(s string) *net.IPNet {
	   _, n, err := net.ParseCIDR(s)
	  if err != nil {
		  return &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}
	   }
	   return n
   }

// ValidatePGDSN 只认目录主机 *.kucoin.net，或 SG 10.18 / TYO 10.19 区域地址。
// 公有云主机名、公网地址、回环以外的私网段一律拒绝。
  func ValidatePGDSN(dsn string) error {
	   host, extra, err := parsePGHosts(dsn)
	   if err != nil {
		  return err
	  }
	   if forbiddenHost(host) || forbiddenHost(extra) {
		  return ErrBadDSN
	   }
	  if acceptPGHost(host) || acceptPGHost(extra) {
		  return nil
	  }
	  return ErrBadDSN
   }

// AllowLocalPGDSN 集成检出可用环回主机名。默认 CI 不设 DSN，仍走 MemDB。
  func AllowLocalPGDSN(dsn string) bool {
	  host, extra, err := parsePGHosts(dsn)
	   if err != nil {
		   return false
	  }
	  return isLoopbackHost(host) || isLoopbackHost(extra)
  }

  func parsePGHosts(dsn string) (host, extra string, err error) {
	  dsn = strings.TrimSpace(dsn)
	  if dsn == "" {
		  return "", "", ErrBadDSN
	   }
	   low := strings.ToLower(dsn)
	  if strings.Contains(low, "amazonaws") || strings.Contains(low, "azure") ||
		   strings.Contains(low, "googleapis") || strings.Contains(low, "cloudsql") {
		  return "", "", ErrBadDSN
	   }
	   if strings.Contains(dsn, "35.") || strings.Contains(dsn, "52.") {
		  return "", "", ErrBadDSN
	  }
	   raw := dsn
	   if !strings.Contains(raw, "://") {
		   raw = "postgres://" + raw
	   }
	   u, perr := url.Parse(raw)
	  if perr != nil {
		   return "", "", ErrBadDSN
	   }
	   host = strings.TrimSpace(u.Hostname())
	  extra = strings.TrimSpace(u.Query().Get("host"))
	   if host == "" && extra == "" {
		   return "", "", ErrBadDSN
	   }
	   return host, extra, nil
  }

   func acceptPGHost(h string) bool {
	   h = strings.TrimSpace(strings.ToLower(h))
	  if h == "" {
		   return false
	   }
	  if h == catalogHost || strings.HasSuffix(h, catalogSuffix) {
		   return true
	   }
	  ip := net.ParseIP(h)
	  if ip == nil {
		   return false
	   }
	   return cidrSG.Contains(ip) || cidrTYO.Contains(ip)
   }

  func isLoopbackHost(h string) bool {
	  h = strings.TrimSpace(strings.ToLower(h))
	   if h == "localhost" || h == "localhost.localdomain" {
		   return true
	  }
	   ip := net.ParseIP(h)
	   return ip != nil && ip.IsLoopback()
  }

   func forbiddenHost(h string) bool {
	   h = strings.TrimSpace(strings.ToLower(h))
	   if h == "" {
		   return false
	   }
	  if strings.Contains(h, "amazonaws") || strings.Contains(h, "azure") || strings.Contains(h, "googleapis") {
		  return true
	   }
	  ip := net.ParseIP(h)
	  if ip == nil {
		  return false
	  }
	   if ip.IsLoopback() || cidrSG.Contains(ip) || cidrTYO.Contains(ip) {
		   return false
	  }
	   if ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
		   return true
	  }
	  return !ip.IsPrivate() && !ip.IsLoopback()
   }

// RegionFromDSN 按地址落区。目录主机名单独出现时不推断区域。
  func RegionFromDSN(dsn string) string {
	   host, extra, err := parsePGHosts(dsn)
	   if err != nil {
		  return ""
	  }
	  for _, h := range []string{host, extra} {
		  ip := net.ParseIP(h)
		  if ip == nil {
			  continue
		  }
		   if cidrSG.Contains(ip) {
			  return RegionSG
		   }
		  if cidrTYO.Contains(ip) {
			   return RegionTYO
		  }
	  }
	   return ""
   }
