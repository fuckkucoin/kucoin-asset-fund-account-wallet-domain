package ledgersvc

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func registerRecon(v1 *gin.RouterGroup, rt *app.Runtime, tickets *recon.Book) {
	  v1.POST("/ledger/reconcile/tickets", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   user, err := validate.UserID(c.DefaultQuery("user_id", rt.Cfg.SeedUserID))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   rep, err := ledger.Reconcile(c.Request.Context(), rt.Store, user)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  rt.Ledger.NoteReconcile(c.Request.Context(), rep)
		  mm := make([]recon.Mismatch, 0, len(rep.Mismatches))
		  for _, m := range rep.Mismatches {
			  mm = append(mm, recon.Mismatch{UserID: m.UserID, Book: m.Book, Asset: m.Asset, Note: m.Note})
		  }
		  opened,
			  err := tickets.OpenFrom(actor.ID, mm)
		   if err != nil && len(opened) == 0 && len(mm) > 0 {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"report": rep, "tickets": opened, "open": tickets.OpenCount(user)})
	  })
	  v1.GET("/ledger/reconcile/tickets", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   user := c.Query("user_id")
		  c.JSON(200, gin.H{"tickets": tickets.List(user, validate.Limit(0, 50, 200)), "open": tickets.OpenCount(user)})
	   })
	   v1.POST("/ledger/reconcile/tickets/:id/ack", func(c *gin.Context) {
		  reconAct(c, tickets, tickets.Ack)
	   })
	   v1.POST("/ledger/reconcile/tickets/:id/resolve", func(c *gin.Context) {
		   got := reconAct(c, tickets, tickets.Resolve)
		   if got != nil && got.State == recon.StateResolved && rt.Ledger != nil {
			   rt.Ledger.NoteReconcile(c.Request.Context(), &ledger.ReconcileReport{OK: true, UserID: got.UserID})
		  }
	  })
  }

   func reconAct(c *gin.Context, tickets *recon.Book, fn func(id, actor string, epoch int64, token string) (*recon.Ticket, error)) *recon.Ticket {
	   actor := perm.FromRequest(c.Request, "")
	  if err := actor.Check(perm.ActOpsAdmin); err != nil {
		  c.JSON(403, gin.H{"error": err.Error()})
		return nil
	}
	var body struct {
		Epoch int64  `json:"epoch"`
		Token string `json:"token"`
	}
	if !mw.BindJSON(c, &body) {
		return nil
	}
	got, err := fn(c.Param("id"), actor.ID, body.Epoch, body.Token)
	if err != nil {
		c.JSON(mw.ProblemOf(c, err).Status, gin.H{"error": err.Error()})
		return got
	}
	c.JSON(200, got)
	return got
}
