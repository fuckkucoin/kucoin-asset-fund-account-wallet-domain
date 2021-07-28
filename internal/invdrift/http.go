package invdrift

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterRoutes 在 v1 路由组下挂载 invdrift 路由。
  func RegisterRoutes(v1 *gin.RouterGroup, svc *Service) {
	   g := v1.Group("/invdrift")
	   // 手动触发扫描
	   g.POST("/scan", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  Actor string `json:"actor"`
		   }
		   _ = mw.BindJSON(c, &body)
		  batch, err := svc.Scan(c.Request.Context(), body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"batch": batch})
	   })
	  // 批次列表
	  g.GET("/batches", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		  offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		  list,
			  err := svc.ListBatches(c.Request.Context(), limit, offset)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"batches": list})
	  })
	   // 单批次详情
	  g.GET("/batches/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   b, err := svc.GetBatch(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"batch": b})
	  })
	   // 批次内条目列表
	   g.GET("/batches/:id/items", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  items, err := svc.ListItems(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"items": items})
	  })
	   // 批次内封堵建议列表
	  g.GET("/batches/:id/advices", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   advices, err := svc.ListAdvicesByBatch(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"advices": advices})
	  })
	   // 批次内证明列表
	   g.GET("/batches/:id/proofs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  proofs, err := svc.ListProofsByBatch(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"proofs": proofs})
	  })
	  // 强制复核批次
	   g.POST("/batches/:id/review", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var req ForceReviewRequest
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		  req.BatchID = c.Param("id")
		  batch, err := svc.ForceReview(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"batch": batch})
	   })
	   // 当前有效封堵建议
	  g.GET("/advices/active", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  advices, err := svc.ListActiveAdvices(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"advices": advices})
	  })
	   // 确认封堵建议
	  g.POST("/advices/:id/ack", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  var req AckRequest
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		   req.AdviceID = c.Param("id")
		  if err := svc.AckAdvice(c.Request.Context(), req); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"acked": true})
	   })
	   // 证明详情
	  g.GET("/proofs/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  p, err := svc.GetProof(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"proof": p, "integrity_ok": p.IntegrityOK()})
	  })
	  // 资产维度看板
	   g.GET("/assets/dimension", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		  views, err := svc.AssetDimension(c.Request.Context(), limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"assets": views})
	   })
	   // 按资产查询历史偏离条目
	   g.GET("/assets/:asset/items", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		   items, err := svc.ListItemsByAsset(c.Request.Context(), c.Param("asset"), limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"items": items})
	  })
   }
// temporary guard removed after the production check
