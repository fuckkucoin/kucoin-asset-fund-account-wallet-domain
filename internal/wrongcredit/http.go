// 路由前缀：/v1/wrongcredit/*
// 端点列表：
// POST /wrongcredit/open 开立案件
// POST /wrongcredit/evidence 附加证据
// POST /wrongcredit/score 触发分值
// POST /wrongcredit/approve-freeze 批准冻结（运营）
// POST /wrongcredit/approve-freeze-2 第二审批人（Tier3）
// POST /wrongcredit/execute-freeze 执行冻结分录
// POST /wrongcredit/approve-revert 批准冲销
// POST /wrongcredit/execute-revert 执行冲销分录
// POST /wrongcredit/waive 豁免案件
// GET /wrongcredit/case 获取单个案件
// GET /wrongcredit/cases 列举案件（带过滤）
// GET /wrongcredit/evidence 获取案件证据
// GET /wrongcredit/timeline 获取案件时间轴
// GET /wrongcredit/mutex 查看互斥锁快照（管理员）
// 权限：
// 写操作（open/evidence/approve*/execute*/waive）需 ActAMLOpen 或 ActOpsAdmin。
// 执行操作（execute-freeze/execute-revert）额外需要 ActLedgerWrite（或 ActOpsAdmin）。
// 读操作需 ActCaseRead。
// mutex 端点需 ActOpsAdmin。

// Author: Kinsley
// Date: 2023.12.27

package wrongcredit

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Register 注册 wrongcredit HTTP 路由到 v1 路由组。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   disc := NewDiscovery()
	  mutex := NewCreditMutex()
	  var kv persist.KV = persist.NewMemKV()
	  if rt != nil && rt.KV != nil {
		   kv = rt.KV
	  }
	  store := NewStore(kv)
	  if rt != nil && rt.SQL != nil && rt.SQL.DB != nil {
		   store.BindSQL(rt.SQL.DB)
	  }
	  svc := NewService(disc, mutex, store)
	  if rt != nil && rt.Store != nil {
		   svc.BindLedger(NewStoreAdapter(rt.Store))
	  }
	  _ = svc.Hydrate(context.Background())
	   g := v1.Group("/wrongcredit")
	  // ─── 写操作 ────────────────────────────────────────────────────────────
	  g.POST("/open", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body OpenReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.OpenedBy == "" {
			   body.OpenedBy = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got, err := svc.Open(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	   g.POST("/evidence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body AttachEvidenceReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got,
			   err := svc.AttachEvidence(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   g.POST("/score", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  CaseID string `json:"case_id"`
			   Actor   string `json:"actor"`
			   USDRate string `json:"usd_rate,omitempty"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got, err := svc.Score(c.Request.Context(), body.CaseID, body.Actor, body.USDRate)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	  g.POST("/approve-freeze", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body ApproveReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got, err := svc.ApproveFreeze(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	  g.POST("/approve-freeze-2", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body ApproveReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got,
			   err := svc.ApproveFreeze2(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  g.POST("/execute-freeze", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   CaseID string `json:"case_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.ExecuteFreeze(c.Request.Context(), strings.TrimSpace(body.CaseID))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   g.POST("/approve-revert", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body ApproveReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		  }
		  got, err := svc.ApproveRevert(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   g.POST("/execute-revert", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  CaseID string `json:"case_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.ExecuteReversal(c.Request.Context(), strings.TrimSpace(body.CaseID))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	  g.POST("/waive", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body WaiveReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-desk").ID
		   }
		   got, err := svc.Waive(c.Request.Context(), body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	  // ─── 读操作 ────────────────────────────────────────────────────────────
	   g.GET("/case", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   id := strings.TrimSpace(c.Query("id"))
		  if id == "" {
			  c.JSON(400, gin.H{"error": "id required"})
			   return
		   }
		   got, err := svc.Get(c.Request.Context(), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	   g.GET("/cases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  req := ListReq{
			  UserID: c.Query("user_id"),
			  Phase: c.Query("phase"),
			  Asset: c.Query("asset"),
			  CreditRef: c.Query("credit_ref"),
			  Limit: 50,
		   }
		   cases := svc.List(c.Request.Context(), req)
		   c.JSON(200, gin.H{"cases": cases, "count": len(cases)})
	   })
	   g.GET("/evidence", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  id := strings.TrimSpace(c.Query("case_id"))
		  if id == "" {
			   c.JSON(400, gin.H{"error": "case_id required"})
			   return
		   }
		  evs := svc.Evidence(c.Request.Context(), id)
		  c.JSON(200, gin.H{"evidence": evs, "case_id": id})
	   })
	  g.GET("/timeline", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  id := strings.TrimSpace(c.Query("case_id"))
		  if id == "" {
			   c.JSON(400, gin.H{"error": "case_id required"})
			  return
		   }
		   tl := svc.Timeline(c.Request.Context(),
			   id)
		   c.JSON(200, gin.H{"events": tl, "case_id": id})
	   })
	   g.GET("/mutex", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  snap := svc.MutexSnapshot()
		  c.JSON(200, gin.H{"claims": snap, "count": len(snap)})
	   })
	   // ─── 管理端点 ─────────────────────────────────────────────────────────
	  g.POST("/halt", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Halt bool `json:"halt"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  svc.SetHalt(body.Halt)
		   c.JSON(200, gin.H{"halt": body.Halt})
	  })
   }
