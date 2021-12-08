package sweeprun

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/airgap"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/sweeppol"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

var (
	platCer *custody.CeremonyBook
	platGap *airgap.Service
	platPipe *signpipe.Pipe
)

// PlatformBooks 本进程里冷引用的三本只读册。归集不另开仪式。
   func PlatformBooks() (*custody.CeremonyBook, *airgap.Service, *signpipe.Pipe) {
	  return platCer, platGap, platPipe
  }

  func mountProof(v1 *gin.RouterGroup, rt *app.Runtime) {
	  o := NewOrch()
	  if platCer == nil {
		  platCer = custody.NewCeremonyBook()
	  }
	   if platGap == nil {
		  platGap = airgap.New()
	   }
	  if platPipe == nil {
		  platPipe = signpipe.New(time.Hour)
	  }
	  BindPlatformRefs(o, platCer, platGap, platPipe)
	   if rt != nil && rt.SQL != nil {
		   o.BindSQL(rt.SQL)
		  _ = o.Load(context.Background())
	   }
	   BindGlobal(o)
	   Routes(v1, o)
   }

  func Routes(v1 *gin.RouterGroup, o *Orch) {
	  if o == nil {
		   o = NewOrch()
	  }
	   v1.GET("/ops/sweep-orch/health", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{
			  "ok":       o.LoadErr() == "" && !o.Halted(),
			  "halted":   o.Halted(),
			  "load_err": o.LoadErr(),
			  "tables":   ProofTables(),
			   "host": "custody-boundary-lab.kucoin.net",
		   })
	   })
	  v1.GET("/ops/sweep-orch/inventory", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{
			   "plan_allow": pairPlan(PlanInventory()),
			   "plan_deny": pairPlan(DenyPlanInventory()),
			  "leg_allow": pairLeg(LegInventory()),
			  "leg_deny":  pairLeg(DenyLegInventory()),
			  "tables": ProofTables(),
		  })
	   })
	   v1.GET("/ops/sweep-orch/plans", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"plans": o.List(c.Query("asset"), 50)})
	   })
	  v1.GET("/ops/sweep-orch/plans/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  got, err := o.Get(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.GET("/ops/sweep-orch/proof/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   b, err := o.Export(c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, b)
	  })
	  v1.POST("/ops/sweep-orch/desk", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Actor string `json:"actor"`
			  Asset string `json:"asset"`
			   Hot   string `json:"hot"`
			  Warm  string `json:"warm"`
			   Cold string `json:"cold"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if _, err := actor(body.Actor); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  if err := o.Seed(body.Asset, body.Hot, body.Warm, body.Cold); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   bal, err := o.Balance(body.Asset)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, bal)
	  })
	  v1.POST("/ops/sweep-orch/rule", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Actor string                 `json:"actor"`
			   Rule  sweeppol.RebalanceRule `json:"rule"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if _, err := actor(body.Actor); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  if err := o.PutRule(body.Rule); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true, "rules": o.Policy().List()})
	   })
	   v1.POST("/ops/sweep-orch/cap", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Actor string `json:"actor"`
			   Asset string `json:"asset"`
			  Cap string `json:"cap"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   if _, err := actor(body.Actor); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  if err := o.SetLaneCap(body.Asset, body.Cap, time.Time{}); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, o.LaneCap(body.Asset, time.Time{}))
	   })
	  v1.POST("/ops/sweep-orch/draft", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body DraftReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := o.Draft(c.Request.Context(), body)
		  if err != nil {
			   c.JSON(statusOr(err), gin.H{"error": err.Error(), "plan": got})
			  return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/ops/sweep-orch/attach-ref", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Actor string `json:"actor"`
			   PlanID string `json:"plan_id"`
			   LegID      string `json:"leg_id"`
			   CeremonyID string `json:"ceremony_id"`
			   AirgapID   string `json:"airgap_id"`
			   SignID string `json:"sign_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := o.AttachRefs(c.Request.Context(), body.PlanID, body.LegID, body.Actor, RefBind{
			  CeremonyID: body.CeremonyID,
			   AirgapID:   body.AirgapID, SignID: body.SignID,
		  })
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	   v1.POST("/ops/sweep-orch/tick", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   Actor string `json:"actor"`
			  Limit int `json:"limit"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if _, err := actor(body.Actor); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   got,
			  err := o.Tick(c.Request.Context(), body.Limit)
		   if err != nil {
			   c.JSON(statusOr(err), gin.H{"error": err.Error(), "report": got})
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/ops/sweep-orch/halt", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   Actor string `json:"actor"`
			  PlanID string `json:"plan_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := o.Halt(c.Request.Context(), body.PlanID, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"halted": o.Halted(), "plan": got})
	  })
	   v1.POST("/ops/sweep-orch/resume", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Actor string `json:"actor"`
			  PlanID string `json:"plan_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := o.Resume(c.Request.Context(), body.PlanID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"halted": o.Halted(), "plan": got})
	   })
	  v1.POST("/ops/sweep-orch/verify", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  ID string `json:"id"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := o.Verify(body.ID); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/ops/sweep-orch/fault", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Actor  string `json:"actor"`
			   Reason string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if _, err := actor(body.Actor); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  o.ArmFault(body.Reason)
		   c.JSON(200, gin.H{"armed": body.Reason != ""})
	   })
	  v1.GET("/ops/sweep-orch/desk", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   if o.Desk() == nil {
			  c.JSON(200, gin.H{"rows": []any{}})
			   return
		  }
		   c.JSON(200, gin.H{"rows": o.Desk().List()})
	  })
  }

   func actor(id string) (string, error) {
	  got, err := validate.Actor(id)
	  if err != nil {
		   return "", ErrActor
	  }
	  return got, nil
   }

  func statusOr(err error) int {
	   return apierr.HTTPStatus(err)
  }

type edgeJSON struct {
	From string `json:"from"`
	To string `json:"to"`
}

  func pairPlan(in [][2]PlanState) []edgeJSON {
	   out := make([]edgeJSON, len(in))
	   for i := range in {
		   out[i] = edgeJSON{From: string(in[i][0]), To: string(in[i][1])}
	  }
	   return out
   }

  func pairLeg(in [][2]LegState) []edgeJSON {
	   out := make([]edgeJSON, len(in))
	  for i := range in {
		  out[i] = edgeJSON{From: string(in[i][0]), To: string(in[i][1])}
	   }
	   return out
   }
