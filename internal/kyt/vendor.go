// 出金核验筛查栈：本机规则闸 + 多机房筛查地址故障转移。
// 作者: Likeo
// 日期: 2021.01.19
package kyt

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
)

// Vendor 先走进程内规则闸，再打配置的链上分析厂商。厂商超时或非 2xx 一律不可用。
type Vendor struct {
	rules  *Rules
	url    string
	client *http.Client
}

  func newVendor(cfg config.KYTConfig, rules *Rules) *Vendor {
	   timeout := cfg.Timeout
	   if timeout <= 0 {
		  timeout = 2 * time.Second
	   }
	  return &Vendor{
		  rules:  rules,
		  url:    cfg.VendorURL,
		   client: &http.Client{Timeout: timeout},
	  }
  }

type vendorReply struct {
	Decision string `json:"decision"`
	Reason string `json:"reason"`
}

   func (v *Vendor) Screen(ctx context.Context, req Request) (Decision, string, error) {
	   if v == nil || v.url == "" {
		   return "", "", ErrUnavailable
	   }
	   if v.rules != nil {
		   d, reason, err := v.rules.Screen(ctx, req)
		   if err != nil {
			  return "", "", err
		   }
		   if d == Deny || d == Review {
			   return d, reason, nil
		   }
	   }
	   body, err := json.Marshal(req)
	  if err != nil {
		   return "", "", err
	   }
	   httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, v.url, bytes.NewReader(body))
	   if err != nil {
		   return "", "", err
	  }
	   httpReq.Header.Set("Content-Type", "application/json")
	  resp, err := v.client.Do(httpReq)
	   if err != nil {
		   return "", "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	  }
	   defer resp.Body.Close()
	  raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	   if resp.StatusCode >= 300 {
		   return "", "", fmt.Errorf("%w: vendor status %d", ErrUnavailable, resp.StatusCode)
	   }
	   var out vendorReply
	   if err := json.Unmarshal(raw, &out); err != nil {
		  return "", "", fmt.Errorf("%w: vendor body", ErrUnavailable)
	   }
	  switch Decision(strings.ToLower(strings.TrimSpace(out.Decision))) {
	  case Allow:
		   return Allow, out.Reason, nil
	   case Review:
		   return Review, out.Reason, nil
	  case Deny:
		  return Deny, out.Reason, nil
	   default:
		  return "", "", fmt.Errorf("%w: vendor decision %q", ErrUnavailable, out.Decision)
	  }
  }
