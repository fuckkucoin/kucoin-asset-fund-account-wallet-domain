// 功能：资金安全巡检与告警之 HTTP 接口层。
// 作者：CainReed
// 日期：2021.06.18

package fundsafety

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  if rt != nil && rt.Ledger != nil {
		  svc.BindLedger(rt.Ledger)
		  if rt.Wallet != nil {
			   svc.BindDeposit(deposit.New(rt.Ledger, rt.Wallet))
		   }
	  }
	  if rt != nil {
		  var hv HaltViewer
		  if rt.FundSurf != nil {
			   hv = gateView{g: rt.FundSurf.HaltGate()}
		   } else if rt.KV != nil {
			   hv = gateView{g: custodyhalt.New(rt.KV)}
		   }
		  var rv ReconViewer
		  if rt.Durable.Recon != nil {
			   rv = reconAdapt{w: recon.WrapBook(rt.Durable.Recon, 0)}
		  }
		  svc.BindCombo(hv, rv)
		   if hv != nil {
			   if gv, ok := hv.(gateView); ok {
				  svc.BindLane(laneAdapt{g: gv.g})
			   }
		  }
	  }
	   v1.POST("/fundsafety/claim", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body CreditClaim
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got,
			  err := svc.ClaimHash(body.Chain, body.TxHash, body.Idem, body.UserID, body.Asset)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	  v1.POST("/fundsafety/partial", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body Partial
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := svc.CheckPartial(body); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true})
	  })
	  v1.POST("/fundsafety/negative", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Amount string `json:"amount"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := svc.RefuseNegative(body.Amount); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"ok": true})
	  })
	  v1.POST("/fundsafety/halt", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  On bool `json:"on"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   svc.SetHalt(body.On)
		   c.JSON(200, gin.H{"halt": svc.Halted()})
	  })
	  v1.GET("/fundsafety/claims", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"claims": svc.Claims(), "host": "custody-boundary-lab.kucoin.net"})
	  })
	  v1.POST("/fundsafety/batch", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  UserID    string `json:"user_id"`
			  Asset     string `json:"asset"`
			  Requested string `json:"requested"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.OpenBatch(body.UserID, body.Asset, body.Requested)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	  v1.POST("/fundsafety/journal", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Lines []JournalLine `json:"lines"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.CheckJournal(body.Lines)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/fundsafety/confirm", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Chain string `json:"chain"`
			   Hash  string `json:"tx_hash"`
			   Have int `json:"have"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.OpenConfirm(body.Chain, body.Hash, body.Have)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	   v1.GET("/fundsafety/combo", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  uid, chain, asset := c.Query("user_id"), c.Query("chain"), c.Query("asset")
		   verdict := svc.ComboOfScope(c.Request.Context(), uid, chain, asset)
		  c.JSON(200, gin.H{
			  "verdict": verdict,
			  "user_id": uid, "chain": chain, "asset": asset,
			  "host": "custody-boundary-lab.kucoin.net",
		  })
	  })
	  v1.GET("/fundsafety/combo/board", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, svc.BoardOf(c.Request.Context(), c.Query("user_id"), c.Query("chain"), c.Query("asset")))
	  })
	  v1.POST("/fundsafety/deposit-patrol", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body deposit.PatrolIn
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   var book *recon.Book
		   if rt != nil {
			   book = rt.Durable.Recon
		  }
		  out, err := svc.DepositPatrol(body, book)
		  if err != nil {
			  c.JSON(409, gin.H{"error": err.Error(), "result": out})
			  return
		   }
		   c.JSON(200, out)
	   })
	   v1.POST("/fundsafety/deposit-sweep", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Rows []deposit.PatrolIn `json:"rows"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   var book *recon.Book
		   if rt != nil {
			   book = rt.Durable.Recon
		   }
		   out, err := svc.DepositSweep(body.Rows, book)
		  if err != nil {
			   c.JSON(409, gin.H{"error": err.Error(), "results": out, "withdraw_request_blocked": false})
			   return
		   }
		   c.JSON(200, gin.H{"results": out, "withdraw_request_blocked": false})
	  })
  }

type gateView struct{ g *custodyhalt.Gate }

  func (v gateView) Halted(ctx context.Context) (bool, error) {
	  if v.g == nil {
		   return false, ErrUnavailable
	  }
	  return v.g.Halted(ctx)
   }

  func (v gateView) CheckActive(ctx context.Context) error {
	   if v.g == nil {
		  return ErrUnavailable
	   }
	   return v.g.CheckActive(ctx)
  }

   func (v gateView) CheckScope(ctx context.Context, chain, asset string) error {
	  if v.g == nil {
		   return ErrUnavailable
	   }
	  return v.g.CheckScope(ctx,
		   chain, asset)
  }

type reconAdapt struct{ w *recon.Workflow }

   func (r reconAdapt) BlocksUser(userID string) bool {
	  return r.w != nil && r.w.BlocksUser(userID)
   }

  func (r reconAdapt) OpenCount(userID string) int {
	  if r.w == nil || r.w.Book() == nil {
		  return 0
	  }
	  return r.w.Book().OpenCount(userID)
  }

type laneAdapt struct{ g *custodyhalt.Gate }

  func (l laneAdapt) CheckLane(ctx context.Context, lane string, chain, asset string) error {
	  if l.g == nil {
		  return ErrUnavailable
	   }
	   return l.g.CheckLane(ctx,
		  custodyhalt.Lane(lane), chain, asset)
  }

  func (l laneAdapt) CheckPair(ctx context.Context, chain, asset string) error {
	   if l.g == nil {
		   return ErrUnavailable
	   }
	   return l.g.CheckPair(ctx, chain, asset)
   }
