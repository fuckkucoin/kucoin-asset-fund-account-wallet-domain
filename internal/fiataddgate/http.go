package fiataddgate

import (
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fiatadd"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
)

// Register 挂载确认闸路由到已有 v1 路由组。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   if v1 == nil || rt == nil {
		   return
	   }
	  book := fiatadd.FromRuntime(rt)
	   signKey := ""
	   if rt.Cfg != nil {
		   signKey = strings.TrimSpace(rt.Cfg.Funds.FiatAPIKey)
	   }
	   kv := rt.KV
	   if kv == nil {
		  kv = persist.NewMemKV()
	  }
	   leases := NewKVLeaseStore(kv)
	   effects := NewKVEffectStore(kv)
	  forks := NewKVForkCaseStore(kv)
	   actor := ""
	   if rt.Cfg != nil {
		  actor = rt.Cfg.SeedUserID
	  }
	   var ledCreator LedgerCreator
	   if rt.Ledger != nil {
		  ledCreator = rt.Ledger
	  }
	  gate := NewGate(book, leases, effects, forks, ledCreator, GateConfig{
		  SignKey: signKey,
		   LeaseTTL: 30 * time.Second,
		   Actor:    actor,
	   })
	   reconciler := NewReconciler(book, forks, effects, nil)
	  opt := callerid.FromRuntime(rt.Mem, "", "", "")
	  if rt.Cfg != nil {
		   opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	   notif := notify.FromConfig(rt.KV, rt.Cfg)
	  // POST /v1/funds/add/:id/gate-confirm
	   v1.POST("/funds/add/:id/gate-confirm", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			  Signature string `json:"signature"`
			   LegacySig bool `json:"legacy_sig"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   sig := strings.TrimSpace(body.Signature)
		   if sig == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "signature required"})
			  return
		  }
		   result, err := gate.GateConfirm(c.Request.Context(), ConfirmRequest{
			  UserID: uid,
			  SessionID: c.Param("id"),
			   Signature: sig,
			  LegacySig: body.LegacySig,
		  })
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   _ = notif.Publish(c.Request.Context(), "fiat:"+result.Session.ID, notify.KindFiatCredited, result.Session.ID)
		   c.JSON(http.StatusOK, gin.H{
			   "session": result.Session,
			   "lease":   result.Lease,
			  "effect": result.Effect,
			   "host":   config.HostFiatOnramp,
		  })
	   })
	  // GET /v1/funds/add/:id/gate-status
	   v1.GET("/funds/add/:id/gate-status", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  sess, err := book.Get(c.Request.Context(), uid, c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   var activeLease *ConfirmLease
		   if l, err := leases.GetActiveBySession(c.Request.Context(), sess.ID); err == nil {
			  activeLease = &l
		   }
		  var forkCase *FiatAddForkCase
		  if fc, err := forks.GetOpenCase(c.Request.Context(), sess.ID); err == nil {
			   forkCase = &fc
		   }
		   c.JSON(http.StatusOK, gin.H{
			   "session": sess,
			   "active_lease": activeLease,
			   "fork_case":    forkCase,
		   })
	  })
	  // GET /v1/funds/add/:id/reconcile (ops)
	  v1.GET("/funds/add/:id/reconcile", func(c *gin.Context) {
		  res, err := reconciler.Reconcile(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"reconcile": res})
	  })
	   // GET /v1/funds/add/fork-cases (ops)
	  v1.GET("/funds/add/fork-cases", func(c *gin.Context) {
		  list, err := forks.ListOpen(c.Request.Context(), 50)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"fork_cases": list, "count": len(list)})
	  })
	   // POST /v1/funds/add/fork-cases/:case_id/close (ops)
	  v1.POST("/funds/add/fork-cases/:case_id/close", func(c *gin.Context) {
		   var body struct {
			   ClosedBy   string `json:"closed_by"`
			   Resolution string `json:"resolution"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if strings.TrimSpace(body.ClosedBy) == "" || strings.TrimSpace(body.Resolution) == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "closed_by and resolution required"})
			   return
		  }
		  fc, err := forks.Close(c.Request.Context(), c.Param("case_id"), body.ClosedBy, body.Resolution)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"fork_case": fc})
	   })
	   // GET /v1/funds/add/fork-cases/:case_id (ops)
	   v1.GET("/funds/add/fork-cases/:case_id", func(c *gin.Context) {
		  fc, err := forks.GetByID(c.Request.Context(), c.Param("case_id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"fork_case": fc})
	  })
	   // GET /v1/funds/add/confirm-leases (ops)
	   v1.GET("/funds/add/confirm-leases", func(c *gin.Context) {
		   // 仅列出活跃（issued）租约；参数 session_id 可选过滤
		   sessionID := strings.TrimSpace(c.Query("session_id"))
		  var list []ConfirmLease
		   var err error
		  if sessionID != "" {
			  list, err = leases.ListBySession(c.Request.Context(), sessionID)
		   } else {
			  list, err = leases.ListExpired(c.Request.Context(), time.Now().Add(365*24*time.Hour), 100)
		   }
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"confirm_leases": list, "count": len(list)})
	   })
	  // GET /v1/funds/add/:id/credit-effects (ops)
	   v1.GET("/funds/add/:id/credit-effects", func(c *gin.Context) {
		  list, err := effects.ListBySession(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"credit_effects": list, "count": len(list)})
	  })
  }
