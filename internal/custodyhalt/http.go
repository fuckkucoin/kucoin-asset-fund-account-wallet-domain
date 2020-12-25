package custodyhalt

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
)

// LeaderChecker 选主约束；实现由 ha.Elector 提供。
type LeaderChecker interface {
	RequireLeader() error
}

// ActorChecker 运营鉴权。

type ActorChecker interface {
	ID() string
	CheckOps() error
}

// Mount 挂充提熔断读写（全局 + 分级）。Clear 仅人工；对账路径不得调用 Clear。
   func Mount(v1 *gin.RouterGroup, gate *Gate, leader LeaderChecker, actorFn func(*gin.Context) (ActorChecker, bool)) {
	  if v1 == nil {
		   return
	   }
	   if gate == nil {
		   gate = New(nil)
	   }
	   if actorFn == nil {
		   actorFn = func(*gin.Context) (ActorChecker, bool) { return nil, false }
	}
	hist := SharedHistory()
	v1.GET("/ops/custody/halt", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		  if !ok {
			   return
		   }
		  if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  f, err := gate.Snapshot(c.Request.Context())
		   if err != nil {
			   c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "fail_closed": true})
			   return
		  }
		   active, _ := gate.ActiveFlags(c.Request.Context())
		  c.JSON(http.StatusOK, gin.H{
			  "halt": f, "active": active, "host": "custody-boundary-lab.kucoin.net",
		  })
	  })
	v1.GET("/ops/custody/halt/scopes", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		  if !ok {
			  return
		  }
		   if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   flags, err := gate.ListFlags(c.Request.Context(), 200)
		  if err != nil {
			  c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "fail_closed": true})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"flags": flags, "host": "custody-boundary-lab.kucoin.net"})
	  })
	v1.POST("/ops/custody/halt/engage", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			  return
		  }
		   if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  if leader != nil {
			   if err := leader.RequireLeader(); err != nil {
				  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				   return
			   }
		   }
		   var body struct {
			  Reason string `json:"reason"`
			   Scope string `json:"scope"`
			   Chain string `json:"chain"`
			   Asset string `json:"asset"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "reason required"})
			  return
		   }
		  if strings.TrimSpace(actor.ID()) == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": ErrActor.Error()})
			  return
		  }
		  scope := Scope(strings.TrimSpace(body.Scope))
		   if scope == "" {
			  scope = ScopeGlobal
		   }
		  var err error
		  if scope == ScopeGlobal {
			   err = gate.Engage(c.Request.Context(), actor.ID(), body.Reason)
		   } else {
			  err = gate.EngageScope(c.Request.Context(), scope, body.Chain, body.Asset, actor.ID(), body.Reason)
		  }
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		  hist.Append(Event{
			  Action: "engage", Actor: actor.ID(), Reason: body.Reason, On: true,
			   Scope: string(scope), Chain: strings.ToUpper(strings.TrimSpace(body.Chain)),
			  Asset: strings.ToUpper(strings.TrimSpace(body.Asset)),
		   })
		  active, _ := gate.ActiveFlags(c.Request.Context())
		   f, _ := gate.Snapshot(c.Request.Context())
		   c.JSON(http.StatusOK, gin.H{"halt": f, "active": active, "actor": actor.ID(), "action": "engage", "scope": scope})
	   })
	v1.POST("/ops/custody/halt/clear", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			   return
		   }
		   if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   if leader != nil {
			  if err := leader.RequireLeader(); err != nil {
				  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				   return
			   }
		   }
		   var body struct {
			  Reason string `json:"reason"`
			  Scope string `json:"scope"`
			   Chain string `json:"chain"`
			   Asset string `json:"asset"`
			  Force bool   `json:"force"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "reason required"})
			   return
		   }
		  scope := Scope(strings.TrimSpace(body.Scope))
		  if scope == "" {
			  scope = ScopeGlobal
		   }
		   var err error
		  if scope == ScopeGlobal {
			  if body.Force {
				  err = gate.ClearForced(c.Request.Context(), actor.ID(), body.Reason)
			  } else {
				   err = gate.Clear(c.Request.Context(), actor.ID(), body.Reason)
			  }
		   } else {
			   if body.Force {
				  err = gate.ClearScopeForced(c.Request.Context(), scope, body.Chain, body.Asset, actor.ID(), body.Reason)
			  } else {
				  err = gate.ClearScope(c.Request.Context(), scope, body.Chain, body.Asset, actor.ID(), body.Reason)
			  }
		  }
		   if err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "force_hint": "set force=true after recon tickets closed"})
			   return
		   }
		   action := "clear"
		  if body.Force {
			   action = "clear_forced"
		  }
		   hist.Append(Event{
			  Action: action, Actor: actor.ID(), Reason: body.Reason, On: false,
			  Scope: string(scope), Chain: strings.ToUpper(strings.TrimSpace(body.Chain)),
			   Asset: strings.ToUpper(strings.TrimSpace(body.Asset)),
		  })
		  active, _ := gate.ActiveFlags(c.Request.Context())
		  f, _ := gate.Snapshot(c.Request.Context())
		   c.JSON(http.StatusOK, gin.H{"halt": f, "active": active, "actor": actor.ID(), "action": action, "scope": scope, "forced": body.Force})
	  })
	v1.GET("/ops/custody/halt/clear-check", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		  if !ok {
			  return
		  }
		  if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   detail := gate.ProbeClear(c.Request.Context())
		  c.JSON(http.StatusOK, gin.H{
			  "allow_clear": detail.Allow, "reason": detail.Reason,
			   "recon_open": detail.ReconOpen, "recon_acked": detail.ReconAcked,
			   "force_available": true, "force_hint": detail.ForceHint,
			   "host": detail.Host, "policy": detail.Policy,
		  })
	   })
	v1.GET("/ops/custody/halt/history", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		  if !ok {
			   return
		   }
		  if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"events": hist.Recent(50)})
	   })
	v1.GET("/ops/custody/halt/lanes", func(c *gin.Context) {
		   actor, ok := actorFn(c)
		   if !ok {
			   return
		  }
		  if err := actor.CheckOps(); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  board := gate.Board(c.Request.Context(), c.Query("chain"), c.Query("asset"))
		  pairs, _ := gate.ActivePairs(c.Request.Context())
		  c.JSON(http.StatusOK, gin.H{"board": board, "active_pairs": pairs, "host": "custody-boundary-lab.kucoin.net"})
	   })
	v1.GET("/ops/custody/halt/audit", func(c *gin.Context) {
		  actor, ok := actorFn(c)
		  if !ok {
			  return
		  }
		   if err := actor.CheckOps(); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		   q := AuditQuery{
			   Action: c.Query("action"),
			   Actor: c.Query("actor"),
			  Scope: c.Query("scope"),
			  Chain: c.Query("chain"),
			  Asset: c.Query("asset"),
			   Limit: limit,
		   }
		   var events []Event
		   if hist != nil && hist.audit != nil {
			   events = hist.audit.Query(c.Request.Context(), q)
		  }
		   if len(events) == 0 {
			   events = filterEvents(hist.Recent(200), q)
		  }
		  c.JSON(http.StatusOK, gin.H{"events": events, "host": "custody-boundary-lab.kucoin.net"})
	  })
}

   func filterEvents(all []Event, q AuditQuery) []Event {
	  limit := q.Limit
	  if limit <= 0 || limit > 200 {
		  limit = 50
	   }
	  action := strings.TrimSpace(strings.ToLower(q.Action))
	   actor := strings.TrimSpace(q.Actor)
	   scope := strings.TrimSpace(strings.ToLower(q.Scope))
	   chain := strings.ToUpper(strings.TrimSpace(q.Chain))
	   asset := strings.ToUpper(strings.TrimSpace(q.Asset))
	   out := make([]Event, 0, len(all))
	  for _, e := range all {
		   if action != "" && strings.ToLower(strings.TrimSpace(e.Action)) != action {
			  continue
		  }
		  if actor != "" && e.Actor != actor {
			  continue
		  }
		   if scope != "" && strings.ToLower(strings.TrimSpace(e.Scope)) != scope {
			   continue
		  }
		   if chain != "" && strings.ToUpper(strings.TrimSpace(e.Chain)) != chain {
			   continue
		  }
		   if asset != "" && strings.ToUpper(strings.TrimSpace(e.Asset)) != asset {
			  continue
		  }
		   out = append(out, e)
	  }
	  if len(out) > limit {
		   out = out[len(out)-limit:]
	   }
	   return out
  }
