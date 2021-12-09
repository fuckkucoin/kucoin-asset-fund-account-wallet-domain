package travelvasp

import (
	"context"
	"os"
	"strings"
)

// AdapterReceipt 厂商投递回执。无 vendor_id 不得当 acked。

type AdapterReceipt struct {
	Protocol string `json:"protocol"`
	Status   string `json:"status"`
	VendorID string `json:"vendor_id,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// OutboundAdapter 出站投递接口。实现必须失败关闭，不得编造 ack。
type OutboundAdapter interface {
	Protocol() string
	Submit(ctx context.Context, env Envelope, peer VASP) (AdapterReceipt, error)
}

// AdapterHub 协议→适配器。缺适配器失败关闭。

type AdapterHub struct {
	clients map[string]OutboundAdapter
}

  func NewAdapterHub() *AdapterHub {
	   return &AdapterHub{clients: map[string]OutboundAdapter{}}
   }

   func (h *AdapterHub) Register(a OutboundAdapter) *AdapterHub {
	   if h == nil || a == nil {
		  return h
	   }
	  if h.clients == nil {
		  h.clients = map[string]OutboundAdapter{}
	  }
	  h.clients[strings.ToUpper(strings.TrimSpace(a.Protocol()))] = a
	   return h
  }

   func (h *AdapterHub) Get(proto string) (OutboundAdapter, bool) {
	   if h == nil {
		   return nil, false
	  }
	  a, ok := h.clients[strings.ToUpper(strings.TrimSpace(proto))]
	  return a, ok && a != nil
   }

   func (h *AdapterHub) List() []string {
	  if h == nil {
		   return nil
	  }
	  out := make([]string, 0, len(h.clients))
	  for k := range h.clients {
		  out = append(out, k)
	   }
	   return out
   }

   func (h *AdapterHub) Submit(ctx context.Context, proto string, env Envelope, peer VASP) (AdapterReceipt, error) {
	   if h == nil {
		   return AdapterReceipt{Protocol: proto, Status: "failed", Detail: "adapter hub missing"}, ErrAdapter
	  }
	   a, ok := h.Get(proto)
	  if !ok {
		   return AdapterReceipt{Protocol: proto, Status: "failed", Detail: "adapter not wired"}, ErrAdapter
	  }
	  rec, err := a.Submit(ctx, env, peer)
	  if err != nil {
		   return rec, err
	   }
	   if rec.Status == "acked" || rec.Status == "acknowledged" {
		   if strings.TrimSpace(rec.VendorID) == "" {
			   rec.Status = "pending"
			   rec.Detail = "vendor receipt id missing; not treating as ack"
		   }
	   }
	   return rec, nil
   }

// NewAdapterHubFromEnv 按环境变量装配 TRUST/SYGNA/NOTABENE HTTP 客户端。

   func NewAdapterHubFromEnv() *AdapterHub {
	   h := NewAdapterHub()
	   token := strings.TrimSpace(os.Getenv("TRAVEL_ADAPTER_TOKEN"))
	  h.Register(NewHTTPAdapter(ProtoTRUST, firstEnv("TRAVEL_TRUST_ENDPOINT", "TRUST_ENDPOINT"), token))
	  h.Register(NewHTTPAdapter(ProtoSYGNA, firstEnv("TRAVEL_SYGNA_ENDPOINT", "SYGNA_ENDPOINT"), token))
	  h.Register(NewHTTPAdapter(ProtoNOTABENE, firstEnv("TRAVEL_NOTABENE_ENDPOINT", "NOTABENE_ENDPOINT"), token))
	  h.Register(NewHTTPAdapter(ProtoCODE, firstEnv("TRAVEL_CODE_ENDPOINT", "CODE_ENDPOINT"), token))
	   return h
  }

   func firstEnv(keys ...string) string {
	   for _, k := range keys {
		  if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			   return v
		  }
	  }
	   return ""
   }

  func (s *Service) BindAdapters(h *AdapterHub) *Service {
	   if s != nil && h != nil {
		  s.adapters = h
	  }
	   return s
  }

   func (s *Service) Adapters() *AdapterHub {
	  if s == nil {
		   return nil
	  }
	  return s.adapters
   }

   func (s *Service) submitAdapter(ctx context.Context, peer VASP, env Envelope) (AdapterReceipt, error) {
	   if s == nil || s.adapters == nil {
		  return AdapterReceipt{Protocol: peer.Protocol, Status: "failed", Detail: "adapter hub missing"}, ErrAdapter
	  }
	  if secretInEndpoint(peer.Endpoint) {
		  return AdapterReceipt{Protocol: peer.Protocol, Status: "failed"}, ErrSecret
	   }
	   return s.adapters.Submit(ctx, peer.Protocol, env, peer)
  }
