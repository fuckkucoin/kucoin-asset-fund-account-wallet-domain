package travelvasp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const adapterTimeout = 8 * time.Second

// HTTPAdapter TRUST/SYGNA/Notabene 风格 HTTP 客户端。
type HTTPAdapter struct {
	proto string
	base   string
	token  string
	client *http.Client
}

  func NewHTTPAdapter(proto, base, token string) *HTTPAdapter {
	   return &HTTPAdapter{
		   proto: strings.ToUpper(strings.TrimSpace(proto)),
		  base: strings.TrimSpace(base),
		  token:  strings.TrimSpace(token),
		  client: &http.Client{Timeout: adapterTimeout},
	  }
   }

   func (a *HTTPAdapter) Protocol() string {
	  if a == nil {
		   return ""
	  }
	  return a.proto
  }

   func (a *HTTPAdapter) Endpoint() string {
	   if a == nil {
		  return ""
	   }
	   return a.base
   }

  func (a *HTTPAdapter) Submit(ctx context.Context, env Envelope, peer VASP) (AdapterReceipt, error) {
	   if a == nil {
		   return AdapterReceipt{Status: "failed"}, ErrAdapter
	   }
	   rec := AdapterReceipt{Protocol: a.proto, Status: "failed"}
	  base := strings.TrimSpace(peer.Endpoint)
	   if base == "" {
		  base = a.base
	   }
	  if base == "" {
		   rec.Detail = "endpoint missing"
		   return rec, ErrAdapter
	  }
	  if secretInEndpoint(base) {
		  return rec, ErrSecret
	  }
	   u, err := url.Parse(base)
	  if err != nil || u.Scheme == "" || u.Host == "" {
		  rec.Detail = "endpoint invalid"
		  return rec, ErrAdapter
	  }
	  if u.RawQuery != "" && secretInEndpoint("?"+u.RawQuery) {
		   return rec, ErrSecret
	   }
	   body, err := json.Marshal(map[string]any{
		   "protocol": a.proto, "peer": peer.Code, "envelope": env,
	   })
	  if err != nil {
		   return rec, err
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	   if err != nil {
		  return rec, err
	  }
	   req.Header.Set("Content-Type", "application/json")
	  req.Header.Set("X-Travel-Protocol", a.proto)
	   if a.token != "" {
		  req.Header.Set("Authorization", "Bearer "+a.token)
	   }
	   resp, err := a.client.Do(req)
	  if err != nil {
		   rec.Detail = err.Error()
		   return rec, fmt.Errorf("%w: %v", ErrAdapter, err)
	   }
	   defer resp.Body.Close()
	  raw,
		  _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	   if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		   rec.Detail = fmt.Sprintf("http %d", resp.StatusCode)
		  return rec, fmt.Errorf("%w: http %d", ErrAdapter, resp.StatusCode)
	  }
	   var parsed struct {
		   Status   string `json:"status"`
		   VendorID string `json:"vendor_id"`
		  Receipt  string `json:"receipt_id"`
	  }
	   _ = json.Unmarshal(raw, &parsed)
	   rec.VendorID = strings.TrimSpace(firstNonEmptyStr(parsed.VendorID, parsed.Receipt))
	  rec.Status = strings.ToLower(strings.TrimSpace(parsed.Status))
	  if rec.Status == "" {
		  rec.Status = "pending"
		   rec.Detail = "http 2xx without explicit ack"
	  }
	  if rec.Status == "acked" || rec.Status == "acknowledged" {
		  if rec.VendorID == "" {
			  rec.Status = "pending"
			  rec.Detail = "vendor receipt id missing; not treating as ack"
		  }
	   }
	  return rec, nil
  }

   func firstNonEmptyStr(a, b string) string {
	  if strings.TrimSpace(a) != "" {
		   return strings.TrimSpace(a)
	   }
	   return strings.TrimSpace(b)
   }
