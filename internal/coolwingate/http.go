// 功能：安全变更冷却标记与出金预检窗口竞态收口 — HTTP 路由。
// 作者：Cason
// 日期：2021.03.25

package coolwingate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func RegisterRoutes(v1 *gin.RouterGroup, eng *Engine) {
	   v1.POST("/coolwingate/mark", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   var req MarkReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		  lease, err := eng.MarkCoolDown(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, lease)
	   })
	   v1.POST("/coolwingate/preflight/bind", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var req PreflightBindReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		   bind, err := eng.BindPreflight(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, bind)
	   })
	  v1.POST("/coolwingate/broadcast/check", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req BroadcastCheckReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   if err := eng.CheckBroadcast(c.Request.Context(), req); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"allowed": true})
	   })
	   v1.POST("/coolwingate/require-withdraw", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var req RequireWithdrawReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		  resp, err := eng.RequireWithdraw(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, resp)
	  })
	   v1.GET("/coolwingate/cool-gen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   uid := c.Query("user_id")
		   resp, err := eng.CoolGenQuery(c.Request.Context(), CoolGenQueryReq{UserID: uid})
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, resp)
	   })
	  v1.POST("/coolwingate/case/confirm", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var req CaseConfirmReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		  wc, err := eng.CaseConfirm(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, wc)
	   })
	   v1.GET("/coolwingate/case/list", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  state := c.Query("state")
		  wid := c.Query("withdraw_id")
		  uid := c.Query("user_id")
		  if wid != "" {
			   cases, err := eng.CaseListByWithdraw(c.Request.Context(), wid, 50)
			  if err != nil {
				   mw.WriteProblem(c, err)
				   return
			  }
			  c.JSON(http.StatusOK, cases)
			  return
		   }
		   if uid != "" {
			  cases,
				   err := eng.CaseListByUser(c.Request.Context(), uid, 50)
			  if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			  c.JSON(http.StatusOK, cases)
			   return
		  }
		  if state != "" {
			   cases, err := eng.CaseList(c.Request.Context(), CaseState(state), 50)
			  if err != nil {
				  mw.WriteProblem(c, err)
				   return
			   }
			   c.JSON(http.StatusOK, cases)
			   return
		   }
		  c.JSON(http.StatusBadRequest, gin.H{"error": "state, withdraw_id, or user_id required"})
	   })
	   v1.GET("/coolwingate/proof/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  uid := c.Query("user_id")
		   caseID := c.Query("case_id")
		   proofs, err := eng.ProofExport(c.Request.Context(), ProofExportReq{
			  UserID: uid,
			   CaseID: caseID,
			  Limit:  100,
		   })
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, proofs)
	   })
	  v1.GET("/coolwingate/lease", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  lid := c.Query("lease_id")
		  uid := c.Query("user_id")
		   if lid != "" {
			   lease, err := eng.GetLease(c.Request.Context(), lid)
			  if err != nil {
				  mw.WriteProblem(c, err)
				   return
			  }
			  c.JSON(http.StatusOK, lease)
			   return
		   }
		  if uid != "" {
			   lease, err := eng.GetLeaseByUser(c.Request.Context(), uid)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			  c.JSON(http.StatusOK, lease)
			   return
		   }
		   c.JSON(http.StatusBadRequest, gin.H{"error": "lease_id or user_id required"})
	  })
	  v1.GET("/coolwingate/bind", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   bid := c.Query("bind_id")
		   wid := c.Query("withdraw_id")
		  if bid != "" {
			  bind, err := eng.GetBind(c.Request.Context(), bid)
			   if err != nil {
				  mw.WriteProblem(c, err)
				   return
			   }
			  c.JSON(http.StatusOK, bind)
			   return
		  }
		  if wid != "" {
			  bind, err := eng.GetBindByWithdraw(c.Request.Context(), wid)
			   if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(http.StatusOK, bind)
			   return
		   }
		   c.JSON(http.StatusBadRequest, gin.H{"error": "bind_id or withdraw_id required"})
	   })
	   v1.POST("/coolwingate/bind/revoke", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  BindID string `json:"bind_id"`
			  Reason string `json:"reason"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   bind, err := eng.RevokeBind(c.Request.Context(), body.BindID, body.Reason)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, bind)
	   })
	  v1.POST("/coolwingate/lease/expire", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  LeaseID string `json:"lease_id"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   lease, err := eng.ExpireLease(c.Request.Context(), body.LeaseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, lease)
	  })
	   v1.POST("/coolwingate/lease/bind", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   LeaseID string `json:"lease_id"`
			   WithdrawID string `json:"withdraw_id"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		   lease, err := eng.BindLease(c.Request.Context(), body.LeaseID, body.WithdrawID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, lease)
	   })
	   v1.POST("/coolwingate/proof/seal", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   ProofID string `json:"proof_id"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   proof, err := eng.SealProof(c.Request.Context(), body.ProofID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, proof)
	   })
   }
