// @FileName: service.go
// @Summary: 资金事件通知投递服务实现。
// @Author: Cason
// @Created: 2021.09.02

package notify

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Service struct {
	cons *Consumer
	mem  *MemSink
}

   func NewService() *Service {
	  mem := NewMemSink("primary")
	   return &Service{cons: NewMem(mem), mem: mem}
   }

  func NewServiceWith(kv persist.KV) *Service {
	   return NewServiceWithChannels(kv, "", "", "")
  }

// NewServiceWithChannels 入队仍写 KV；配置了厂商密钥时 Flush 走公开邮件/推送主机。

   func NewServiceWithChannels(kv persist.KV, emailURL, pushURL, key string) *Service {
	   mem := NewMemSink("primary")
	   sinks := []Sink{mem}
	  if strings.TrimSpace(key) != "" {
		  if emailURL != "" {
			  sinks = append(sinks, NewHTTPSink("email", emailURL, key))
		  }
		  if pushURL != "" {
			   sinks = append(sinks, NewHTTPSink("push", pushURL, key))
		   }
	   }
	   var sink Sink = mem
	   if len(sinks) > 1 {
		  sink = NewMulti(sinks...)
	   }
	  return &Service{cons: New(kv, sink), mem: mem}
   }

  func FromConfig(kv persist.KV, cfg *config.Config) *Service {
	  if cfg == nil {
		  return NewServiceWith(kv)
	   }
	  return NewServiceWithChannels(kv, cfg.Funds.NotifyEmailURL, cfg.Funds.NotifyPushURL, cfg.Funds.NotifyAPIKey)
  }

   func (s *Service) Consumer() *Consumer { return s.cons }

   func (s *Service) Delivered() []Delivery { return s.mem.List() }

  func (s *Service) Publish(ctx context.Context, key, kind, payload string) error {
	  _, err := s.cons.Enqueue(ctx, key, kind, payload)
	   return err
  }

  func (s *Service) Flush(ctx context.Context) Report {
	  return s.cons.Tick(ctx)
  }
