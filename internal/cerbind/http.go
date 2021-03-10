package cerbind

import (
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Mount 把路由挂到 /v1。调用方需已完成 Attach。
  func Mount(v1 *gin.RouterGroup, eng *Engine) {
	  if v1 == nil || eng == nil {
		  return
	  }
	  v1.POST("/cerbind/policies", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Actor  string `json:"actor"`
			  Policy Policy `json:"policy"`
			  ID     string `json:"id"`
			  Name string `json:"name"`
			  Asset string `json:"asset"`
			   Chain     string `json:"chain"`
			  Purpose   string `json:"purpose"`
			  MinAmount string `json:"min_amount"`
			  MaxAmount string `json:"max_amount"`
			  DestRisk string `json:"dest_risk"`
			  Side      string `json:"side"`
			   ForceCold bool   `json:"force_cold"`
			   Threshold int    `json:"threshold"`
			   Shares    int    `json:"shares"`
			   BindMode string `json:"bind_mode"`
			   Priority int   `json:"priority"`
			   Enabled  *bool `json:"enabled"`
			   ApprovalNeed bool `json:"approval_need"`
			  MaxAgeSec int `json:"max_age_sec"`
			   LimitRef string `json:"limit_ref"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   p := body.Policy
		   if body.ID != "" {
			   p.ID = body.ID
		   }
		   if body.Name != "" {
			  p.Name = body.Name
		   }
		  if body.Asset != "" {
			  p.Asset = body.Asset
		   }
		   if body.Chain != "" {
			  p.Chain = body.Chain
		  }
		  if body.Purpose != "" {
			   p.Purpose = body.Purpose
		  }
		  if body.MinAmount != "" {
			  p.MinAmount = body.MinAmount
		  }
		   if body.MaxAmount != "" {
			  p.MaxAmount = body.MaxAmount
		  }
		   if body.DestRisk != "" {
			   p.DestRisk = body.DestRisk
		   }
		   if body.Side != "" {
			   p.Side = body.Side
		  }
		  if body.ForceCold {
			  p.ForceCold = true
		  }
		   if body.Threshold > 0 {
			   p.Threshold = body.Threshold
		  }
		   if body.Shares > 0 {
			   p.Shares = body.Shares
		   }
		  if body.BindMode != "" {
			   p.BindMode = body.BindMode
		   }
		   if body.Priority > 0 {
			   p.Priority = body.Priority
		   }
		   if body.Enabled != nil {
			  p.Enabled = *body.Enabled
		  } else if p.ID != "" && body.Policy.ID == "" {
			   p.Enabled = true
		  }
		   p.ApprovalNeed = p.ApprovalNeed || body.ApprovalNeed
		  if body.MaxAgeSec > 0 {
			  p.MaxAgeSec = body.MaxAgeSec
		  }
		   if body.LimitRef != "" {
			   p.LimitRef = body.LimitRef
		  }
		  got, err := eng.PutPolicy(c.Request.Context(), p, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.GET("/cerbind/policies", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  rows,
			   err := eng.Policies(c.Request.Context())
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"policies": rows})
	  })
	  v1.POST("/cerbind/policies/eval", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body PlanReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := eng.Eval(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/cerbind/snapshots", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body Snapshot
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got,
			   err := eng.Ingest(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/cerbind/sessions/plan", func(c *gin.Context) {
		   if !allow(c, perm.ActCeremony) {
			  return
		  }
		   var body PlanReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := eng.Plan(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/cerbind/sessions/open", func(c *gin.Context) {
		  if !allow(c, perm.ActCeremony) {
			  return
		  }
		   var body fenceBody
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  got,
			  err := eng.Open(c.Request.Context(), body.ID, body.Actor, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/cerbind/sessions/receipt", func(c *gin.Context) {
		   if !allow(c, perm.ActCeremony) {
			   return
		  }
		  var body struct {
			   fenceBody
			  Slot string `json:"slot"`
			  Note string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := eng.Receipt(c.Request.Context(), body.ID, body.Actor, body.Slot, body.Note, body.Epoch, body.Token)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/cerbind/sessions/bind", func(c *gin.Context) {
		  if !allow(c, perm.ActCeremony) {
			   return
		   }
		   var body struct {
			   fenceBody
			   BatchID string `json:"batch_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := eng.Bind(c.Request.Context(), body.ID, body.BatchID, body.Actor, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	  v1.POST("/cerbind/sessions/sign-ready", func(c *gin.Context) {
		   if !allow(c, perm.ActCeremony) {
			  return
		   }
		   var body fenceBody
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := eng.MarkReady(c.Request.Context(), body.ID, body.Actor, body.Epoch, body.Token)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/cerbind/sessions/close", func(c *gin.Context) {
		   if !allow(c, perm.ActCeremony) {
			   return
		   }
		  var body fenceBody
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := eng.Close(c.Request.Context(), body.ID, body.Actor, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/cerbind/sessions/abort", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		   var body struct {
			   ID    string `json:"id"`
			   Actor string `json:"actor"`
			   Reason string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := eng.Abort(c.Request.Context(), body.ID, body.Actor, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	   v1.GET("/cerbind/sessions", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  rows, err := eng.Sessions(c.Request.Context(), 50)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"sessions": rows})
	  })
	   v1.GET("/cerbind/sessions/:id", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		  }
		  got, err := eng.View(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	   v1.GET("/cerbind/bindings", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  got, err := eng.BindingOf(c.Request.Context(), c.Query("batch_id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/cerbind/gate/check", func(c *gin.Context) {
		   if !allow(c,
			  perm.ActCustodySign) {
			   return
		  }
		  var body Subject
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := eng.CheckBinding(c.Request.Context(), body); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/cerbind/worker/tick", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		  got, err := eng.Tick(c.Request.Context())
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	   v1.GET("/cerbind/alerts", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		   }
		   rows, err := eng.Alerts(c.Request.Context(), 50)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"alerts": rows})
	   })
   }

type fenceBody struct {
	ID string `json:"id"`
	Actor string `json:"actor"`
	Epoch int64  `json:"epoch"`
	Token string `json:"token"`
}

  func allow(c *gin.Context, act perm.Action) bool {
	  if err := perm.FromRequest(c.Request, "").Check(act); err != nil {
		   c.JSON(403, gin.H{"error": err.Error()})
		   return false
	  }
	  return true
   }
