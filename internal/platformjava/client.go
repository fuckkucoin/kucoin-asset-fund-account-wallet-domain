package platformjava

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/svcdial"
)

// Client 经 svcdial 发现 Java 平台服务实例。
type Client struct {
	Dial *svcdial.Client
}

  func New(d *svcdial.Client) *Client {
	  return &Client{Dial: d}
   }

// DoJSON 调用已注册的 Java 服务；service 见本包 Service* 常量。
  func (c *Client) DoJSON(ctx context.Context, service, method, path string, in any) (status int, body []byte, err error) {
	   if c == nil || c.Dial == nil {
		   return 0, nil, fmt.Errorf("platformjava: dial unavailable")
	   }
	   service = strings.TrimSpace(service)
	  if service == "" {
		   return 0, nil, fmt.Errorf("platformjava: service required")
	  }
	  method = strings.ToUpper(strings.TrimSpace(method))
	   if method == "" {
		   method = http.MethodGet
	  }
	   if path != "" && !strings.HasPrefix(path, "/") {
		   path = "/" + path
	   }
	  var raw []byte
	  if in != nil && method != http.MethodGet && method != http.MethodHead {
		  raw, err = json.Marshal(in)
		  if err != nil {
			  return 0, nil, err
		  }
	   }
	   return c.Dial.Call(ctx, service, method, path, raw)
  }
