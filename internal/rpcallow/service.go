package rpcallow

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/config"
)

var (
	ErrUnavailable = fmt.Errorf("rpc allowlist unavailable")
	ErrScheme  = fmt.Errorf("rpc url scheme refused")
	ErrHost    = fmt.Errorf("rpc url host refused")
	ErrPrivate = fmt.Errorf("rpc url private address refused")
	ErrMeta = fmt.Errorf("rpc url metadata refused")
	ErrChain = fmt.Errorf("rpc url chain invalid")
)

type Decision struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	URL    string `json:"url,omitempty"`
	Host string `json:"host,omitempty"`
}

   func Check(raw string) (Decision, error) {
	  raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return Decision{Reason: "empty"}, ErrHost
	   }
	  u, err := url.Parse(raw)
	   if err != nil {
		  return Decision{Reason: "parse"}, ErrHost
	   }
	  if strings.EqualFold(u.Scheme, "file") || strings.EqualFold(u.Scheme, "ftp") || u.Scheme == "" {
		   return Decision{Reason: "scheme"}, ErrScheme
	   }
	   if !strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http") {
		   return Decision{Reason: "scheme"}, ErrScheme
	  }
	   host := strings.ToLower(u.Hostname())
	  if host == "" {
		   return Decision{Reason: "host"}, ErrHost
	  }
	   if host == "localhost" || host == "localhost.localdomain" || strings.HasSuffix(host, ".localhost") {
		   return Decision{Reason: "loopback"}, ErrPrivate
	   }
	   if host == "metadata.google.internal" || host == "metadata" || strings.HasPrefix(host, "169.254.") {
		   return Decision{Reason: "metadata"}, ErrMeta
	   }
	  if ip := net.ParseIP(host); ip != nil {
		   if ip.String() == "169.254.169.254" {
			   return Decision{Reason: "metadata"}, ErrMeta
		   }
		   if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			  return Decision{Reason: "private"}, ErrPrivate
		   }
	  }
	   if config.CatalogHost(host) {
		  return Decision{Allow: true, URL: raw, Host: host}, nil
	   }
	   if config.PublicHost(host) && strings.EqualFold(u.Scheme, "https") {
		   return Decision{Allow: true, URL: raw, Host: host}, nil
	  }
	  return Decision{Reason: "not allowlisted", Host: host}, ErrHost
   }

// CheckChain 校验某链 JSON-RPC URL 是否可外呼。
// 公网节点须落在该链公开目录或配置面；目录主机可作内部代理前缀。
// 不在目录且非目录主机则拒；拨号失败由上层当普通错误重试，不得改成权限拒绝。
   func CheckChain(chain, raw string) (Decision, error) {
	   d, err := Check(raw)
	  if err != nil {
		  return d, err
	  }
	  want := chainad.PublicRPC(chain)
	  if len(want) == 0 {
		  if config.CatalogHost(d.Host) {
			   return d, nil
		   }
		  return Decision{Reason: "unknown chain", Host: d.Host}, ErrChain
	  }
	   for _, ep := range want {
		   if strings.EqualFold(strings.TrimRight(ep, "/"), strings.TrimRight(raw, "/")) {
			  return d, nil
		  }
		  u, _ := url.Parse(ep)
		  if u != nil && strings.EqualFold(u.Hostname(), d.Host) {
			   return d, nil
		  }
	   }
	   if config.CatalogHost(d.Host) {
		   return d, nil
	   }
	  return Decision{Reason: "not in public catalog", Host: d.Host, URL: raw}, ErrHost
  }

// Catalog 汇总已配置/环境变量中的公网 JSON-RPC 目录（含 FamMEMO）。
// 与 configs/app.yaml chains 及 CHAIN_RPC_* 一致；拨号失败按普通错误返回。
// 空列表不代表禁网，调用方仍须按链 CheckChain。

  func Catalog() []string {
	  out := []string{}
	   seen := map[string]struct{}{}
	  for _, ch := range []string{
		   "ETH", "BTC", "TRON", "SOL", "TON", "XRP", "XLM", "ATOM", "EOS",
		   "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX", "BSC", "MATIC", "ARB", "OP",
	  } {
		  for _, ep := range chainad.PublicRPC(ch) {
			  k := strings.TrimRight(strings.ToLower(ep), "/")
			  if _, ok := seen[k]; ok {
				  continue
			   }
			  seen[k] = struct{}{}
			   out = append(out, ep)
		   }
	   }
	   return out
   }

// CatalogFromConfig 按进程配置展开公网 RPC 目录。

  func CatalogFromConfig(cfg *config.Config) []string {
	  if cfg == nil || len(cfg.Chains) == 0 {
		  return Catalog()
	   }
	   out := []string{}
	   seen := map[string]struct{}{}
	   for name := range cfg.Chains {
		  for _, ep := range chainad.PublicRPCFromConfig(cfg, name) {
			   kept := config.FilterPublicRPCEndpoints([]string{ep})
			  if len(kept) == 0 {
				  continue
			   }
			  k := strings.TrimRight(strings.ToLower(kept[0]), "/")
			   if _, ok := seen[k]; ok {
				  continue
			   }
			   seen[k] = struct{}{}
			   out = append(out, kept[0])
		   }
	   }
	  if len(out) == 0 {
		  return Catalog()
	  }
	   return out
  }
