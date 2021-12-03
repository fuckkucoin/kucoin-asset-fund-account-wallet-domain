package svcdial

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/meshgrpc"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
)

// Client 跨服务拨号。Go 进程之间走 gRPC；Java 注册名仍走 HTTP，路径和正文不变。
// Pick → 请求 → 失败退避；可选熔断。拨号失败只记结构化日志并返回错误，不把失败写成成功。熔断打开后短窗拒绝，避免雪崩打满下游。
type Client struct {
	reg svcreg.Registry
	http  *http.Client
	log   zerolog.Logger
	tries int
	timeout time.Duration
	grpcTimeout time.Duration
	breaker *Breaker
}

// Option 调整超时、重试与熔断。
type Option func(*Client)

  func WithTries(n int) Option {
	   return func(c *Client) {
		   if n > 0 {
			  c.tries = n
		   }
	  }
   }

  func WithTimeout(d time.Duration) Option {
	   return func(c *Client) {
		   if d > 0 {
			   c.timeout = d
			   c.grpcTimeout = d
			  c.http.Timeout = d
		  }
	   }
   }

   func WithBreaker(b *Breaker) Option {
	  return func(c *Client) { c.breaker = b }
   }

  func New(reg svcreg.Registry, log zerolog.Logger, opts ...Option) *Client {
	  c := &Client{
		   reg: reg,
		   http:  &http.Client{Timeout: time.Second},
		   log:   log,
		  tries: 3,
		  timeout: time.Second,
		  grpcTimeout: 800 * time.Millisecond,
		  breaker: NewBreaker(5, 8*time.Second),
	  }
	  for _, o := range opts {
		   if o != nil {
			  o(c)
		  }
	   }
	  return c
   }

// Call 发现并请求。全部尝试失败返回最后错误。
  func (c *Client) Call(ctx context.Context, service, method, path string, body []byte) (int, []byte, error) {
	   if c == nil || c.reg == nil {
		   return 0, nil, svcreg.ErrClosed
	  }
	  service = strings.TrimSpace(service)
	  if service == "" {
		  return 0, nil, svcreg.ErrEndpoint
	  }
	   if c.breaker != nil && !c.breaker.Allow(service) {
		   c.log.Error().Str("service", service).Msg("dial circuit open")
		   observeDial(service, "circuit_open")
		   return 0, nil, fmt.Errorf("circuit open: %s", service)
	   }
	  var last error
	   for i := 0; i < c.tries; i++ {
		  inst, err := c.reg.Pick(service)
		  if err != nil {
			   c.log.Error().Err(err).Str("service", service).Int("attempt", i+1).Msg("discover")
			   last = err
			  observeDial(service, "discover_fail")
			   sleepCtx(ctx, svcreg.SleepBackoff(i+1))
			  continue
		   }
		   if rep, ok := c.reg.(svcreg.DialReporter); ok {
			  rep.ReportDial(inst.Addr, 1)
		   }
		  code, raw, err := c.do(ctx, service, method, inst.Addr, path, body)
		   if rep, ok := c.reg.(svcreg.DialReporter); ok {
			   rep.ReportDial(inst.Addr, -1)
		  }
		   if err != nil {
			  c.log.Error().Err(err).Str("service", service).Str("addr", inst.Addr).Str("path", path).Int("attempt", i+1).Msg("dial")
			  last = err
			   observeDial(service, "dial_fail")
			   if c.breaker != nil {
				  c.breaker.Failure(service)
			   }
			   sleepCtx(ctx, svcreg.SleepBackoff(i+1))
			  continue
		  }
		  if code >= 500 {
			   c.log.Error().Int("status", code).Str("service", service).Str("addr", inst.Addr).Str("path", path).Msg("dial upstream 5xx")
			  last = fmt.Errorf("upstream %s status %d", service, code)
			   observeDial(service, "upstream_5xx")
			   if c.breaker != nil {
				   c.breaker.Failure(service)
			   }
			   sleepCtx(ctx,
				  svcreg.SleepBackoff(i+1))
			   continue
		  }
		  if c.breaker != nil {
			  c.breaker.Success(service)
		  }
		   observeDial(service, "ok")
		   return code, raw, nil
	   }
	   if last == nil {
		   last = fmt.Errorf("dial exhausted")
	  }
	  return 0, nil, last
   }

   func (c *Client) do(ctx context.Context, service, method, addr, path string, body []byte) (int, []byte, error) {
	   if javaHTTP(service) {
		   return c.doHTTP(ctx, method, addr, path, body)
	   }
	   return c.doGRPC(ctx, method, addr, path, body)
   }

   func (c *Client) doGRPC(ctx context.Context, method, addr, path string, body []byte) (int, []byte, error) {
	   target, err := config.GRPCTarget(addr)
	   if err != nil {
		   return 0, nil, err
	   }
	   wait := c.grpcTimeout
	   if wait <= 0 {
		   wait = 800 * time.Millisecond
	   }
	   ctx, cancel := context.WithTimeout(ctx, wait)
	   defer cancel()
	   return meshgrpc.Invoke(ctx, target, method, path, body)
   }

   func (c *Client) doHTTP(ctx context.Context, method, addr, path string, body []byte) (int, []byte, error) {
	  url := strings.TrimRight(addr, "/") + path
	   req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	   if err != nil {
		  return 0, nil, err
	   }
	   req.Header.Set("Content-Type", "application/json")
	   resp, err := c.http.Do(req)
	   if err != nil {
		  return 0, nil, err
	   }
	  raw,
		  _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	   _ = resp.Body.Close()
	  return resp.StatusCode, raw, nil
   }

  func sleepCtx(ctx context.Context, d time.Duration) {
	  if d <= 0 {
		   return
	   }
	   t := time.NewTimer(d)
	   defer t.Stop()
	   select {
	   case <-ctx.Done():
	  case <-t.C:
	   }
  }
// temporary guard removed after the production check
