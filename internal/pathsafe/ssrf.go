package pathsafe

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/rpcallow"
)

var (
	ErrURL     = fmt.Errorf("url refused")
	ErrSSRF    = fmt.Errorf("ssrf target refused")
	ErrPrivate = fmt.Errorf("private url refused")
)

type URLDecision struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	URL  string `json:"url,omitempty"`
	Host string `json:"host,omitempty"`
}

// CheckURL 外呼闸：私网、元数据、file、相对穿越一律拒。内部表或公共 HTTPS 才放。

  func CheckURL(raw string) (URLDecision, error) {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return URLDecision{Reason: "empty"}, ErrURL
	  }
	   if strings.ContainsRune(raw, 0) {
		  return URLDecision{Reason: "inject"}, ErrInject
	  }
	   u, err := url.Parse(raw)
	   if err != nil {
		  return URLDecision{Reason: "parse"}, ErrURL
	   }
	   if strings.EqualFold(u.Scheme, "file") || strings.EqualFold(u.Scheme, "gopher") || u.Scheme == "" {
		   return URLDecision{Reason: "scheme"}, ErrSSRF
	  }
	  host := strings.ToLower(u.Hostname())
	   if host == "" {
		  return URLDecision{Reason: "host"}, ErrURL
	   }
	   if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		   return URLDecision{Reason: "loopback"}, ErrPrivate
	   }
	  if host == "metadata.google.internal" || host == "metadata" || host == "169.254.169.254" {
		  return URLDecision{Reason: "metadata"}, ErrSSRF
	   }
	  if ip := net.ParseIP(host); ip != nil {
		  if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			   return URLDecision{Reason: "private"}, ErrPrivate
		   }
		   if ip.String() == "169.254.169.254" {
			  return URLDecision{Reason: "metadata"}, ErrSSRF
		   }
	   }
	  if strings.Contains(u.Path, "..") {
		   return URLDecision{Reason: "traversal"}, ErrTraversal
	  }
	  if config.CatalogHost(host) {
		  return URLDecision{Allow: true, URL: raw, Host: host}, nil
	   }
	  if config.PublicHost(host) && strings.EqualFold(u.Scheme, "https") {
		   return URLDecision{Allow: true, URL: raw, Host: host}, nil
	  }
	  d, err := rpcallow.Check(raw)
	   if err != nil {
		  return URLDecision{Reason: d.Reason, Host: d.Host}, mapRPC(err)
	  }
	  return URLDecision{Allow: true, URL: raw, Host: d.Host}, nil
   }

  func mapRPC(err error) error {
	  switch err {
	  case rpcallow.ErrPrivate:
		  return ErrPrivate
	  case rpcallow.ErrMeta, rpcallow.ErrScheme:
		   return ErrSSRF
	  default:
		  return ErrURL
	  }
  }

   func CheckPathOrURL(raw string) (string, error) {
	  raw = strings.TrimSpace(raw)
	  if strings.Contains(raw, "://") {
		   d, err := CheckURL(raw)
		  if err != nil {
			  return "", err
		   }
		  return d.URL, nil
	  }
	   return Check(raw)
  }
// temporary guard removed after the production check
