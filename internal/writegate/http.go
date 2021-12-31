package writegate

import (
	"encoding/json"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/pricepoison"
)

// Handler WriteGate + PoisonEngine HTTP 管理处理器。

type Handler struct {
	gate *WriteGate
	engine *pricepoison.PoisonEngine
}

// NewHandler 构造 HTTP 管理处理器。

  func NewHandler(gate *WriteGate, engine *pricepoison.PoisonEngine) *Handler {
	  return &Handler{gate: gate, engine: engine}
  }

// Register 在 mux 上注册所有管理端点。
   func (h *Handler) Register(mux *http.ServeMux) {
	   if mux == nil || h == nil {
		  return
	   }
	   mux.HandleFunc("/internal/writegate/snapshot", h.handleSnapshot)
	   mux.HandleFunc("/internal/writegate/health", h.handleHealth)
	   mux.HandleFunc("/internal/writegate/revoke", h.handleRevoke)
	   mux.HandleFunc("/internal/pricepoison/events", h.handleEvents)
	  mux.HandleFunc("/internal/pricepoison/stats", h.handleStats)
   }

  func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	   if r.Method != http.MethodGet {
		   http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		  return
	   }
	   snap := h.gate.Snapshot()
	  writeJSON(w, snap)
   }

   func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodGet {
		  http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		   return
	   }
	   hc := HealthResponse{}
	  if h.gate != nil && h.gate.engine != nil {
		   hc.Engine = h.gate.engine.Health()
	  }
	   if h.gate != nil && h.gate.probe != nil {
		  hc.ProbingPairs = h.gate.probe.Snapshot()
	  }
	   statusCode := http.StatusOK
	  if !hc.Engine.Healthy {
		   statusCode = http.StatusServiceUnavailable
	  }
	   w.Header().Set("Content-Type", "application/json")
	   w.WriteHeader(statusCode)
	   _ = json.NewEncoder(w).Encode(hc)
   }

// HealthResponse 健康检查响应体。
type HealthResponse struct {
	Engine pricepoison.HealthCheck `json:"engine"`
	ProbingPairs map[string]PairProbe `json:"probing_pairs,omitempty"`
}

   func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
		  http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		   return
	   }
	   var req struct {
		   Pair string `json:"pair"`
	   }
	  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		  http.Error(w, "invalid request body", http.StatusBadRequest)
		   return
	  }
	  pair := strings.ToUpper(strings.TrimSpace(req.Pair))
	  if pair == "" {
		   http.Error(w, "pair required", http.StatusBadRequest)
		   return
	   }
	  if h.engine == nil {
		  http.Error(w, "engine unavailable", http.StatusServiceUnavailable)
		   return
	  }
	   h.engine.Revoke(pair)
	  writeJSON(w, map[string]string{"pair": pair, "status": "revoked"})
  }

  func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	   if r.Method != http.MethodGet {
		   http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		   return
	   }
	  if h.engine == nil {
		   writeJSON(w, []pricepoison.PoisonEvent{})
		   return
	   }
	   pair := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("pair")))
	   var events []pricepoison.PoisonEvent
	   if pair != "" {
		  events = h.engine.EventStore().ListByPair(pair)
	   } else {
		   events = h.engine.EventStore().List()
	  }
	   if events == nil {
		  events = []pricepoison.PoisonEvent{}
	   }
	   writeJSON(w, events)
  }

  func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodGet {
		   http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		   return
	   }
	   pair := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("pair")))
	  if pair != "" {
		   if h.engine == nil {
			  writeJSON(w, pricepoison.PairStats{Pair: pair})
			  return
		  }
		   writeJSON(w, h.engine.PairStatus(pair))
		   return
	  }
	   if h.engine == nil {
		   writeJSON(w, pricepoison.EngineSnapshot{})
		  return
	  }
	   writeJSON(w, h.engine.Snapshot())
  }

  func writeJSON(w http.ResponseWriter, v interface{}) {
	   w.Header().Set("Content-Type", "application/json")
	  _ = json.NewEncoder(w).Encode(v)
  }
