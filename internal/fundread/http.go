package fundread

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundmat"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Mount 注册强一致读、最终一致读、重建、滞后与泵控制。

  func Mount(v1 *gin.RouterGroup, svc *Service, opt callerid.Opt) {
	  if v1 == nil || svc == nil {
		  return
	  }
	  g := v1.Group("/funding/views")
	   g.GET("/strong", func(c *gin.Context) {
		   q, err := queryOf(c, opt)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   if err := perm.FromRequest(c.Request, q.UserID).Check(perm.ActQueryRead); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  view, err := svc.Strong(c.Request.Context(), q)
		  if errors.Is(err, ErrNotCaughtUp) || errors.Is(err, ErrFenceExpired) {
			  c.JSON(409, gin.H{
				  "error": "投影未追平", "detail": err.Error(), "stale": true,
				   "watermarks": view.Watermarks, "fence": view.Fence, "user_id": q.UserID,
			  })
			  return
		   }
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, view)
	  })
	  g.GET("/eventual", func(c *gin.Context) {
		   q, err := queryOf(c, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   if err := perm.FromRequest(c.Request, q.UserID).Check(perm.ActQueryRead); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   view, err := svc.Eventual(c.Request.Context(), q)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, view)
	  })
	   g.GET("/lag", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  rep, err := svc.Mat.Lag(c.Request.Context())
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, rep)
	   })
	   g.GET("/version", func(c *gin.Context) {
		   q, err := queryOf(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  if err := perm.FromRequest(c.Request, q.UserID).Check(perm.ActQueryRead); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   v, err := svc.Mat.Version(c.Request.Context(), q.UserID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, v)
	   })
	  g.POST("/fences", func(c *gin.Context) {
		   q, err := queryOf(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   if err := perm.FromRequest(c.Request, q.UserID).Check(perm.ActQueryRead); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   var body struct {
			  TTLSec int64 `json:"ttl_sec"`
		  }
		   _ = c.ShouldBindJSON(&body)
		  f,
			   err := svc.IssueFence(c.Request.Context(), q, time.Duration(body.TTLSec)*time.Second)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, f)
	   })
	   g.POST("/rebuild", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
			  Scope  string `json:"scope"`
			   Tolerance string `json:"tolerance"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength > 0 {
			  mw.WriteProblem(c, err)
			  return
		  }
		   if body.UserID == "" {
			  uid,
				  err := callerid.Require(c, opt)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			   }
			   body.UserID = uid
		  }
		  job,
			   err := svc.Mat.RebuildNow(c.Request.Context(), body.UserID, body.Scope, body.Tolerance)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   code := 200
		   if job.Status == fundmat.JobFailed {
			  code = 409
		   }
		  c.JSON(code, job)
	  })
	  g.GET("/rebuild/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  job, err := svc.Mat.Job(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, job)
	   })
	   g.POST("/events", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  var body struct {
			  ID     string `json:"id"`
			   Stream string `json:"stream"`
			  Seq    int64  `json:"seq"`
			  UserID string `json:"user_id"`
			   Asset   string `json:"asset"`
			  Subject string `json:"subject"`
			  Op      string `json:"op"`
			  Amount string `json:"amount"`
			   Bucket string `json:"bucket"`
			  CausalID   string `json:"causal_id"`
			  OccurredAt string `json:"occurred_at"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   ev := fundmat.FundingEvent{
			   ID: body.ID, Stream: fundmat.Stream(body.Stream), Seq: body.Seq,
			  UserID: body.UserID, Asset: body.Asset, Subject: body.Subject,
			  Op: fundmat.Op(body.Op), Amount: body.Amount, Bucket: fundmat.Bucket(body.Bucket),
			  CausalID: body.CausalID,
		  }
		  if body.OccurredAt != "" {
			  t, err := time.Parse(time.RFC3339, body.OccurredAt)
			   if err != nil {
				  mw.WriteProblem(c,
					   fundmat.ErrBadEvent)
				  return
			   }
			   ev.OccurredAt = t
		  }
		   stored, err := svc.Mat.Ingest(c.Request.Context(), ev)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, stored)
	   })
	   g.POST("/pump", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  stats, err := svc.Mat.Tick(c.Request.Context())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, stats)
	  })
	  g.POST("/pump/pause", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  var body struct {
			  ForMs int64 `json:"for_ms"`
		  }
		   _ = c.ShouldBindJSON(&body)
		  d := time.Duration(body.ForMs) * time.Millisecond
		   if d <= 0 {
			   d = time.Minute
		   }
		   svc.Mat.Pause(d)
		  c.JSON(200, gin.H{"paused": true})
	  })
	  g.POST("/pump/resume", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   svc.Mat.Resume()
		   c.JSON(200, gin.H{"paused": false})
	   })
   }

  func queryOf(c *gin.Context, opt callerid.Opt) (Query, error) {
	  uid,
		  err := callerid.Require(c, opt)
	   if err != nil {
		   return Query{}, err
	  }
	   if raw := strings.TrimSpace(c.Query("user_id")); raw != "" && perm.FromRequest(c.Request, uid).Check(perm.ActOpsAdmin) == nil {
		   uid = raw
	  }
	  seq, _ := strconv.ParseInt(c.Query("fence_seq"), 10, 64)
	   epoch, _ := strconv.ParseInt(c.Query("fence_epoch"), 10, 64)
	   return Query{
		   UserID: uid, Asset: c.Query("asset"), Subject: c.Query("subject"),
		   FenceID: c.Query("fence_id"), FenceSeq: seq, FenceEpoch: epoch,
		   Stream: fundmat.Stream(strings.TrimSpace(c.Query("stream"))),
	   }, nil
   }
