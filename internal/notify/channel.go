package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
)

// HTTPSink 向公开邮件/推送主机投递。缺密钥不得假装已送达。

type HTTPSink struct {
	NameVal string
	URL     string
	Key    string
	Client *http.Client
}

  func NewHTTPSink(name, rawURL, key string) *HTTPSink {
	   return &HTTPSink{
		  NameVal: strings.TrimSpace(name),
		   URL:     strings.TrimSpace(rawURL),
		   Key:     strings.TrimSpace(key),
		   Client: &http.Client{Timeout: 3 * time.Second},
	  }
   }

  func (s *HTTPSink) Name() string {
	  if s == nil || s.NameVal == "" {
		  return "http"
	  }
	   return s.NameVal
   }

   func (s *HTTPSink) Accept(ctx context.Context, d Delivery) error {
	  if s == nil || s.Key == "" {
		   return ErrUnsigned
	   }
	  if !config.AllowOutbound(s.URL) {
		   return ErrDest
	  }
	  body, err := json.Marshal(map[string]string{
		   "kind": d.Kind, "key": d.Key, "payload": d.Payload, "channel": s.Name(),
	   })
	   if err != nil {
		  return err
	  }
	   req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	   if err != nil {
		  return err
	   }
	  req.Header.Set("Content-Type", "application/json")
	   req.Header.Set("Authorization", "Bearer "+s.Key)
	   resp, err := s.Client.Do(req)
	  if err != nil {
		   return ErrSink
	  }
	   defer resp.Body.Close()
	   if resp.StatusCode >= 300 {
		  return ErrSink
	  }
	  return nil
   }
