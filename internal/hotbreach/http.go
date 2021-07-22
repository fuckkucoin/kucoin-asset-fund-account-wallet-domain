package hotbreach

import (
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 注册 hotbreach 路由到 v1 RouterGroup。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var kv interface{ GetKV() interface{} }
	   _ = kv
	   svc := buildService(rt)
	  g := v1.Group("/hotbreach")
	   // ── 租约端点 ──
	   g.POST("/leases", withPerm(perm.ActWithdrawSubmit, func(c *gin.Context) {
		  var req IssueLeaseReq
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		   lease, err := svc.IssueLease(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusCreated, lease)
	  }))
	  g.POST("/leases/:id/consume", withPerm(perm.ActWithdrawSubmit, func(c *gin.Context) {
		   id := c.Param("id")
		  var req ConsumeLeaseReq
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		   req.LeaseID = id
		   lease,
			  err := svc.ConsumeLease(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, lease)
	  }))
	   g.DELETE("/leases/:id", withPerm(perm.ActWithdrawSubmit, func(c *gin.Context) {
		  id := c.Param("id")
		  reason := c.Query("reason")
		   if reason == "" {
			   reason = "manual revoke"
		  }
		   if err := svc.RevokeLease(c.Request.Context(), id, reason); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"ok": true, "lease_id": id})
	   }))
	   g.GET("/leases/:id", withPerm(perm.ActQueryRead, func(c *gin.Context) {
		  id := c.Param("id")
		   resp, err := svc.GetLease(c.Request.Context(), id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, resp)
	  }))
	   g.GET("/leases", withPerm(perm.ActQueryRead, func(c *gin.Context) {
		   asset := c.Query("asset")
		   limit := parseIntQuery(c, "limit", 50)
		   leases,
			   err := svc.ListLeases(c.Request.Context(), asset, limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"leases": leases, "total": len(leases)})
	   }))
	   // ── 击穿端点 ──
	   g.GET("/breaches", withPerm(perm.ActQueryRead, func(c *gin.Context) {
		  asset := c.Query("asset")
		  withProofs := c.Query("with_proofs") == "true"
		  limit := parseIntQuery(c, "limit", 50)
		  resp,
			  err := svc.QueryBreaches(c.Request.Context(), asset, withProofs, limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, resp)
	   }))
	   g.GET("/breaches/:id", withPerm(perm.ActQueryRead, func(c *gin.Context) {
		  id := c.Param("id")
		   bc, err := svc.GetBreach(c.Request.Context(), id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, bc)
	   }))
	  g.POST("/breaches/:id/gate", withPerm(perm.ActCustodySweep, func(c *gin.Context) {
		  // ops 手动触发巡检推进
		  _, err := svc.Patrol(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   id := c.Param("id")
		   bc, err := svc.GetBreach(c.Request.Context(), id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, bc)
	  }))
	   g.POST("/breaches/:id/clear", withPerm(perm.ActCustodySweep, func(c *gin.Context) {
		   id := c.Param("id")
		  var req ClearBreachReq
		  if !mw.BindJSON(c, &req) {
			  return
		  }
		  req.BreachID = id
		  bc,
			  err := svc.ClearBreach(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, bc)
	   }))
	   // ── 证明端点 ──
	  g.GET("/proofs/:id", withPerm(perm.ActQueryRead, func(c *gin.Context) {
		  id := c.Param("id")
		   proof, err := svc.GetProof(c.Request.Context(), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, proof)
	  }))
	   // ── 挡写守卫端点（供出金服务内部调用）──
	  g.POST("/guard", withPerm(perm.ActWithdrawSubmit, func(c *gin.Context) {
		  var req GuardReq
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		  if err := svc.Guard(c.Request.Context(), req); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"ok": true, "asset": req.Asset})
	   }))
  }

// buildService 从 Runtime 构造 Service（kv/store nil 时降级内存）。
  func buildService(rt *app.Runtime) *Service {
	   if rt == nil {
		   return NewService(nil, nil, nil)
	  }
	   // Runtime 可能含 KV；此处用 nil 兼容性构造（实际部署需注入KV）
	   svc := NewService(nil, nil, rt.Store)
	  return svc
   }

// withPerm 鉴权中间件包装。
  func withPerm(act perm.Action, handler gin.HandlerFunc) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(act); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  handler(c)
	  }
  }

// parseIntQuery 解析整数 query 参数，缺省返回 defaultVal。

   func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	   s := c.Query(key)
	   if s == "" {
		   return defaultVal
	   }
	  n, err := strconv.Atoi(s)
	  if err != nil || n <= 0 {
		   return defaultVal
	  }
	   return n
  }
