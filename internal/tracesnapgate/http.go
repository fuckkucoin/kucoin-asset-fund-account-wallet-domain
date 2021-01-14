package tracesnapgate

import (
	"errors"
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 将 tracesnapgate 路由注册到 v1 路由组。

   func Register(v1 *gin.RouterGroup, engine *Engine) {
	  g := v1.Group("/tracesnap")
	  // POST /tracesnap/snapshots — 创建 open 快照。
	   g.POST("/snapshots", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			  Actor  string `json:"actor"`
			   IdemKey string `json:"idem_key"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  req := CreateRequest{
			   UserID:  body.UserID,
			  Asset:   body.Asset,
			  Actor:   body.Actor,
			   IdemKey: body.IdemKey,
		   }
		   s, err := engine.CreateSnapshot(c.Request.Context(), req)
		  if err != nil {
			   writeSnapError(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, s)
	   })
	  // POST /tracesnap/snapshots/:id/seal — 密封快照。
	  g.POST("/snapshots/:id/seal", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   s, err := engine.Seal(c.Request.Context(), SealRequest{
			  SnapshotID: c.Param("id"),
			   Actor:      body.Actor,
		   })
		  if err != nil {
			   writeSnapError(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, s)
	   })
	  // POST /tracesnap/snapshots/:id/replay — 重放复验。
	   g.POST("/snapshots/:id/replay", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  res, err := engine.Replay(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   if de, isDrift := IsDriftError(err); isDrift {
				  c.JSON(http.StatusConflict, gin.H{
					   "error":   "drift_detected",
					   "message": ErrDriftDetected.Error(),
					  "drift_proof_id": de.ProofID,
					   "proof_hash":     de.ProofHash,
				  })
				   return
			  }
			  writeSnapError(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, res)
	   })
	   // POST /tracesnap/snapshots/:id/export — 法医导出。
	   g.POST("/snapshots/:id/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   bundle, err := engine.Export(c.Request.Context(), ExportRequest{
			   SnapshotID: c.Param("id"),
			  Actor: body.Actor,
		  })
		  if err != nil {
			  writeSnapError(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, bundle)
	   })
	  // POST /tracesnap/snapshots/:id/bind-case — 绑定调查案。
	  g.POST("/snapshots/:id/bind-case", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   CaseID  string `json:"case_id"`
			   BoundBy string `json:"bound_by"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  b, err := engine.BindCase(c.Request.Context(), BindCaseRequest{
			   CaseID:     body.CaseID,
			  SnapshotID: c.Param("id"),
			  BoundBy:    body.BoundBy,
		   })
		   if err != nil {
			   writeSnapError(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, b)
	  })
	   // GET /tracesnap/snapshots/:id — 读快照详情。
	  g.GET("/snapshots/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  s, err := engine.GetSnapshot(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  writeSnapError(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, s)
	   })
	  // GET /tracesnap/snapshots/:id/proofs — 读漂移证明列表。
	  g.GET("/snapshots/:id/proofs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  proofs, err := engine.ListProofsBySnapshot(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   writeSnapError(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
	   })
	  // GET /tracesnap/proofs/:id — 读单条漂移证明。
	  g.GET("/proofs/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  p, err := engine.GetProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   writeSnapError(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, p)
	  })
	   // GET /tracesnap/snapshots?user_id=&limit= — 用户快照列表。
	  g.GET("/snapshots", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  uid := c.Query("user_id")
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		  if limit <= 0 || limit > 200 {
			   limit = 20
		   }
		  snaps,
			  err := engine.ListSnapshotsByUser(c.Request.Context(), uid, limit)
		   if err != nil {
			  writeSnapError(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"snapshots": snaps, "count": len(snaps)})
	  })
   }

// writeSnapError 将 tracesnapgate 错误映射到 HTTP 状态码。

   func writeSnapError(c *gin.Context, err error) {
	  switch {
	  case errors.Is(err, ErrNotFound):
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrSealedAlready):
		   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	  case errors.Is(err, ErrCaseBound):
		  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrSuperseded):
		  c.JSON(http.StatusGone, gin.H{"error": err.Error()})
	  case errors.Is(err, ErrNoSnapshot):
		   c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrNotSealed):
		   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrNotOpen):
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrLiveMode):
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	   case errors.Is(err, ErrInvalidUserID), errors.Is(err, ErrInvalidAsset):
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	  default:
		   mw.WriteProblem(c, err)
	  }
   }
