package claimmutex

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 向 v1 路由组注册所有 claimmutex 路由。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   _ = rt
	  RegisterWithService(v1, svc)
  }

// RegisterWithService 将指定 Service 实例注册到路由组（方便注入）。
   func RegisterWithService(v1 *gin.RouterGroup, svc *Service) {
	   g := v1.Group("/claimmutex")
	  // ── 租约操作 ─────────────────────────────────────────────────────────────
	   g.POST("/lease/acquire", requirePerm(perm.ActDepositObserve), func(c *gin.Context) {
		  var req AcquireLeaseRequest
		  if !mw.BindJSON(c, &req) {
			  return
		  }
		   lease,
			   err := svc.AcquireLease(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, lease)
	  })
	   g.POST("/lease/advance", requirePerm(perm.ActDepositObserve), func(c *gin.Context) {
		  var body struct {
			  LeaseID string `json:"lease_id"`
			   ActorID string `json:"actor_id"`
			  Kind    string `json:"kind"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  lease, err := svc.AdvanceLease(c.Request.Context(), body.LeaseID, body.ActorID, body.Kind)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, lease)
	   })
	  g.POST("/lease/block", requirePerm(perm.ActOpsAdmin), func(c *gin.Context) {
		   var body struct {
			  LeaseID string `json:"lease_id"`
			   Reason  string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   if err := svc.BlockOutput(c.Request.Context(), body.LeaseID, body.Reason); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"ok": true})
	  })
	   // ── 自动贷记绑定 ──────────────────────────────────────────────────────────
	   g.POST("/autocredit/register", requirePerm(perm.ActDepositObserve), func(c *gin.Context) {
		   var req RegisterAutoCreditRequest
		  if !mw.BindJSON(c, &req) {
			   return
		  }
		   bind, err := svc.RegisterAutoCredit(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, bind)
	  })
	   // ── 效果证明 ──────────────────────────────────────────────────────────────
	   g.POST("/proof/create", requirePerm(perm.ActDepositObserve), func(c *gin.Context) {
		  var req CreateProofRequest
		   if !mw.BindJSON(c, &req) {
			  return
		   }
		  proof, err := svc.CreateProof(c.Request.Context(), req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, proof)
	   })
	   g.POST("/proof/complete", requirePerm(perm.ActDepositObserve), func(c *gin.Context) {
		  var req CompleteClaimRequest
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		   proof, err := svc.CompleteClaim(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, proof)
	  })
	   // ── 双贷案件 ──────────────────────────────────────────────────────────────
	  g.POST("/dupcase/prove", requirePerm(perm.ActOpsAdmin), func(c *gin.Context) {
		   var req ProveDupCaseRequest
		   if !mw.BindJSON(c, &req) {
			   return
		   }
		   dc,
			  err := svc.ProveDupCase(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, dc)
	   })
	  g.POST("/dupcase/close", requirePerm(perm.ActOpsAdmin), func(c *gin.Context) {
		   var req CloseDupCaseRequest
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		  dc, err := svc.CloseDupCase(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, dc)
	  })
	  // ── 查询接口 ──────────────────────────────────────────────────────────────
	   g.GET("/output/effect", requirePerm(perm.ActQueryRead), func(c *gin.Context) {
		   key := OutputKey{
			  Chain:  strings.TrimSpace(c.Query("chain")),
			  TxHash: strings.TrimSpace(c.Query("tx_hash")),
		  }
		   if idx := strings.TrimSpace(c.Query("output_index")); idx != "" {
			  n, err := strconv.Atoi(idx)
			  if err != nil {
				   c.JSON(http.StatusBadRequest, gin.H{"error": "output_index must be integer"})
				  return
			   }
			   key.OutputIndex = n
		   }
		  if key.String() == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "chain and tx_hash are required"})
			  return
		  }
		  sum,
			  err := svc.GetOutputEffect(c.Request.Context(), key)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, sum)
	   })
	   g.GET("/dupcase/list", requirePerm(perm.ActQueryRead), func(c *gin.Context) {
		   limit := 50
		  if l := c.Query("limit"); l != "" {
			   if n, err := strconv.Atoi(l); err == nil && n > 0 {
				   limit = n
			  }
		  }
		   cases, err := svc.ListOpenDupCases(c.Request.Context(), limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"cases": cases, "count": len(cases)})
	  })
	   g.GET("/proof/pending", requirePerm(perm.ActQueryRead), func(c *gin.Context) {
		   limit := 50
		  if l := c.Query("limit"); l != "" {
			   if n, err := strconv.Atoi(l); err == nil && n > 0 {
				  limit = n
			  }
		  }
		  proofs, err := svc.ListPendingProofs(c.Request.Context(), limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
	   })
  }

  func requirePerm(act perm.Action) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(act); err != nil {
			   c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  c.Next()
	   }
  }
