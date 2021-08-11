package hotreserve

import (
	"time"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Mount(v1 *gin.RouterGroup, svc *Service) {
	   if v1 == nil || svc == nil {
		   return
	  }
	   v1.GET("/hotreserve/forecast", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead, perm.ActCustodySweep) {
			   return
		   }
		   asset := c.Query("asset")
		   if asset == "" {
			   c.JSON(400, gin.H{"error": ErrAsset.Error()})
			   return
		   }
		  got,
			   err := svc.Forecast(asset)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   v1.GET("/hotreserve/forecast/history", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		   got, err := svc.History(c.Query("asset"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"history": got})
	  })
	   v1.POST("/hotreserve/forecast/refresh", func(c *gin.Context) {
		  if !allow(c, perm.ActCustodySweep) {
			  return
		   }
		   var body struct {
			  Asset string `json:"asset"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.Asset == "" {
			  got, err := svc.RefreshAll(c.Request.Context())
			  if err != nil {
				   mw.WriteProblem(c, err)
				   return
			  }
			   c.JSON(200, gin.H{"forecasts": got})
			  return
		  }
		  got, err := svc.Refresh(c.Request.Context(), body.Asset)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/hotreserve/samples", func(c *gin.Context) {
		   if !allow(c, perm.ActCustodySweep) {
			  return
		  }
		   var body struct {
			   Asset string `json:"asset"`
			   Inflow string `json:"inflow"`
			  Outflow string    `json:"outflow"`
			  At      time.Time `json:"at"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := svc.RecordSample(c.Request.Context(), body.Asset, body.Inflow, body.Outflow, body.At)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	  v1.GET("/hotreserve/operational", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead, perm.ActCustodySign) {
			  return
		   }
		  got, err := svc.Explain(c.Query("asset"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	   v1.GET("/hotreserve/reservations", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead, perm.ActCustodySign) {
			  return
		   }
		   c.JSON(200, gin.H{"reservations": svc.List(c.Query("asset"), c.Query("state"))})
	   })
	  v1.POST("/hotreserve/reservations", func(c *gin.Context) {
		  if !allow(c, perm.ActCustodySign, perm.ActWithdrawSubmit) {
			  return
		   }
		   var body CreateReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.Create(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/hotreserve/reservations/:id/earmark", func(c *gin.Context) {
		   if !allow(c, perm.ActCustodySign) {
			   return
		   }
		  var body struct {
			   MovementID string `json:"movement_id"`
			   FreezeRef  string `json:"freeze_ref"`
			  Note       string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.Earmark(c.Request.Context(), EarmarkReq{ID: c.Param("id"), MovementID: body.MovementID, FreezeRef: body.FreezeRef, Note: body.Note})
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.POST("/hotreserve/reservations/:id/consume", func(c *gin.Context) {
		   if !allow(c, perm.ActCustodySign) {
			   return
		   }
		  var body struct {
			  BroadcastRef string `json:"broadcast_ref"`
			   Note string `json:"note"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.Consume(c.Request.Context(), ConsumeReq{ID: c.Param("id"), BroadcastRef: body.BroadcastRef, Note: body.Note})
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.POST("/hotreserve/reservations/:id/cancel", func(c *gin.Context) {
		   if !allow(c, perm.ActCustodySign) {
			   return
		   }
		  var body struct {
			  Note string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.Cancel(c.Request.Context(), c.Param("id"), body.Note)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/hotreserve/reservations/:id/force-release", func(c *gin.Context) {
		   if !allow(c, perm.ActCustodyApprove) {
			   return
		   }
		  var body struct {
			  Approver string `json:"approver"`
			   Note     string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  actor := perm.FromRequest(c.Request, "")
		   got, err := svc.ForceRelease(c.Request.Context(), ReleaseReq{
			  ID: c.Param("id"), Requester: actor.ID, Approver: body.Approver, Note: body.Note,
		  })
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.GET("/hotreserve/journal", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		   c.JSON(200, gin.H{"journal": svc.Journals(c.Query("reservation_id"), c.Query("asset"))})
	  })
	  v1.GET("/hotreserve/align", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead, perm.ActCustodySweep) {
			   return
		  }
		  c.JSON(200, gin.H{"reports": svc.Reports(c.Query("asset")), "open": svc.OpenDrifts()})
	  })
	   v1.POST("/hotreserve/align/run", func(c *gin.Context) {
		  if !allow(c, perm.ActCustodySweep) {
			  return
		   }
		   var body struct {
			  Asset string `json:"asset"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if body.Asset == "" {
			   got, err := svc.PatrolAll(c.Request.Context())
			   if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(200, gin.H{"reports": got})
			  return
		   }
		   got, err := svc.Patrol(c.Request.Context(), body.Asset)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	   v1.POST("/hotreserve/expire/run", func(c *gin.Context) {
		  if !allow(c, perm.ActCustodySweep) {
			   return
		   }
		   n, err := svc.ExpireDue(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"expired": n})
	  })
	  v1.POST("/hotreserve/observe", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		   var body struct {
			  Asset string `json:"asset"`
			  Kind  string `json:"kind"`
			   Amount string `json:"amount"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  var err error
		  switch body.Kind {
		   case TrackFreeze:
			   err = svc.SetObservedFreeze(c.Request.Context(), body.Asset, body.Amount)
		   case TrackBroadcast:
			  err = svc.SetObservedBroadcast(c.Request.Context(), body.Asset, body.Amount)
		   default:
			   err = ErrState
		  }
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"ok": true})
	  })
	   v1.POST("/hotreserve/hints/suggest", func(c *gin.Context) {
		   if !allow(c, perm.ActCustodySweep) {
			  return
		   }
		   var body struct {
			  Asset string `json:"asset"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.Suggest(c.Request.Context(), body.Asset)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	   v1.GET("/hotreserve/hints", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  c.JSON(200, gin.H{"hints": svc.Hints(c.Query("asset"))})
	  })
	  v1.POST("/hotreserve/plans", func(c *gin.Context) {
		   if !allow(c, perm.ActCustodySweep) {
			   return
		   }
		  var body struct {
			  Asset string     `json:"asset"`
			  Plans []PlanHold `json:"plans"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if err := svc.NoteSweepPlans(c.Request.Context(), body.Asset, body.Plans); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"plans": svc.Plans(body.Asset)})
	  })
	   v1.GET("/hotreserve/gates", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			  return
		  }
		   c.JSON(200, gin.H{"gates": svc.GateLogs(c.Query("asset"))})
	  })
   }

  func allow(c *gin.Context, acts ...perm.Action) bool {
	   actor := perm.FromRequest(c.Request, "")
	   for _, act := range acts {
		  if actor.Check(act) == nil {
			  return true
		  }
	  }
	   c.JSON(403, gin.H{"error": perm.ErrDenied.Error()})
	   return false
  }
