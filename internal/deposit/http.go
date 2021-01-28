// 充值入账域之 HTTP 接口层。

// Author: Blaise
// Date: 2021.01.07

package deposit

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

  func idOpt(rt *app.Runtime) callerid.Opt {
	  if rt == nil || rt.Cfg == nil {
		  return callerid.Opt{}
	  }
	  return callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
   }

  func bindUser(c *gin.Context, rt *app.Runtime, existing string) (string, error) {
	  return callerid.Bind(c, existing, idOpt(rt))
   }

// applyInbound 入账 credited 后按地址 to 划入目标科目。MAIN 不划，避免改写钱包账用例。

   func applyInbound(rt *app.Runtime, ctx context.Context, tr *ledger.Transfer, idem string) error {
	   if rt == nil || rt.FundSurf == nil || tr == nil || tr.State != ledger.StateCredited {
		   return nil
	  }
	  if idem == "" {
		   idem = tr.IdempotencyKey
	  }
	  return rt.FundSurf.ApplyInbound(ctx, tr.UserID, tr.Chain, tr.Asset, tr.Amount.String(), idem)
  }

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := Attach(rt, New(rt.Ledger, rt.Wallet))
	  v1.POST("/deposits/observe", func(c *gin.Context) {
		   var body struct {
			   UserID         string `json:"user_id"`
			  Chain          string `json:"chain"`
			  TxHash         string `json:"tx_hash"`
			   Asset          string `json:"asset"`
			  IdempotencyKey string `json:"idempotency_key"`
			  Wait bool   `json:"wait"`
			   Memo string `json:"memo"`
			   Tag  string `json:"tag"`
			   Network     string `json:"network"`
			   OutputIndex int    `json:"output_index"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		   uid, err := bindUser(c, rt, body.UserID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   body.UserID = uid
		  if uid, err := validate.UserID(body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   } else {
			   body.UserID = uid
		  }
		  if ch, err := validate.Chain(body.Chain); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  } else {
			   body.Chain = ch
		   }
		  if as, err := validate.Asset(body.Asset); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  } else {
			  body.Asset = as
		  }
		   if _, err := validate.Idem(body.IdempotencyKey); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   if _, err := validate.TxHash(body.TxHash); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  tr, err := svc.Observe(c.Request.Context(), ObserveReq{
			  UserID: body.UserID, Chain: body.Chain, TxHash: body.TxHash,
			  Asset: body.Asset, IdempotencyKey: body.IdempotencyKey, WaitConfirm: body.Wait,
			  Memo: body.Memo, Tag: body.Tag, Network: body.Network, OutputIndex: body.OutputIndex,
		   })
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, "")})
			  return
		  }
		   if err := applyInbound(rt, c.Request.Context(), tr, body.IdempotencyKey); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, "")})
			   return
		  }
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	  })
	  v1.POST("/deposits/:id/confirm", func(c *gin.Context) {
		  if err := svc.Confirm(c.Request.Context(), c.Param("id")); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   tr, _ := svc.Get(c.Request.Context(), c.Param("id"))
		   if err := applyInbound(rt, c.Request.Context(), tr, ""); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, "")})
			   return
		  }
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	  })
	  v1.POST("/deposits/:id/unwind", func(c *gin.Context) {
		   if err := svc.Unwind(c.Request.Context(), c.Param("id")); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  tr, _ := svc.Get(c.Request.Context(), c.Param("id"))
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	  })
	   v1.GET("/deposits/:id", func(c *gin.Context) {
		  tr, err := svc.Get(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	  })
	  v1.GET("/deposits", func(c *gin.Context) {
		   c.JSON(200, gin.H{"runs": svc.Runs().Pending(50), "host": "deposit-withdraw.kucoin.net"})
	  })
	   registerExtra(v1, rt, svc)
	  registerPatrol(v1, rt, svc)
	   registerAttrib(v1, rt, svc)
  }

  func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  v1.POST("/deposits/recover", func(c *gin.Context) {
		  n,
			  err := svc.Recover(c.Request.Context(), svc.Runs(), 50)
		   if err != nil {
			  c.JSON(mw.ProblemOf(c, err).Status, gin.H{"error": err.Error(), "recovered": n})
			   return
		  }
		  c.JSON(200, gin.H{"recovered": n, "host": "deposit-withdraw.kucoin.net"})
	   })
	   v1.GET("/deposits/by-hash", func(c *gin.Context) {
		   chain, err := validate.Chain(c.DefaultQuery("chain", "ETH"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   hash, err := validate.TxHash(c.Query("tx_hash"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  run, ok := svc.Runs().ByHash(chain, hash)
		  if !ok {
			   c.JSON(404, gin.H{"error": "deposit run not found"})
			   return
		   }
		  c.JSON(200, gin.H{"run": run})
	  })
	   v1.POST("/deposits/match", func(c *gin.Context) {
		  var body struct {
			  Chain  string `json:"chain"`
			  TxHash string `json:"tx_hash"`
			  Asset string `json:"asset"`
			   Amount        string `json:"amount"`
			  Address       string `json:"address"`
			   Confirmations int    `json:"confirmations"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  chain,
			  err := validate.Chain(body.Chain)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   hash, err := validate.TxHash(body.TxHash)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   asset, err := validate.Asset(body.Asset)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  if rt == nil || rt.Wallet == nil {
			   mw.WriteProblem(c, errors.New("deposit wallet unavailable"))
			   return
		   }
		  obs, err := rt.Wallet.Inspect(c.Request.Context(), chain, hash)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  if obs.Asset == "" {
			  obs.Asset = asset
		  }
		   m := svc.Matcher()
		   cand, err := m.Match(obs, obs.Asset, obs.Amount.String())
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  var ticket any
		  if !m.CanCredit(cand) {
			  if ex, e := svc.OpenExceptionFromMatch(cand); e == nil {
				  ticket = ex
			  }
		   }
		   out := gin.H{"match": cand, "chain": chain, "host": "deposit-withdraw.kucoin.net"}
		  if ticket != nil {
			   out["exception"] = ticket
		   }
		   c.JSON(200, out)
	   })
	   v1.POST("/deposits/attrib/bind", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			  Chain  string `json:"chain"`
			  Kind string `json:"kind"`
			   Key  string `json:"key"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   uid, err := bindUser(c, rt, body.UserID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  body.UserID = uid
		   kind, err := ParseAttribKind(body.Kind)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   b, err := svc.Attributor().BindUnique(body.UserID, body.Chain, kind, body.Key)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, b)
	   })
	  v1.POST("/deposits/delay", func(c *gin.Context) {
		  var body struct {
			  Chain string `json:"chain"`
			   TxHash string `json:"tx_hash"`
			  Confirms int    `json:"confirmations"`
			   Tier     string `json:"tier"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  row, err := svc.Delays().HoldCredit(body.Chain, body.TxHash, body.Confirms, ParseTier(body.Tier))
		  if err != nil {
			   c.JSON(200, gin.H{"delay": row, "ready": false, "error": err.Error()})
			   return
		   }
		  c.JSON(200, gin.H{"delay": row, "ready": true})
	  })
	  v1.POST("/deposits/attrib/resolve", func(c *gin.Context) {
		   var body struct {
			   Chain string `json:"chain"`
			   Memo string `json:"memo"`
			  Tag  string `json:"tag"`
			   UTXO string `json:"utxo"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  b, err := svc.Attributor().Resolve(body.Chain, body.Memo, body.Tag, body.UTXO, "")
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, b)
	  })
	   registerSafety(v1, rt, svc)
   }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  v1.POST("/deposits/observe-once", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
			  Chain string `json:"chain"`
			   TxHash string `json:"tx_hash"`
			   Asset          string `json:"asset"`
			   IdempotencyKey string `json:"idempotency_key"`
			   WaitConfirm    bool   `json:"wait"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid,
			  err := bindUser(c, rt, body.UserID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   body.UserID = uid
		   if rt != nil && rt.Ledger != nil {
			   if err := rt.Ledger.CheckUser(c.Request.Context(), body.UserID); err != nil {
				   mw.WriteProblem(c, err)
				   return
			   }
		  }
		  tr, err := svc.Observe(c.Request.Context(), ObserveReq{
			  UserID: body.UserID, Chain: body.Chain, TxHash: body.TxHash, Asset: body.Asset,
			   IdempotencyKey: body.IdempotencyKey, WaitConfirm: body.WaitConfirm,
		  })
		   if err != nil {
			   if errors.Is(err, ledger.ErrDuplicateHash) {
				   c.JSON(409, gin.H{"error": err.Error()})
				  return
			  }
			   mw.WriteProblem(c, err)
			  return
		   }
		   if err := applyInbound(rt, c.Request.Context(), tr, body.IdempotencyKey); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"transfer": tr, "host": "deposit-withdraw.kucoin.net"})
	   })
	   v1.POST("/deposits/refuse-frozen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := bindUser(c, rt, body.UserID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  body.UserID = uid
		  if _, err := validate.UserID(body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   if rt != nil && rt.Ledger != nil {
			  if err := rt.Ledger.CheckUser(c.Request.Context(), body.UserID); err != nil {
				  mw.WriteProblem(c, err)
				   return
			  }
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/deposits/unwind-replay", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  ID string `json:"id"`
			   UserID string `json:"user_id"`
			  Chain  string `json:"chain"`
			  TxHash string `json:"tx_hash"`
			  Asset  string `json:"asset"`
			   IdempotencyKey string `json:"idempotency_key"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   if err := svc.Unwind(c.Request.Context(), body.ID); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   tr, err := svc.Observe(c.Request.Context(), ObserveReq{
			   UserID: body.UserID,
			  Chain:  body.Chain, TxHash: body.TxHash, Asset: body.Asset,
			  IdempotencyKey: body.IdempotencyKey, WaitConfirm: true,
		  })
		  if err != nil {
			  c.JSON(200, gin.H{"unwound": true, "error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"unwound": true, "transfer": tr})
	   })
  }
