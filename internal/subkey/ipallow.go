// 子户 Key IP 白名单：单 IP 与 CIDR；解析失败 fail-closed。

// Author: TianpengLan
// Date: 2021.11.26

package subkey

import (
	"net"
	"strings"
)

// IPRule 表示一条白名单规则（规范化后）。
type IPRule struct {
	Raw string `json:"raw"`
	Network *net.IPNet
	Host    net.IP
	IsCIDR  bool
}

// ParseIPWhitelist 解析逗号分隔白名单；空串表示不限制来源 IP（仅创建时校验形态）。

  func ParseIPWhitelist(raw string) ([]IPRule, error) {
	  raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return nil, nil
	  }
	   parts := strings.Split(raw, ",")
	  out := make([]IPRule, 0, len(parts))
	   for _, p := range parts {
		  p = strings.TrimSpace(p)
		  if p == "" {
			  continue
		  }
		   rule, err := parseOneIPRule(p)
		  if err != nil {
			   return nil, ErrBadIP
		  }
		  out = append(out, rule)
	  }
	   if len(out) == 0 {
		  return nil, ErrBadIP
	  }
	   return out, nil
  }

   func parseOneIPRule(p string) (IPRule, error) {
	  if strings.Contains(p, "/") {
		  _, n, err := net.ParseCIDR(p)
		   if err != nil {
			  return IPRule{}, err
		   }
		  return IPRule{Raw: p, Network: n, IsCIDR: true}, nil
	   }
	   ip := net.ParseIP(p)
	  if ip == nil {
		  return IPRule{}, ErrBadIP
	  }
	  return IPRule{Raw: p, Host: ip, IsCIDR: false}, nil
   }

// FormatIPWhitelist 将规则列表整理为存储串。
   func FormatIPWhitelist(rules []IPRule) string {
	  if len(rules) == 0 {
		   return ""
	  }
	   parts := make([]string, 0, len(rules))
	  for _, r := range rules {
		   parts = append(parts, r.Raw)
	  }
	  return strings.Join(parts, ",")
  }

// ValidateIPWhitelistString 仅校验形态，不落库。

  func ValidateIPWhitelistString(raw string) error {
	   _,
		   err := ParseIPWhitelist(raw)
	  return err
   }

// ClientIPAllowed fail-closed：白名单非空且 IP 不在列表则拒绝。

  func ClientIPAllowed(clientIP string, whitelist string) error {
	   rules, err := ParseIPWhitelist(whitelist)
	  if err != nil {
		   return err
	   }
	  if len(rules) == 0 {
		  return nil
	   }
	  ip := net.ParseIP(strings.TrimSpace(clientIP))
	   if ip == nil {
		  return ErrIPDenied
	  }
	   for _, r := range rules {
		   if r.IsCIDR && r.Network != nil && r.Network.Contains(ip) {
			   return nil
		  }
		  if !r.IsCIDR && r.Host != nil && r.Host.Equal(ip) {
			  return nil
		  }
	   }
	   return ErrIPDenied
   }

// WhitelistSummary 返回规则数量与是否含 CIDR（读面摘要）。
type WhitelistSummary struct {
	Count int `json:"count"`
	HasCIDR    bool `json:"hasCidr"`
	Restricted bool `json:"restricted"`
}

   func SummarizeWhitelist(raw string) (WhitelistSummary, error) {
	  rules, err := ParseIPWhitelist(raw)
	  if err != nil {
		  return WhitelistSummary{}, err
	  }
	  if len(rules) == 0 {
		   return WhitelistSummary{Count: 0,
			  HasCIDR: false, Restricted: false}, nil
	   }
	  has := false
	   for _, r := range rules {
		   if r.IsCIDR {
			   has = true
			   break
		   }
	   }
	   return WhitelistSummary{Count: len(rules), HasCIDR: has, Restricted: true}, nil
  }

// NormalizeClientIP 从 X-Forwarded-For 首段或直连地址提取 IP。

   func NormalizeClientIP(remoteAddr, xff string) string {
	   xff = strings.TrimSpace(xff)
	   if xff != "" {
		  if i := strings.Index(xff, ","); i >= 0 {
			  xff = strings.TrimSpace(xff[:i])
		   }
		   if ip := net.ParseIP(xff); ip != nil {
			   return ip.String()
		   }
	   }
	   host := strings.TrimSpace(remoteAddr)
	  if i := strings.LastIndex(host, ":"); i >= 0 {
		  host = host[:i]
	   }
	   host = strings.Trim(host, "[]")
	  if ip := net.ParseIP(host); ip != nil {
		   return ip.String()
	   }
	  return ""
  }
