package guardlease

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 注册 guardlease HTTP 路由。
   func Register(v1 *gin.RouterGroup, svc *Service) {
	  g := v1.Group("/guardlease")
	  // ---- Lease ----
	  g.POST("/leases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var req IssueLeaseRequest
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		   if req.ActorID == "" {
			  req.ActorID = perm.FromRequest(c.Request, "").ID
		   }
		  lease, err := svc.IssueLease(c.Request.Context(), req)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(201, lease)
	   })
	  g.GET("/leases/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   lease, err := svc.GetLease(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, lease)
	  })
	  g.POST("/leases/:id/bind", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var req BindLeaseRequest
		  if !mw.BindJSON(c, &req) {
			   return
		  }
		   req.LeaseID = c.Param("id")
		  lease, err := svc.BindLease(c.Request.Context(), req)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, lease)
	   })
	  g.GET("/leases", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   leases, err := svc.ListActiveLeases(c.Request.Context(), 50)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"leases": leases})
	  })
	   // ---- Partial Amount Guard ----
	   g.POST("/partial", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var req CheckPartialAmtRequest
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		  im, err := svc.CheckPartialAmt(c.Request.Context(), req)
		  if err != nil {
			  if err == ErrPartialImbalance && im != nil {
				  c.JSON(422, gin.H{"error": err.Error(), "imbalance": im})
				  return
			  }
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true})
	   })
	  // ---- Imbalances ----
	  g.GET("/imbalances", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  ims, err := svc.ListOpenImbalances(c.Request.Context(), 50)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"imbalances": ims})
	  })
	  g.POST("/imbalances/:id/prove", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Actor1 string `json:"actor1"`
			   Actor2 string `json:"actor2"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   im, err := svc.ProveImbalance(c.Request.Context(), c.Param("id"), body.Actor1, body.Actor2)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, im)
	   })
	   g.POST("/imbalances/:id/close", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   actor := strings.TrimSpace(perm.FromRequest(c.Request, "").ID)
		  im, err := svc.CloseImbalance(c.Request.Context(), c.Param("id"), actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, im)
	  })
	   // ---- Conserve Proof ----
	   g.POST("/proofs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var req ConserveProofRequest
		  if !mw.BindJSON(c, &req) {
			  return
		   }
		  proof,
			  err := svc.RecordProof(c.Request.Context(), req)
		  if err != nil {
			  if err == ErrConserveFail {
				  c.JSON(422, gin.H{"error": err.Error(), "proof": proof})
				   return
			   }
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(201, proof)
	  })
	   g.GET("/proofs/failed", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   proofs, err := svc.ListFailedProofs(c.Request.Context(), 50)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"proofs": proofs})
	  })
	   g.POST("/proofs/:id/clear", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var req FourEyesClearRequest
		   if !mw.BindJSON(c, &req) {
			  return
		  }
		   req.ProofID = c.Param("id")
		  proof, err := svc.FourEyesClear(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, proof)
	   })
	  // ---- Status ----
	  g.GET("/status", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{
			   "halted": svc.Halted(),
		  })
	   })
   }
