// 账本快照差分增量校验——HTTP 路由处理器。
// CreateDate: 2022.06.14

// Author: Vance
// Date: 2021.08.11

package ledseal

import (
	"net/http"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// registerRoutes 注册 ledseal 的全部 HTTP 路由。
  func registerRoutes(v1 *gin.RouterGroup, svc *Service) {
	   // ──── 调度计划 API ────
	  // POST /ledseal/plans 创建计划
	  v1.POST("/ledseal/plans", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var req CreatePlanReq
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		  plan, err := svc.CreatePlan(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusCreated, plan)
	   })
	   // GET /ledseal/plans 列出所有计划
	  v1.GET("/ledseal/plans", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  limit := intParam(c.Query("limit"), 50)
		  plans, err := svc.ListPlans(c.Request.Context(), limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"plans": plans, "count": len(plans)})
	  })
	   // GET /ledseal/plans/:id 取单个计划
	  v1.GET("/ledseal/plans/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   plan, err := svc.GetPlan(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, plan)
	   })
	   // POST /ledseal/plans/:id/activate 激活计划
	   v1.POST("/ledseal/plans/:id/activate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  plan, err := svc.ActivatePlan(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, plan)
	   })
	   // POST /ledseal/plans/:id/pause 暂停计划
	   v1.POST("/ledseal/plans/:id/pause", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   plan, err := svc.PausePlan(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, plan)
	   })
	  // ──── 切片 API ────
	  // GET /ledseal/slices 列出切片（按 plan_id 过滤）
	  v1.GET("/ledseal/slices", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   planID := strings.TrimSpace(c.Query("plan_id"))
		  limit := intParam(c.Query("limit"), 50)
		  slices, err := svc.ListSlices(c.Request.Context(), planID, limit)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"slices": slices, "count": len(slices)})
	   })
	  // GET /ledseal/slices/latest 取计划最新切片
	   v1.GET("/ledseal/slices/latest", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   planID := strings.TrimSpace(c.Query("plan_id"))
		   if planID == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "plan_id required"})
			  return
		   }
		   sl, err := svc.LatestSlice(c.Request.Context(), planID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, sl)
	  })
	  // ──── 差分批次 API ────
	  // GET /ledseal/batches 列出批次（按 plan_id / state 过滤）
	   v1.GET("/ledseal/batches", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  planID := strings.TrimSpace(c.Query("plan_id"))
		   stateRaw := strings.TrimSpace(c.Query("state"))
		  limit := intParam(c.Query("limit"), 50)
		  var state BatchState
		  if stateRaw != "" {
			   state = BatchState(stateRaw)
		   }
		  batches,
			  err := svc.ListBatches(c.Request.Context(), planID, state, limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"batches": batches, "count": len(batches)})
	  })
	   // GET /ledseal/batches/:id 取单个批次
	  v1.GET("/ledseal/batches/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   b, err := svc.GetBatch(c.Request.Context(), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, b)
	  })
	  // ──── 漂移行 API ────
	   // GET /ledseal/drifts 列出漂移行（按 batch_id 过滤）
	  v1.GET("/ledseal/drifts", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  batchID := strings.TrimSpace(c.Query("batch_id"))
		   limit := intParam(c.Query("limit"), 200)
		   rows, err := svc.ListDriftRows(c.Request.Context(), batchID, limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"drifts": rows, "count": len(rows)})
	   })
	  // POST /ledseal/drifts/confirm 运营台确认漂移行
	   v1.POST("/ledseal/drifts/confirm", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var req ConfirmDriftReq
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		   if err := svc.ConfirmDrift(c.Request.Context(), req); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"ok": true, "drift_id": req.DriftID})
	   })
	   // ──── 封印根 API ────
	  // GET /ledseal/roots/:id 取封印根
	   v1.GET("/ledseal/roots/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   root, err := svc.GetSealRoot(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, root)
	   })
	  // GET /ledseal/roots/:id/verify 校验封印根完整性
	   v1.GET("/ledseal/roots/:id/verify", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  if err := svc.VerifyRoot(c.Request.Context(), c.Param("id")); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"ok": true, "root_id": c.Param("id")})
	   })
	  // POST /ledseal/roots/export 导出封印根（对账台调用）
	   v1.POST("/ledseal/roots/export", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req ExportRootReq
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		  root, err := svc.ExportRoot(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, root)
	   })
  }

   func intParam(s string, def int) int {
	   n, err := strconv.Atoi(s)
	  if err != nil || n <= 0 {
		   return def
	  }
	  return n
  }
