package opsapi

import (
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
	"github.com/gin-gonic/gin"
)

// registerIncident 挂对账事故升级。Engage 熔断可选；禁止 Clear。
  func registerIncident(v1 *gin.RouterGroup, rt *app.Runtime, el *ha.Elector) {
	   if v1 == nil || rt == nil {
		   return
	  }
	  v1.POST("/ops/admin/recon/incidents", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   if el != nil {
			   if err := el.RequireLeader(); err != nil {
				   c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "ha": el.Status()})
				   return
			  }
		   }
		  var body struct {
			   EngageMode string `json:"engage_mode"`
			   MinLevel   int    `json:"min_level"`
			  Chain      string `json:"chain"`
		   }
		   _ = c.ShouldBindJSON(&body)
		  wf := reconWorkflow(rt)
		  hooks := recon.IncidentHooks{}
		   if rt.FundSurf != nil {
			   hooks.Halt = custodyhalt.EngageOnly{
				  Gate: rt.FundSurf.HaltGate(),
				   Hist: custodyhalt.SharedHistory(),
			  }
		   }
		  var store *recon.IncidentStore
		   opts := recon.IncidentPersistOpts{
			   Policy: recon.IncidentPolicy{
				   Mode: recon.EngageMode(strings.TrimSpace(body.EngageMode)),
				  MinLevel: body.MinLevel,
				   Chain:    body.Chain,
			  },
		  }
		  if rt.KV != nil {
			  store = recon.NewIncidentStore(rt.KV)
			  opts.EscStore = recon.NewEscalationStore(rt.KV)
			   opts.Drift = recon.NewDriftFreezeStore(rt.KV)
		   }
		   recs, err := wf.RaiseIncidentsPersistOpts(c.Request.Context(), time.Now().UTC(), actor.ID, hooks, store, opts)
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "incidents": recs})
			   return
		   }
		   mode := string(opts.Policy.Mode)
		   if mode == "" {
			   mode = string(recon.EngageAuto)
		  }
		   minLvl := opts.Policy.MinLevel
		  if minLvl <= 0 {
			  minLvl = 2
		   }
		   c.JSON(http.StatusOK, gin.H{
			  "incidents":   recs,
			   "count":       len(recs),
			   "engage_mode": mode,
			   "min_level": minLvl,
			   "chain": strings.ToUpper(strings.TrimSpace(body.Chain)),
			   "engage_hint": "auto prefers pair when chain+common asset; pair requires both; never clears halt",
			   "host": "ledger-transfer-freeze-reconciliation.kucoin.net",
		   })
	  })
  }

   func reconWorkflow(rt *app.Runtime) *recon.Workflow {
	  if rt == nil || rt.Durable.Recon == nil {
		   return recon.NewWorkflow(0)
	  }
	  return recon.WrapBook(rt.Durable.Recon, 0)
  }
