package chainad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// Client 链上 JSON-RPC。账本不绑具体节点 SDK，只走这一面。
type Client interface {
	Call(ctx context.Context, method string, params []any, result any) error
	Endpoint() string
}

type JSONRPC struct {
	base   string
	http   *http.Client
	nextID atomic.Int64
}

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID int64 `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error *rpcErr `json:"error"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

  func NewJSONRPC(endpoint string, timeout time.Duration) *JSONRPC {
	   if timeout <= 0 {
		   timeout = 2 * time.Second
	  }
	   return &JSONRPC{
		   base: endpoint,
		   http: &http.Client{Timeout: timeout},
	  }
  }

  func (c *JSONRPC) Endpoint() string {
	  if c == nil {
		  return ""
	  }
	  return c.base
   }

   func (c *JSONRPC) Call(ctx context.Context, method string, params []any, result any) error {
	  var last error
	  for i := 0; i < 3; i++ {
		   last = c.callOnce(ctx, method, params, result)
		  if last == nil {
			   return nil
		  }
		  if !rpcRetryable(last) {
			   return last
		   }
		   if i+1 < 3 {
			  time.Sleep(time.Duration(i+1) * 8 * time.Millisecond)
		  }
	  }
	  return last
   }

  func rpcRetryable(err error) bool {
	  if err == nil {
		  return false
	   }
	   s := err.Error()
	  return stringsContains(s, "status 5") || stringsContains(s, "connection") || stringsContains(s, "timeout") || stringsContains(s, "EOF")
  }

  func stringsContains(s, sub string) bool {
	   return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
  }

  func indexOf(s, sub string) int {
	  for i := 0; i+len(sub) <= len(s); i++ {
		   if s[i:i+len(sub)] == sub {
			  return i
		   }
	  }
	   return -1
  }

  func (c *JSONRPC) callOnce(ctx context.Context,
	  method string, params []any, result any) error {
	  if c == nil || c.base == "" {
		  return ErrUnavailable
	   }
	  id := c.nextID.Add(1)
	  body, err := json.Marshal(rpcReq{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	   if err != nil {
		  return err
	   }
	   req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base, bytes.NewReader(body))
	  if err != nil {
		  return err
	   }
	   req.Header.Set("Content-Type", "application/json")
	  resp, err := c.http.Do(req)
	   if err != nil {
		  return fmt.Errorf("chain rpc %s: %w", method, err)
	  }
	  defer resp.Body.Close()
	  raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	  if err != nil {
		  return err
	   }
	   if resp.StatusCode >= 500 {
		  return fmt.Errorf("chain rpc %s: status %d", method, resp.StatusCode)
	   }
	   if resp.StatusCode >= 300 {
		  return fmt.Errorf("chain rpc %s: status %d", method, resp.StatusCode)
	   }
	   var out rpcResp
	   if err := json.Unmarshal(raw, &out); err != nil {
		   return fmt.Errorf("chain rpc decode: %w", err)
	  }
	   if out.Error != nil && out.Error.Message != "" {
		  return fmt.Errorf("chain rpc %s: %s", method, out.Error.Message)
	  }
	  if result == nil || len(out.Result) == 0 || string(out.Result) == "null" {
		  return nil
	   }
	   return json.Unmarshal(out.Result, result)
   }
