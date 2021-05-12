package feeresgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/feereserve"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// Register 向 gin v1 路由组注册 feeresgate 接口，兼容 svcboot.Main 签名。
  func Register(v1 *gin.RouterGroup, _ *app.Runtime) {
	   eng := New(Config{
		  Store: feereserve.NewMemStore(),
	   })
	   registerRoutes(v1, eng)
  }

   func registerRoutes(v1 *gin.RouterGroup, eng *Engine) {
	  v1.POST("/feeresgate/reserve", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req feereserve.ReserveReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   lease,
			   err := eng.Reserve(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, lease)
	  })
	  v1.POST("/feeresgate/settle", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var req feereserve.SettleReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  lease, err := eng.Settle(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, lease)
	  })
	  v1.POST("/feeresgate/release", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var req feereserve.ReleaseReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  lease, err := eng.Release(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, lease)
	   })
	   v1.POST("/feeresgate/block", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  var req feereserve.BlockReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		   }
		   lease, err := eng.Block(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, lease)
	   })
	   v1.GET("/feeresgate/lease", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   wid := c.Query("withdraw_id")
		  lid := c.Query("lease_id")
		  if lid != "" {
			  lease, err := eng.GetLease(c.Request.Context(), lid)
			   if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(http.StatusOK, lease)
			  return
		   }
		   if wid != "" {
			   lease, err := eng.GetLeaseByWithdraw(c.Request.Context(), wid)
			   if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(http.StatusOK, lease)
			   return
		  }
		   c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_id or lease_id required"})
	   })
	   v1.POST("/feeresgate/case/confirm", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  var req feereserve.CaseConfirmReq
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   rc, err := eng.CaseConfirm(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, rc)
	  })
	  v1.GET("/feeresgate/case/list", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  state := c.Query("state")
		   wid := c.Query("withdraw_id")
		   if wid != "" {
			   cases, err := eng.CaseListByWithdraw(c.Request.Context(), wid, 50)
			   if err != nil {
				  mw.WriteProblem(c, err)
				   return
			  }
			  c.JSON(http.StatusOK, cases)
			  return
		  }
		   if state != "" {
			  cases, err := eng.CaseList(c.Request.Context(), feereserve.CaseState(state), 50)
			  if err != nil {
				  mw.WriteProblem(c, err)
				   return
			   }
			   c.JSON(http.StatusOK, cases)
			  return
		  }
		   c.JSON(http.StatusBadRequest, gin.H{"error": "state or withdraw_id required"})
	  })
	  v1.GET("/feeresgate/proof/export", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  wid := c.Query("withdraw_id")
		  lid := c.Query("lease_id")
		   proofs, err := eng.ProofExport(c.Request.Context(), feereserve.ProofExportReq{
			   WithdrawID: wid,
			   LeaseID:    lid,
			   Limit:      100,
		   })
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, proofs)
	   })
  }
