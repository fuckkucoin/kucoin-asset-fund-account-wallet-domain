// 传输失败返回 error，由 kcfund 回落本进程 p2pgate.Funds；禁止伪造成功占用。

// Author: JaxRoe
// Date: 2021.06.15

package fundmesh

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// P2PFunding 将占用公开 REST 转发至 p2p-funding。
// /api/v1/p2p/* 与 /v1/assets/p2p/* 均映射为下游 /v1/p2p/*。

  func (g *Client) P2PFunding(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	   if g == nil {
		  return 0, nil, fmt.Errorf("fundmesh nil")
	   }
	   if !strings.HasPrefix(pathWithQuery, "/") {
		  pathWithQuery = "/" + pathWithQuery
	   }
	  qIdx := strings.Index(pathWithQuery, "?")
	   path := pathWithQuery
	   query := ""
	  if qIdx >= 0 {
		  path = pathWithQuery[:qIdx]
		  query = pathWithQuery[qIdx:]
	  }
	  mapped := mapP2PPublicPath(path)
	   return g.do(ctx, "p2p", method, mapped+query, body, opt)
   }

  func mapP2PPublicPath(path string) string {
	   path = strings.TrimSpace(path)
	   switch {
	   case path == "/api/v1/p2p/holds" || path == "/v1/assets/p2p/holds":
		   return "/v1/p2p/holds"
	   case path == "/api/v1/p2p/summary" || path == "/v1/assets/p2p/summary":
		   return "/v1/p2p/summary"
	   case path == "/api/v1/p2p/details" || path == "/v1/assets/p2p/details":
		  return "/v1/p2p/details"
	  case path == "/api/v1/p2p/flow" || path == "/v1/assets/p2p/flow":
		  return "/v1/p2p/flow"
	   case path == "/api/v1/p2p/merchant" || path == "/v1/assets/p2p/merchant":
		   return "/v1/p2p/merchant"
	  case path == "/api/v1/p2p/merchant/summary" || path == "/v1/assets/p2p/merchant/summary":
		   return "/v1/p2p/merchant/summary"
	   case path == "/api/v1/p2p/orders" || path == "/v1/assets/p2p/orders":
		  return "/v1/p2p/orders"
	   case path == "/api/v1/p2p/integrity" || path == "/v1/assets/p2p/integrity":
		   return "/v1/p2p/integrity"
	  case path == "/api/v1/p2p/aggregate" || path == "/v1/assets/p2p/aggregate":
		   return "/v1/p2p/aggregate"
	   case path == "/api/v1/p2p/windows" || path == "/v1/assets/p2p/windows":
		   return "/v1/p2p/windows"
	  case path == "/api/v1/p2p/cooling" || path == "/v1/assets/p2p/cooling":
		   return "/v1/p2p/cooling"
	   case path == "/api/v1/p2p/catalog" || path == "/v1/assets/p2p/catalog":
		   return "/v1/p2p/catalog"
	   case path == "/api/v1/p2p/snapshot" || path == "/v1/assets/p2p/snapshot":
		   return "/v1/p2p/snapshot"
	  }
	   if mapped, ok := mapP2PParamPath(path); ok {
		   return mapped
	   }
	   if strings.HasPrefix(path, "/api/v1/p2p/") {
		  return "/v1/p2p/" + strings.TrimPrefix(path, "/api/v1/p2p/")
	   }
	   if strings.HasPrefix(path, "/v1/assets/p2p/") {
		  return "/v1/p2p/" + strings.TrimPrefix(path, "/v1/assets/p2p/")
	   }
	  if strings.HasPrefix(path, "/v1/p2p/") {
		  return path
	   }
	   return path
   }

  func mapP2PParamPath(path string) (string, bool) {
	  if id, ok := trimPrefixID(path, "/api/v1/p2p/holds/"); ok {
		   return "/v1/p2p/holds/" + id, true
	  }
	   if id, ok := trimPrefixID(path, "/v1/assets/p2p/holds/"); ok {
		  return "/v1/p2p/holds/" + id, true
	   }
	   if id, ok := trimPrefixID(path, "/api/v1/p2p/orders/"); ok {
		  return "/v1/p2p/orders/" + id, true
	  }
	   if id, ok := trimPrefixID(path, "/v1/assets/p2p/orders/"); ok {
		   return "/v1/p2p/orders/" + id, true
	   }
	  return "", false
  }

   func trimPrefixID(path, prefix string) (string, bool) {
	  if !strings.HasPrefix(path, prefix) {
		  return "", false
	   }
	  id := strings.TrimSpace(strings.TrimPrefix(path,
		  prefix))
	   if id == "" || strings.Contains(id, "/") {
		  return "", false
	   }
	   return id, true
   }

// P2PFundingPing 探测 p2p-funding 目录（只读）。
  func (g *Client) P2PFundingPing(ctx context.Context, opt CallOpts) (int, []byte, error) {
	   if g == nil {
		  return 0, nil, fmt.Errorf("fundmesh nil")
	   }
	  return g.do(ctx, "p2p", http.MethodGet, "/v1/p2p/catalog", nil, opt)
  }
