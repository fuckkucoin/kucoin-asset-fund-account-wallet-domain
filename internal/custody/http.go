// 托管与多方签发域之 HTTP 接口层。

// Author: Ronan
// Date: 2021.01.20

package custody

import (
	"encoding/hex"
	"errors"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func status(err error) int {
	   switch {
	   case errors.Is(err, ErrSameActor), errors.Is(err, ErrNotApproved):
		  return http.StatusForbidden
	   case errors.Is(err, ErrApprovalNotFound), errors.Is(err, ErrCeremonyNotFound):
		  return http.StatusNotFound
	  case errors.Is(err,
		   ErrApprovalClosed), errors.Is(err, ErrIllegalApproval), errors.Is(err, ErrIllegalCeremony):
		  return http.StatusConflict
	  default:
		   return apierr.HTTPStatus(err)
	  }
  }

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New(rt.Store,
		  rt.Ledger, rt.Enc, rt.Cfg.Transfer.HotMax)
	  svc.UseCeremony(ProcessCeremony())
	  if rt.Durable.Dual != nil {
		  svc.BindQuorum(rt.Durable.Dual)
	  }
	  v1.GET("/custody/inventory", func(c *gin.Context) {
		   rows, err := svc.Inventory(c.Request.Context())
		   if err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{
			  "inventory": rows,
			   "hsm":  svc.HSMKind(),
			  "note": "pkcs11 or cloud-kms ECDSA; hot/cold books",
		  })
	   })
	  v1.GET("/custody/hsm", func(c *gin.Context) {
		   c.JSON(200, gin.H{
			  "keeper": svc.HSMKind(),
			   "label": "PKCS#11 or cloud KMS ECDSA",
		   })
	  })
	   v1.POST("/custody/approvals", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCustodyApprove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Kind       string `json:"kind"`
			   MovementID string `json:"movement_id"`
			   Chain      string `json:"chain"`
			  To string `json:"to"`
			  Amount string `json:"amount"`
			  Note string `json:"note"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		   got, err := svc.approvals.Open(ApprovalKind(body.Kind), actor.ID, body.MovementID, body.Chain, body.To, body.Amount, body.Note)
		   if err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, got)
	  })
	  v1.GET("/custody/approvals", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"approvals": svc.approvals.List(50)})
	   })
	   v1.POST("/custody/approvals/:id/approve", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCustodyApprove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Allow bool `json:"allow"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  got, err := svc.approvals.Approve(c.Param("id"), actor.ID, body.Allow)
		  if err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.POST("/custody/sweep", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCustodySweep); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   ApprovalID string `json:"approval_id"`
		   }
		  _ = c.ShouldBindJSON(&body)
		  if _, err := svc.approvals.RequireApproved(body.ApprovalID, string(KindSweep), ""); err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			  return
		   }
		   if _, err := svc.approvals.MarkExecuted(body.ApprovalID, actor.ID); err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error()})
			  return
		   }
		   svc.Sweep(c.Request.Context())
		   rows, _ := svc.Inventory(c.Request.Context())
		  c.JSON(200, gin.H{"ok": true, "inventory": rows, "hsm": svc.HSMKind()})
	  })
	   v1.POST("/custody/sign", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   ApprovalID string `json:"approval_id"`
			   MovementID     string `json:"movement_id"`
			   Chain          string `json:"chain"`
			   To             string `json:"to"`
			   Amount         string `json:"amount"`
			   Memo           string `json:"memo"`
			   Tag            string `json:"tag"`
			  DestinationTag string `json:"destination_tag"`
			  TagName string `json:"tag_name"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		   if _, err := svc.approvals.RequireApproved(body.ApprovalID, string(KindSign), body.MovementID); err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error()})
			  return
		  }
		   if _, err := svc.approvals.MarkExecuted(body.ApprovalID, actor.ID); err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		  }
		   destTag := body.DestinationTag
		   if destTag == "" {
			  destTag = body.Tag
		  }
		  if destTag == "" {
			  destTag = body.Memo
		   }
		   sig, err := svc.SignWithdraw(body.MovementID, body.Chain, body.To, body.Amount, destTag)
		  if err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"signature_hex": hex.EncodeToString(sig), "hsm": svc.HSMKind()})
	  })
	   // 抬价独立编解码面联签；双人审批后签 CanonicalFeeBump。
	   v1.POST("/custody/sign-fee-bump", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCustodySign); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  ApprovalID string `json:"approval_id"`
			  TransferID string `json:"transfer_id"`
			  Chain      string `json:"chain"`
			   To         string `json:"to"`
			  Amount     string `json:"amount"`
			  NewFee string `json:"new_fee"`
			   Memo           string `json:"memo"`
			   Tag            string `json:"tag"`
			  DestinationTag string `json:"destination_tag"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		  movementID := body.TransferID
		  if _, err := svc.approvals.RequireApproved(body.ApprovalID, string(KindSign), movementID); err != nil {
			  c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		  }
		  if _, err := svc.approvals.MarkExecuted(body.ApprovalID, actor.ID); err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		   }
		  destTag := body.DestinationTag
		  if destTag == "" {
			   destTag = body.Tag
		  }
		   if destTag == "" {
			  destTag = body.Memo
		   }
		  sig, err := svc.SignFeeBump(body.TransferID, body.Chain, body.To, body.Amount, body.NewFee, destTag)
		  if err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			  return
		  }
		  canon := FeeBumpCanonical(body.TransferID, body.Chain, body.To, body.Amount, body.NewFee, destTag)
		  c.JSON(200, gin.H{
			  "signature_hex": hex.EncodeToString(sig), "hsm": svc.HSMKind(),
			   "canonical": canon, "destination_tag": destTag, "path": "custody-sign-fee-bump",
		   })
	   })
	  v1.POST("/custody/ceremony", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActCeremony); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Label     string `json:"label"`
			   Shares    int    `json:"shares"`
			   Threshold int    `json:"threshold"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		   got, err := svc.ceremony.Start(actor.ID, body.Label, body.Shares, body.Threshold)
		   if err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/custody/ceremony", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"ceremonies": svc.ceremony.List(50), "label": "key ceremony recorder"})
	  })
	  v1.POST("/custody/ceremony/:id/advance", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActCeremony); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Phase string `json:"phase"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		  got, err := svc.ceremony.Advance(c.Param("id"), actor.ID, CeremonyPhase(body.Phase))
		  if err != nil {
			   c.JSON(status(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, got)
	   })
	  registerQuorum(v1, rt, svc)
	   registerLots(v1, NewLedger())
  }
