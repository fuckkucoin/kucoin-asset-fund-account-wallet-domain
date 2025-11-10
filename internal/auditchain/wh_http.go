package auditchain

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/auditlog"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// RegisterWarehouse 挂查询、重放、导出。rt 只用来在有 SQL 时换仓储，调用方也可事先 Bind。
   func RegisterWarehouse(v1 *gin.RouterGroup, rt *app.Runtime, wh *Warehouse, desk *Desk) {
	   if wh == nil {
		   st := Store(NewMem())
		   if rt != nil && rt.SQL != nil && rt.SQL.DB != nil {
			   st = NewSQL(rt.SQL)
		   }
		   wh = NewWarehouse(st, DefaultConfig())
	   }
	  if rt != nil && rt.Ledger != nil {
		  wh.SetReader(LedgerBook{Ledger: rt.Ledger})
	  }
	   if desk == nil {
		  desk = NewDesk(wh, nil)
	   }
	   g := v1.Group("/audit-wh")
	   g.GET("/kinds", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  c.JSON(200, gin.H{"kinds": Kinds()})
	   })
	   g.GET("/segments", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		  }
		   segs, err := wh.Store().AllSegments(c.Request.Context())
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"segments": segs})
	   })
	   g.GET("/timeline", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		   }
		   from, to, err := parseWindow(c.Query("from"), c.Query("to"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  rep, err := wh.Timeline(c.Request.Context(), c.Query("user_id"), c.Query("asset"), from, to)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   lines := make([]auditlog.WhLine, 0, len(rep.Steps))
		   for _, s := range rep.Steps {
			   lines = append(lines, auditlog.WhLine{
				   Seq: int64(s.Ord), Kind: s.Kind, User: rep.UserID, Asset: rep.Asset,
				  Amount: s.Amount, Ref: s.RefID, Body: s.Note, At: s.At,
			   })
		  }
		   page := auditlog.PageLines(lines,
			   0, 200)
		  c.JSON(200, gin.H{"report": rep, "page": page, "csv": auditlog.LinesCSV(page.Lines)})
	  })
	  g.POST("/replay", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		   var body struct {
			   UserID string `json:"user_id"`
			  Asset string `json:"asset"`
			   From string `json:"from"`
			  To   string `json:"to"`
			   Expect []string `json:"expect"`
			   DualID string `json:"dual_id"`
			  RunNow bool   `json:"run_now"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  from, to, err := parseWindow(body.From, body.To)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		   j, err := wh.SubmitReplay(c.Request.Context(), body.UserID, body.Asset, actor, from, to, body.Expect, body.DualID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  if body.RunNow {
			   j, err = wh.RunReplayID(c.Request.Context(), j.ID)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			   }
		   }
		  c.JSON(200, j)
	   })
	   g.GET("/replay/:id", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			  return
		  }
		   j, err := wh.GetReplay(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, j)
	  })
	  g.POST("/exports", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		   var body struct {
			  SegmentID string `json:"segment_id"`
			   UserID    string `json:"user_id"`
			   Asset string `json:"asset"`
			   From  string `json:"from"`
			   To    string `json:"to"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  from, to, err := parseWindow(body.From, body.To)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  view, err := desk.Request(c.Request.Context(), actor, body.SegmentID, body.UserID, body.Asset, from, to)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, view)
	   })
	   g.POST("/exports/:id/approve", func(c *gin.Context) {
		   if !allowAny(c, perm.ActOpsAdmin, perm.ActCustodyApprove) {
			  return
		   }
		  var body struct {
			   Allow bool   `json:"allow"`
			   Epoch int64  `json:"epoch"`
			   Token string `json:"token"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		  view, err := desk.Approve(c.Request.Context(), c.Param("id"), actor, body.Allow, body.Epoch, body.Token)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, view)
	   })
	  g.POST("/exports/:id/release", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  bundle, err := desk.Release(c.Request.Context(), c.Param("id"), actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, bundle)
	   })
	  g.GET("/alerts", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		  rows, err := wh.Store().Alerts(c.Request.Context(), true)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"alerts": rows, "export_halted": ExportHalted(), "ingest_failures": ingest.Failures()})
	  })
	  g.GET("/integrity", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		   rep, err := wh.Inspect(c.Request.Context())
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, rep)
	   })
  }

   func allow(c *gin.Context, act perm.Action) bool {
	   return allowAny(c, act)
   }

   func allowAny(c *gin.Context, acts ...perm.Action) bool {
	  actor := perm.FromRequest(c.Request, "")
	  for _, act := range acts {
		  if actor.Check(act) == nil {
			   return true
		  }
	  }
	  c.JSON(403, gin.H{"error": perm.ErrDenied.Error()})
	   return false
   }

   func parseWindow(from, to string) (time.Time, time.Time, error) {
	  var a, b time.Time
	  var err error
	  if strings.TrimSpace(from) != "" {
		   a, err = time.Parse(time.RFC3339, strings.TrimSpace(from))
		  if err != nil {
			  return time.Time{},
				   time.Time{}, ErrReplay
		   }
	   }
	   if strings.TrimSpace(to) != "" {
		  b,
			   err = time.Parse(time.RFC3339, strings.TrimSpace(to))
		  if err != nil {
			  return time.Time{}, time.Time{}, ErrReplay
		  }
	  }
	  return a, b, nil
   }
