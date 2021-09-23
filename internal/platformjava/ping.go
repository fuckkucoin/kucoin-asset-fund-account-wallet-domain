package platformjava

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
)

// PingResult 单次契约探测结果。

type PingResult struct {
	Service    string
	StatusCode int
	Err error
}

  func (r PingResult) OK() bool {
	  return r.Err == nil && r.StatusCode >= 200 && r.StatusCode < 300
  }

// PingHealth 对单个 Java 服务做 /healthz 探测（经 svcdial 发现）。

   func PingHealth(ctx context.Context, c *Client, service string) PingResult {
	  res := PingResult{Service: strings.TrimSpace(service)}
	  if c == nil || c.Dial == nil {
		   res.Err = fmt.Errorf("platformjava: dial unavailable")
		  return res
	   }
	   if res.Service == "" {
		   res.Err = fmt.Errorf("platformjava: service required")
		   return res
	   }
	  code,
		  _, err := c.DoJSON(ctx, res.Service, http.MethodGet, config.PathHealthz, nil)
	   res.StatusCode = code
	  res.Err = err
	  if err == nil && (code < 200 || code >= 300) {
		  res.Err = fmt.Errorf("platformjava: ping %s status %d", res.Service, code)
	  }
	   return res
  }

// PingCatalog 顺序探测 Catalog 中全部服务；不因单项失败而中断。
  func PingCatalog(ctx context.Context, c *Client) []PingResult {
	  out := make([]PingResult, 0, len(Catalog()))
	   for _, ct := range Catalog() {
		   out = append(out, PingHealth(ctx, c, ct.Service))
	   }
	   return out
  }

// RequireAllOK 用于集成门禁：全部 2xx 才返回 nil。
   func RequireAllOK(results []PingResult) error {
	  var bad []string
	  for _, r := range results {
		  if r.OK() {
			   continue
		   }
		   if r.Err != nil {
			  bad = append(bad, fmt.Sprintf("%s: %v", r.Service, r.Err))
		   } else {
			  bad = append(bad, fmt.Sprintf("%s: status %d", r.Service, r.StatusCode))
		  }
	  }
	  if len(bad) == 0 {
		   return nil
	   }
	   return fmt.Errorf("java platform contract ping failed: %s", strings.Join(bad, "; "))
  }
