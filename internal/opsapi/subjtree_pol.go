package opsapi

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/acctpol"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/subjtree"
)

// SubjtreeHub 科目树 ops hub（注入依赖）。

type SubjtreeHub struct {
	Tree  *subjtree.Store
	Pol   *acctpol.Store
	Eng   *acctpol.Engine
	Audit *acctpol.AuditStore
	Dual  *acctpol.DualReadLog
	Gate *acctpol.Gate
}

// RegisterSubjtreePol 注册科目树与政策运营端点到 v1 路由组。
   func RegisterSubjtreePol(v1 *gin.RouterGroup, sh *SubjtreeHub, el interface {
	   RequireLeader() error
	  Status() interface{}
  }) {
	  if sh == nil {
		   return
	  }
	   // ── 科目树：只读 ──────────────────────────────────────────────
	   // GET /ops/subjtree/versions → 所有代次号列表
	  v1.GET("/ops/subjtree/versions", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   vers := sh.Tree.Versions(c.Request.Context())
		  c.JSON(http.StatusOK, gin.H{"versions": vers})
	  })
	  // GET /ops/subjtree/current → 当前代次
	   v1.GET("/ops/subjtree/current", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  tv, err := sh.Tree.Current(c.Request.Context())
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, tv)
	   })
	   // GET /ops/subjtree/version/:ver → 指定代次
	  v1.GET("/ops/subjtree/version/:ver", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   ver, err := strconv.Atoi(c.Param("ver"))
		   if err != nil || ver <= 0 {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version"})
			  return
		  }
		   tv, err := sh.Tree.Get(c.Request.Context(), ver)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, tv)
	   })
	  // ── 科目树：写（需选主）──────────────────────────────────────
	   // POST /ops/subjtree/publish → 发布新代次
	  v1.POST("/ops/subjtree/publish", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  if err := el.RequireLeader(); err != nil {
			  c.JSON(leaderStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  var tv subjtree.TreeVersion
		  if err := c.ShouldBindJSON(&tv); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   if err := sh.Tree.Publish(c.Request.Context(), tv, actor.ID); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"version": tv.Version, "actor": actor.ID})
	   })
	  // POST /ops/subjtree/close-node → 关闭科目（生成派生代次）
	  v1.POST("/ops/subjtree/close-node", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  if err := el.RequireLeader(); err != nil {
			  c.JSON(leaderStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   NodeID string `json:"node_id"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.NodeID) == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "node_id required"})
			  return
		   }
		  derived, err := sh.Tree.CloseNode(c.Request.Context(), body.NodeID, actor.ID)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"derived_version": derived.Version, "actor": actor.ID})
	   })
	  // ── 政策代次：只读 ───────────────────────────────────────────
	  // GET /ops/acctpol/versions → 所有代次号
	   v1.GET("/ops/acctpol/versions", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   vers := sh.Pol.Versions(c.Request.Context())
		  c.JSON(http.StatusOK, gin.H{"versions": vers})
	  })
	   // GET /ops/acctpol/current → 当前活跃代次
	   v1.GET("/ops/acctpol/current", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   pv,
			   err := sh.Pol.Current(c.Request.Context())
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, pv)
	   })
	   // GET /ops/acctpol/version/:ver → 指定代次
	  v1.GET("/ops/acctpol/version/:ver", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   ver, err := strconv.Atoi(c.Param("ver"))
		   if err != nil || ver <= 0 {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version"})
			  return
		   }
		  pv, err := sh.Pol.Get(c.Request.Context(), ver)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, pv)
	  })
	  // GET /ops/acctpol/audit → 审计日志（最近 100 条）
	   v1.GET("/ops/acctpol/audit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   if sh.Audit == nil {
			  c.JSON(http.StatusOK, gin.H{"entries": []struct{}{}})
			  return
		   }
		   action := acctpol.AuditAction(c.Query("action"))
		   limit := 100
		   if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
			   limit = l
		   }
		   entries := sh.Audit.List(action,
			  limit)
		   c.JSON(http.StatusOK, gin.H{"entries": entries})
	  })
	  // GET /ops/acctpol/dualread → 双读差异统计 + 最近记录
	   v1.GET("/ops/acctpol/dualread", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  if sh.Dual == nil {
			  c.JSON(http.StatusOK, gin.H{"total": 0, "diverge": 0})
			  return
		   }
		  total, diverge := sh.Dual.Stats()
		  recent := sh.Dual.Recent(20)
		   c.JSON(http.StatusOK, gin.H{"total": total, "diverge": diverge, "recent": recent})
	  })
	   // ── 政策代次：写（需选主）────────────────────────────────────
	   // POST /ops/acctpol/publish → 发布新策略代次
	   v1.POST("/ops/acctpol/publish", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   if err := el.RequireLeader(); err != nil {
			  c.JSON(leaderStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Policy acctpol.PolicyVersion `json:"policy"`
			  Reason string                `json:"reason"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   if err := sh.Pol.Publish(c.Request.Context(), body.Policy, actor.ID); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   if sh.Audit != nil {
			  _ = sh.Audit.RecordPublish(c.Request.Context(), actor.ID, body.Policy.Version, body.Reason)
		   }
		  c.JSON(http.StatusOK, gin.H{"version": body.Policy.Version, "actor": actor.ID})
	   })
	  // POST /ops/acctpol/rollback → 回滚到历史代次
	  v1.POST("/ops/acctpol/rollback", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  if err := el.RequireLeader(); err != nil {
			  c.JSON(leaderStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  TargetVersion int `json:"target_version"`
			  Reason string `json:"reason"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil || body.TargetVersion <= 0 {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "target_version required"})
			   return
		   }
		   curPV, _ := sh.Pol.Current(c.Request.Context())
		  fromVer := 0
		   if curPV.Version > 0 {
			   fromVer = curPV.Version
		  }
		   if err := sh.Pol.Rollback(c.Request.Context(), body.TargetVersion); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  if sh.Pol != nil {
			  _ = sh.Pol.PersistRollback(c.Request.Context(), body.TargetVersion)
		   }
		  if sh.Audit != nil {
			  _ = sh.Audit.RecordRollback(c.Request.Context(),
				   actor.ID, fromVer, body.TargetVersion, body.Reason)
		  }
		  c.JSON(http.StatusOK, gin.H{
			  "from_version":   fromVer,
			  "target_version": body.TargetVersion,
			  "actor":          actor.ID,
		  })
	  })
	   // POST /ops/acctpol/pin → 用户 Pin 到指定代次
	   v1.POST("/ops/acctpol/pin", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   if err := el.RequireLeader(); err != nil {
			   c.JSON(leaderStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID  string `json:"user_id"`
			  Version int    `json:"version"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.UserID) == "" || body.Version <= 0 {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id and version required"})
			   return
		   }
		   if sh.Eng != nil {
			   sh.Eng.PinUser(body.UserID, body.Version)
		   }
		  if sh.Audit != nil {
			  _ = sh.Audit.RecordPinUser(c.Request.Context(), actor.ID, body.UserID, body.Version)
		  }
		   c.JSON(http.StatusOK, gin.H{"user_id": body.UserID, "version": body.Version, "actor": actor.ID})
	   })
	   // POST /ops/acctpol/unpin → 解除用户 Pin
	  v1.POST("/ops/acctpol/unpin", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  if err := el.RequireLeader(); err != nil {
			   c.JSON(leaderStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  UserID string `json:"user_id"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.UserID) == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			   return
		  }
		   if sh.Eng != nil {
			  sh.Eng.UnpinUser(body.UserID)
		  }
		  if sh.Audit != nil {
			   _ = sh.Audit.RecordUnpinUser(c.Request.Context(), actor.ID, body.UserID, 0)
		   }
		  c.JSON(http.StatusOK, gin.H{"user_id": body.UserID, "actor": actor.ID})
	   })
	  // POST /ops/acctpol/gate-check → 调试：检查单次 hop 准入
	   v1.POST("/ops/acctpol/gate-check", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  if sh.Gate == nil {
			   c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gate not initialized"})
			  return
		  }
		  var req acctpol.GateRequest
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   result,
			  err := sh.Gate.Check(c.Request.Context(), req)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(http.StatusOK, result)
	  })
   }

  func leaderStatus(err error) int {
	   if err != nil && strings.Contains(err.Error(), "not leader") {
		   return http.StatusConflict
	  }
	  return apierr.HTTPStatus(err)
  }
