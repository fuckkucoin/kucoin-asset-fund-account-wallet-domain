package ledgersvc

import (
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/trace"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
	"strconv"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  v1.POST("/ledger/transfers", func(c *gin.Context) {
		   var body struct {
			  UserID    string `json:"user_id"`
			   Direction string `json:"direction"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			   Chain  string `json:"chain"`
			  ToAddress string `json:"to_address"`
			   TxHash    string `json:"tx_hash"`
			   IdempotencyKey string `json:"idempotency_key"`
			  Wait bool `json:"wait"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   body.UserID = uid
		   amt,
			   err := ledger.ParseAmount(body.Amount)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  tr, err := rt.Ledger.Create(c.Request.Context(), ledger.CreateReq{
			  UserID: body.UserID, Direction: ledger.Direction(body.Direction),
			  Asset: body.Asset, Amount: amt, Chain: body.Chain, ToAddress: body.ToAddress,
			   TxHash: body.TxHash, IdempotencyKey: body.IdempotencyKey, WaitConfirm: body.Wait,
		  })
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
			   return
		   }
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	  })
	   v1.POST("/ledger/freeze", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			   Frozen *bool `json:"frozen"`
			   Reason  string `json:"reason"`
			  Cascade bool   `json:"cascade"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  body.UserID = uid
		   if body.Frozen == nil {
			  c.JSON(400, gin.H{"error": "frozen required"})
			  return
		  }
		  if rt.Durable.Recon != nil {
			  if err := recon.GuardFreezeChange(recon.WrapBook(rt.Durable.Recon, 0), body.UserID); err != nil {
				   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				   return
			  }
		  }
		  if err := rt.Ledger.SetFrozenGate(c.Request.Context(), body.UserID, *body.Frozen, body.Cascade && !*body.Frozen); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   got, _ := rt.Ledger.Frozen(c.Request.Context(), body.UserID)
		   c.JSON(200, gin.H{"user_id": body.UserID, "frozen": got, "reason": body.Reason})
	  })
	   v1.POST("/ledger/transfers/:id/unfreeze", func(c *gin.Context) {
		  var body struct {
			   Epoch  int64  `json:"epoch"`
			  Token  string `json:"token"`
			  Reason string `json:"reason"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  reason := body.Reason
		  if reason == "" {
			   reason = "unfreeze"
		  }
		  if err := rt.Ledger.UnfreezeFenced(c.Request.Context(), c.Param("id"), reason, body.Epoch, body.Token); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   tr, _ := rt.Ledger.Get(c.Request.Context(), c.Param("id"))
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	  })
	   v1.GET("/ledger/fence-rules", func(c *gin.Context) {
		  c.JSON(200, gin.H{"rules": ledger.FenceRules()})
	  })
	   v1.GET("/ledger/transfers/:id/fence", func(c *gin.Context) {
		  var epoch int64
		   if raw := c.Query("epoch"); raw != "" {
			  n, err := strconv.ParseInt(raw, 10, 64)
			  if err != nil {
				   c.JSON(400, gin.H{"error": "epoch"})
				  return
			  }
			  epoch = n
		  }
		  prev, err := rt.Ledger.PreviewFence(c.Request.Context(), c.Param("id"), ledger.FenceOp(c.Query("op")), epoch, c.Query("token"))
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, prev)
	  })
	  v1.GET("/ledger/fence-events", func(c *gin.Context) {
		   ev,
			   err := rt.Ledger.RecentFenceEvents(c.Request.Context(), 50)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"events": ev})
	  })
	   v1.POST("/ledger/transfers/:id/reorg", func(c *gin.Context) {
		  var body struct {
			  Epoch int64  `json:"epoch"`
			  Token string `json:"token"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := rt.Ledger.ReorgFenced(c.Request.Context(), c.Param("id"), body.Epoch, body.Token); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   tr, _ := rt.Ledger.Get(c.Request.Context(), c.Param("id"))
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	  })
	   v1.POST("/ledger/transfers/:id/advance", func(c *gin.Context) {
		   var body struct {
			   Epoch int64  `json:"epoch"`
			   Token string `json:"token"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   tr, err := rt.Ledger.AdvanceSubmitted(c.Request.Context(), c.Param("id"), body.Epoch, body.Token)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
			   return
		   }
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	  })
	  v1.GET("/ledger/transfers/:id", func(c *gin.Context) {
		  tr, err := rt.Ledger.Get(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	  })
	   v1.GET("/ledger/journal", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  list,
			  err := rt.Ledger.ListBills(c.Request.Context(), user, 100)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   if list == nil {
			   list = []ledger.Bill{}
		  }
		  c.JSON(200, gin.H{"user_id": user, "journal": list})
	   })
	   v1.GET("/ledger/reconcile", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   extra, mm, err := rt.InflightFrozen(c.Request.Context(), user)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  rep, err := ledger.ReconcileWith(c.Request.Context(), rt.Store, user, extra)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   ledger.MergeMismatches(rep, mm)
		  rt.Ledger.NoteReconcile(c.Request.Context(), rep)
		   c.JSON(200, rep)
	  })
	   v1.POST("/ledger/kyt/:id/decide", func(c *gin.Context) {
		  var body struct {
			  Actor string `json:"actor"`
			   Decision string `json:"decision"`
			  Reason string `json:"reason"`
		   }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  tr, err := rt.Ledger.DecideKYT(c.Request.Context(), c.Param("id"), body.Actor, body.Decision, body.Reason)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
			  return
		  }
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	  })
	  registerExtra(v1, rt)
   }

  func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime) {
	   v1.POST("/ledger/transfers/:id/confirm", func(c *gin.Context) {
		   if err := rt.Ledger.ConfirmAndCredit(c.Request.Context(), c.Param("id")); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  tr, _ := rt.Ledger.Get(c.Request.Context(), c.Param("id"))
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	   })
	  // 运营回滚；FeeDebit 已入账走 ReverseFee（earn.ReverseEarlyFee 同路径）。
	   // 复用既有 rollback 面接线 ReverseFee，不新开公开口。
	   // 非 FeeDebit credited 不得假装冲正。
	   v1.POST("/ledger/transfers/:id/rollback", func(c *gin.Context) {
		  var body struct {
			   Reason string `json:"reason"`
		   }
		  _ = c.ShouldBindJSON(&body)
		   reason := body.Reason
		   if reason == "" {
			   reason = "rollback"
		  }
		   ctx := c.Request.Context()
		   id := c.Param("id")
		  cur, err := rt.Ledger.Get(ctx, id)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  if cur != nil && cur.Direction == ledger.FeeDebit {
			  if err := rt.Ledger.ReverseFee(ctx, id); err != nil {
				  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				   return
			  }
		   } else if err := rt.Ledger.FailAndRollback(ctx, id, reason); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   tr, _ := rt.Ledger.Get(ctx, id)
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, trace.ID(c.Request.Context()))})
	  })
	  v1.GET("/ledger/transfers", func(c *gin.Context) {
		   rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   user, err := validate.UserID(rawUID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   limit := validate.Limit(atoi(c.Query("limit")), 50, 200)
		   list, err := rt.Ledger.List(c.Request.Context(), user, limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  views := make([]any, 0, len(list))
		  for _, tr := range list {
			   views = append(views, ledger.ViewTransfer(tr, ""))
		   }
		  c.JSON(200, gin.H{"user_id": user, "transfers": views})
	   })
	  v1.GET("/ledger/accounts", func(c *gin.Context) {
		   rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  user, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   books, err := rt.Ledger.Accounts(c.Request.Context(), user)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": user, "accounts": books})
	  })
	  tickets := rt.Durable.Recon
	  if tickets == nil {
		   tickets = recon.New()
	  }
	  registerRecon(v1, rt, tickets)
	  fees := feeeng.New()
	   v1.POST("/ledger/fees/quote", func(c *gin.Context) {
		  var body struct {
			   Kind   string `json:"kind"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			   Volume string `json:"volume"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   q, err := fees.Quote(feeeng.QuoteReq{Kind: body.Kind, Asset: body.Asset, Gross: body.Amount, Volume: body.Volume})
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, q)
	  })
	   registerSafety(v1, rt)
  }

   func atoi(s string) int {
	   n, _ := strconv.Atoi(s)
	  return n
  }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime) {
	  gate := fundsafety.New()
	   if rt != nil {
		   gate.BindLedger(rt.Ledger)
	   }
	  v1.POST("/ledger/refuse-negative", func(c *gin.Context) {
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
		   if err := gate.RefuseNegative(body.Amount); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	   v1.POST("/ledger/refuse-frozen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  UserID string `json:"user_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid,
			  err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		  if _, err := validate.UserID(body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   if err := gate.RefuseFrozen(c.Request.Context(), body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true})
	  })
	  v1.GET("/ledger/drift", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  user, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  if err := rt.Ledger.CheckDrift(user); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true, "user_id": user})
	  })
  }
