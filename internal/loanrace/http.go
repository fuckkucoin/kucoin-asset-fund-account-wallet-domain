// HTTP 面：对外路由与请求编排。

// Author: Cason
// Date: 2021.08.30

package loanrace

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 注册借贷逾期冻结竞态收口 HTTP API。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   v1.GET("/loanrace/view", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   loanID := c.Query("loan_id")
		  if loanID == "" {
			   c.JSON(400, gin.H{"error": "loan_id required"})
			  return
		  }
		  view, err := svc.GetView(loanID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, view)
	  })
	  v1.GET("/loanrace/overdue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  loanID := c.Query("loan_id")
		   if loanID == "" {
			  c.JSON(400, gin.H{"error": "loan_id required"})
			  return
		  }
		   state, err := svc.GetRaceState(loanID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  frozen := svc.store.IsFrozen(loanID)
		  c.JSON(200, gin.H{
			   "loan_id": loanID,
			  "race_state": state,
			  "frozen": frozen,
		  })
	  })
	  v1.POST("/loanrace/freeze", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   LoanID string `json:"loan_id"`
			   UserID string `json:"user_id"`
			  Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-loanrace").ID
		  }
		   lease, err := svc.FreezeForOverdue(body.LoanID, body.UserID, body.Actor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, lease)
	  })
	   v1.POST("/loanrace/repay/start", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  LoanID  string `json:"loan_id"`
			  LeaseID string `json:"lease_id"`
			   Actor  string `json:"actor"`
			  Amount string `json:"amount"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-loanrace").ID
		  }
		  gate,
			  err := svc.StartControlledRepay(body.LoanID, body.LeaseID, body.Actor, body.Amount)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gate)
	   })
	  v1.POST("/loanrace/repay/pay", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   LoanID string `json:"loan_id"`
			  Amount string `json:"amount"`
			  Actor  string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "ops-loanrace").ID
		  }
		   gate, err := svc.PayThroughGate(body.LoanID, body.Amount, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gate)
	   })
	   v1.POST("/loanrace/repay/complete", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
	   })
	   v1.POST("/loanrace/writeoff/apply", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   LoanID string `json:"loan_id"`
			   Principal string `json:"principal"`
			  Interest string `json:"interest"`
			  Reason string `json:"reason"`
			   Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  wo,
			  err := svc.ApplyWriteoff(body.LoanID, body.Principal, body.Interest, body.Reason, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, wo)
	   })
	   v1.POST("/loanrace/writeoff/approve", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   WriteoffID string `json:"writeoff_id"`
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   wo, err := svc.ApproveWriteoff(body.WriteoffID, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, wo)
	   })
	  v1.POST("/loanrace/writeoff/execute", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   WriteoffID string `json:"writeoff_id"`
			   Actor      string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  wo,
			   err := svc.ExecuteWriteoff(body.WriteoffID, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, wo)
	  })
	  v1.POST("/loanrace/writeoff/reject", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   WriteoffID string `json:"writeoff_id"`
			   Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   wo, err := svc.RejectWriteoff(body.WriteoffID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, wo)
	   })
	   v1.POST("/loanrace/unfreeze/submit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   LoanID string `json:"loan_id"`
			  Kind        string `json:"kind"`
			   EvidenceRef string `json:"evidence_ref"`
			  Actor string `json:"actor"`
			  Note  string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  proof, err := svc.SubmitUnfreezeProof(body.LoanID, body.Kind, body.EvidenceRef, body.Actor, body.Note)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, proof)
	  })
	   v1.POST("/loanrace/unfreeze/validate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  ProofID string `json:"proof_id"`
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   proof, err := svc.ValidateUnfreezeProof(body.ProofID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, proof)
	   })
	   v1.POST("/loanrace/unfreeze/clear", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   LoanID string `json:"loan_id"`
			   Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   err := svc.ClearFrozenWithProof(body.LoanID, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"loan_id": body.LoanID, "frozen": false})
	  })
	  v1.GET("/loanrace/unfreeze/proof", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  loanID := c.Query("loan_id")
		  if loanID == "" {
			  c.JSON(400, gin.H{"error": "loan_id required"})
			  return
		  }
		  proof,
			   err := svc.ProofForLoan(loanID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, proof)
	   })
	   v1.GET("/loanrace/writeoff", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  loanID := c.Query("loan_id")
		   if loanID == "" {
			  c.JSON(400, gin.H{"error": "loan_id required"})
			  return
		  }
		   wo, err := svc.WriteoffForLoan(loanID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, wo)
	   })
	  v1.GET("/loanrace/suspense", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  loanID := c.Query("loan_id")
		  if loanID == "" {
			   c.JSON(400, gin.H{"error": "loan_id required"})
			   return
		  }
		  entries := svc.SuspenseForLoan(loanID)
		  c.JSON(200, gin.H{"loan_id": loanID, "entries": entries})
	  })
	  v1.POST("/loanrace/scan-overdue", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-loanrace").ID
		  }
		   n, err := svc.ScanOverdueWorker(c.Request.Context(), func(_ string) bool { return true }, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"frozen": n})
	  })
	  v1.GET("/loanrace/lease", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  leaseID := c.Query("lease_id")
		   if leaseID == "" {
			   c.JSON(400, gin.H{"error": "lease_id required"})
			  return
		   }
		   lease, err := svc.GetLease(leaseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, lease)
	   })
	   v1.GET("/loanrace/gate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   loanID := c.Query("loan_id")
		  if loanID == "" {
			   c.JSON(400, gin.H{"error": "loan_id required"})
			  return
		   }
		   gate, err := svc.GateForLoan(loanID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gate)
	   })
   }
