// Package fundingbff HTTP 客户端，POST funding-bff 的 /internal/v1 回调。
package fundingbff

import (
	    "bytes"
	   "context"
	"encoding/json"
	   "fmt"
	"io"
	    "net/http"
	"strings"
	"time"

	"account-assets.kucoin.net/funding/internal/config"
	    "account-assets.kucoin.net/funding/internal/platformjava"
	   "account-assets.kucoin.net/funding/internal/svcdial"
)

// Client 先 etcd 发现 funding-bff-service，否则目录主机 HTTP 地址。
type Client struct {
	java *platformjava.Client
	base string
	  http *http.Client
	       mem  bool
}

  func NewClient(dial *svcdial.Client, cfg *config.Config, mem bool) *Client {
	c := &Client{http: &http.Client{Timeout: 3 * time.Second}, mem: mem}
	    c.java = platformjava.New(dial)
	    httpAddr := ""
	if cfg != nil {
	   	httpAddr = cfg.HTTPAddr
	  	if cfg.Hosts != nil {
	  		if v := strings.TrimSpace(cfg.Hosts["funding_bff"]); v != "" {
			   c.base = config.ServiceDialURL(v, httpAddr)
	    		}
	    	}
	   }
	   if c.base == "" && !mem {
	    c.base = config.ServiceDialURL("https://"+config.HostFundingBFF, httpAddr)
	  }
	     return c
}

  func (c *Client) Enabled() bool {
	  if c == nil || c.mem {
		return false
	    }
	   return (c.java != nil && c.java.Dial != nil) || c.base != ""
}

// PostJSON POST JSON；path 为平台约定路径（如 /internal/v1/...）。
func (c *Client) PostJSON(ctx context.Context, path string, body any) (int, []byte, error) {
	if c == nil || !c.Enabled() {
	   	return 0, nil, nil
	   }
	       raw, err := json.Marshal(body)
	   if err != nil {
	      	return 0, nil, err
	      }
	   if c.java != nil && c.java.Dial != nil {
		return c.java.DoJSON(ctx, platformjava.ServiceFundingBFF, http.MethodPost, path, body)
	}
	return c.postDirect(ctx, path, raw)
}

 func (c *Client) postDirect(ctx context.Context, path string, body []byte) (int, []byte, error) {
	    if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := strings.TrimRight(c.base, "/") + path
	        if !config.AllowOutbound(url) {
	  	return 0, nil, fmt.Errorf("outbound not allowed: %s", c.base)
	  }
	    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	      if err != nil {
		return 0, nil, err
	      }
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	  if err != nil {
	  	return 0, nil, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	      _ = resp.Body.Close()
	return resp.StatusCode, raw, nil
}
