// 功能：出金提现域之 HTTP 接口层。
// 作者：EliotVoss
// 日期：2021.01.13

package withdraw

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/wdverify"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  screen := kyt.Screener(kyt.Static{Mode: kyt.Allow})
	   if rt != nil && rt.Cfg.KYT.Mode != "" {
		  if sc, err := kyt.NewScreener(rt.Cfg.KYT); err == nil {
			   screen = sc
		   }
	  }
	  svc := AttachWith(rt, compliance.New(screen, policy.NewRegistry(policy.DefaultV1())))
	   BindRuntimeLimit(svc, rt)
	   registerInternal(v1, rt)
	  v1.POST("/withdrawals", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  UserID string `json:"user_id"`
			  Asset string `json:"asset"`
			  Amount string `json:"amount"`
			  Chain  string `json:"chain"`
			   ToAddress string `json:"to_address"`
			   Memo    string `json:"memo"`
			  Tag     string `json:"tag"`
			  TagName string `json:"tag_name"`
			  DestinationTag string `json:"destination_tag"`
			  Comment        string `json:"comment"`
			  IdempotencyKey string `json:"idempotency_key"`
			  Wait           bool   `json:"wait"`
			   TradePassword  string `json:"trade_password"`
			  TOTP           string `json:"totp"`
			  EmailCode      string `json:"email_code"`
			   SMSCode   string `json:"sms_code"`
			   TicketID  string `json:"ticket_id"`
			  DestClass string `json:"dest_class"`
			  Risk int `json:"risk"`
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
		  if uid, err := validate.UserID(body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   } else {
			  body.UserID = uid
		   }
		   if err := validateRequest(&body); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  amt, err := ledger.ParseAmount(body.Amount)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  memo := firstNonEmpty(body.Memo, body.Tag, body.DestinationTag, body.Comment)
		   // 出金提交前统一验证组合闸（禁提/冷却/白名单/因子）；失败结构化返回，不改状态机。
		   if rt != nil && rt.FundSurf != nil {
			  if err := rt.FundSurf.RequireWithdrawSecure(c.Request.Context(), body.UserID, body.Chain, body.ToAddress, wdverify.Presented{
				   TradePassword: body.TradePassword, TOTP: body.TOTP, Email: body.EmailCode, SMS: body.SMSCode,
			   }); err != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
		   } else if rt != nil && rt.SecLock != nil {
			  if err := rt.SecLock.RequireWithdraw(c.Request.Context(), body.UserID); err != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
		  }
		  tr, err := svc.Request(c.Request.Context(), Request{
			  UserID: body.UserID,
			  Asset: body.Asset, Amount: amt, Chain: body.Chain,
			  ToAddress: body.ToAddress,
			   Memo:      memo, Tag: body.Tag, TagName: body.TagName,
			  IdempotencyKey: body.IdempotencyKey, WaitConfirm: body.Wait,
			   TicketID: body.TicketID, DestClass: body.DestClass, Risk: body.Risk,
		  })
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, "")})
			   return
		   }
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	  })
	  v1.POST("/withdrawals/:id/confirm", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   if err := svc.Confirm(c.Request.Context(), c.Param("id")); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   tr, _ := svc.Get(c.Request.Context(), c.Param("id"))
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	   })
	   v1.GET("/withdrawals/:id", func(c *gin.Context) {
		   tr, err := svc.Get(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   run, _ := svc.Runs().ByTransfer(c.Param("id"))
		   c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, ""), "run": run})
	   })
	  v1.GET("/withdrawals", func(c *gin.Context) {
		  c.JSON(200, gin.H{"stuck": svc.Runs().StuckFrozen(50), "host": "deposit-withdraw.kucoin.net"})
	   })
	   v1.POST("/withdrawals/recover", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  n, err := svc.Recover(c.Request.Context(), svc.Runs(), 50)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "recovered": n})
			  return
		   }
		  c.JSON(200, gin.H{"recovered": n})
	   })
	   registerExtra(v1, rt, svc)
	  registerGuard(v1, svc)
   }

  func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   v1.POST("/withdrawals/:id/cancel", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Reason string `json:"reason"`
		  }
		   _ = c.ShouldBindJSON(&body)
		  reason := body.Reason
		  if reason == "" {
			  reason = "cancel"
		  }
		   if err := svc.Cancel(c.Request.Context(), c.Param("id"), reason); err != nil {
			  c.JSON(mw.ProblemOf(c, err).Status, gin.H{"error": err.Error()})
			  return
		  }
		  tr, _ := svc.Get(c.Request.Context(), c.Param("id"))
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	  })
	   v1.POST("/withdrawals/broadcast", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   TransferID string `json:"transfer_id"`
			  Chain      string `json:"chain"`
			   TxHash     string `json:"tx_hash"`
			  Asset string `json:"asset"`
			  Amount string `json:"amount"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   row, err := svc.Broadcasts().Track(body.TransferID, body.Chain, body.TxHash, body.Asset, body.Amount)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, row)
	   })
	  v1.GET("/withdrawals/broadcast/:id", func(c *gin.Context) {
		   row, ok := svc.Broadcasts().Get(c.Param("id"))
		  if !ok {
			  c.JSON(404, gin.H{"error": "broadcast not tracked"})
			   return
		   }
		  c.JSON(200, row)
	   })
	  v1.POST("/withdrawals/broadcast/:id/observe", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Hash          string `json:"hash"`
			  Confirmations int    `json:"confirmations"`
			  Height    int64  `json:"height"`
			  BlockHash string `json:"block_hash"`
			  Reorged   bool   `json:"reorged"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  obs := wallet.Observation{
			  Hash: body.Hash, Confirmations: body.Confirmations, Height: body.Height,
			  BlockHash: body.BlockHash, Reorged: body.Reorged,
		   }
		  if body.Reorged {
			  obs.Status = wallet.TxReorged
		   }
		   row, err := svc.Broadcasts().Observe(c.Param("id"), obs)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "broadcast": row})
			   return
		   }
		   c.JSON(200, gin.H{"broadcast": row})
	   })
	  // 重组后开始 reorg→resubmit saga（resigning）。
	  v1.POST("/withdrawals/broadcast/:id/resubmit/begin", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Note string `json:"note"`
		   }
		  _ = c.ShouldBindJSON(&body)
		   row, err := svc.Broadcasts().BeginResubmit(c.Param("id"), body.Note)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "broadcast": row})
			  return
		   }
		   c.JSON(200, gin.H{"broadcast": row, "saga": row.SagaPhase})
	  })
	  // 新 hash 完成重提（resubmitting→broadcast）。
	   v1.POST("/withdrawals/broadcast/:id/resubmit/complete", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  NewHash string `json:"new_hash"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		   row, err := svc.Broadcasts().CompleteResubmit(c.Param("id"), body.NewHash)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "broadcast": row})
			  return
		   }
		   c.JSON(200, gin.H{"broadcast": row, "saga": row.SagaPhase})
	  })
	  v1.POST("/withdrawals/broadcast/sagas/tick", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  TimeoutSec int `json:"timeout_sec"`
		  }
		   _ = c.ShouldBindJSON(&body)
		  to := DefaultSagaTimeout
		  if body.TimeoutSec > 0 {
			  to = time.Duration(body.TimeoutSec) * time.Second
		   }
		   res := svc.Broadcasts().TickSagas(to)
		  c.JSON(200, gin.H{"tick": res, "open": svc.Broadcasts().OpenSagas()})
	  })
	   v1.GET("/withdrawals/broadcast/sagas", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		   c.JSON(200, gin.H{"sagas": svc.Broadcasts().OpenSagas(), "timed_out": svc.Broadcasts().TimedOutSagas()})
	  })
	   // 列出 saga 超时失败（须人工重开）。
	  v1.GET("/withdrawals/broadcast/sagas/timed-out", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"timed_out": svc.Broadcasts().TimedOutSagas()})
	   })
	  // 超时失败后人工重开 resigning（ops）；保留 old_tx_hash。
	   v1.POST("/withdrawals/broadcast/:id/resubmit/reopen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Note string `json:"note"`
		  }
		  _ = c.ShouldBindJSON(&body)
		   row, err := svc.Broadcasts().ReopenAfterTimeout(c.Param("id"), body.Note)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "broadcast": row})
			  return
		  }
		   c.JSON(200, gin.H{"broadcast": row, "saga": row.SagaPhase, "path": "ops-reopen-after-timeout"})
	   })
	  v1.GET("/withdrawals/fee-bump/:id", func(c *gin.Context) {
		  row, ok := svc.Bumper().Get(c.Param("id"))
		  if !ok {
			   c.JSON(404, gin.H{"error": "fee bump not tracked"})
			   return
		  }
		  c.JSON(200, row)
	   })
	   v1.POST("/withdrawals/fee-bump", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   TransferID string `json:"transfer_id"`
			  Chain      string `json:"chain"`
			   OldHash string `json:"old_hash"`
			   OldFee string `json:"old_fee"`
			   NewFee string `json:"new_fee"`
			   NewHash string `json:"new_hash"`
			  Memo string `json:"memo"`
			   Tag string `json:"tag"`
			   TagName string `json:"tag_name"`
			  DestinationTag string `json:"destination_tag"`
			  Comment        string `json:"comment"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  // 抬价登记并透传 destination_tag（独立编解码面）。
		  hints := BumpHints{
			  Memo: firstNonEmpty(body.Memo,
				  body.Comment), Tag: body.Tag, TagName: body.TagName,
			  DestinationTag: firstNonEmpty(body.DestinationTag, body.Tag, body.Memo, body.Comment),
		   }
		   row, err := svc.Bumper().BumpWithHints(body.TransferID, body.Chain, body.OldHash, body.OldFee, body.NewFee, body.NewHash, hints)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, row)
	   })
	   v1.POST("/withdrawals/fee-bump/encode", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  TransferID string `json:"transfer_id"`
			   From       string `json:"from"`
			  To         string `json:"to"`
			   Asset string `json:"asset"`
			  Amount string `json:"amount"`
			   Memo string `json:"memo"`
			  Tag            string `json:"tag"`
			   TagName        string `json:"tag_name"`
			   DestinationTag string `json:"destination_tag"`
			  AutoSign      bool `json:"auto_sign"`
			   AutoBroadcast bool `json:"auto_broadcast"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  // 抬价 encode；可选 custody 联签 + Registry 自动广播。
		   hints := BumpHints{
			   Memo: body.Memo, Tag: body.Tag, TagName: body.TagName,
			   DestinationTag: firstNonEmpty(body.DestinationTag, body.Tag, body.Memo),
		  }
		  got, err := svc.EncodeSignBroadcastFeeBump(c.Request.Context(), body.TransferID, body.From, body.To, body.Asset, body.Amount, hints, body.AutoSign, body.AutoBroadcast)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  // 抬价 encode→custody 联签→Registry 广播一体（默认全开）。
	  v1.POST("/withdrawals/fee-bump/sign-broadcast", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   TransferID string `json:"transfer_id"`
			  From string `json:"from"`
			  To     string `json:"to"`
			  Asset  string `json:"asset"`
			   Amount string `json:"amount"`
			   Memo   string `json:"memo"`
			  Tag            string `json:"tag"`
			  TagName        string `json:"tag_name"`
			   DestinationTag string `json:"destination_tag"`
			   SkipSign       bool   `json:"skip_sign"`
			  SkipBroadcast  bool   `json:"skip_broadcast"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   hints := BumpHints{
			   Memo: body.Memo, Tag: body.Tag, TagName: body.TagName,
			   DestinationTag: firstNonEmpty(body.DestinationTag, body.Tag, body.Memo),
		   }
		  got,
			   err := svc.EncodeSignBroadcastFeeBump(c.Request.Context(), body.TransferID, body.From, body.To, body.Asset, body.Amount, hints, !body.SkipSign, !body.SkipBroadcast)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/withdrawals/rbf", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   TransferID string `json:"transfer_id"`
			   Chain      string `json:"chain"`
			   OldHash    string `json:"old_hash"`
			   OldFee string `json:"old_fee"`
			  NewFee string `json:"new_fee"`
			  NewHash string `json:"new_hash"`
			   Memo           string `json:"memo"`
			  Tag            string `json:"tag"`
			   TagName        string `json:"tag_name"`
			  DestinationTag string `json:"destination_tag"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   hints := BumpHints{
			   Memo: body.Memo, Tag: body.Tag, TagName: body.TagName,
			   DestinationTag: firstNonEmpty(body.DestinationTag, body.Tag, body.Memo),
		   }
		   row, err := svc.ReplaceUTXOWithHints(body.TransferID, body.Chain, body.OldHash, body.OldFee, body.NewFee, body.NewHash, hints)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, row)
	   })
	  v1.GET("/withdrawals/stuck", func(c *gin.Context) {
		  c.JSON(200, gin.H{"stuck": svc.ScanStuck(svc.Broadcasts().clock(), 0)})
	   })
	   v1.POST("/withdrawals/stuck/recover", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   TransferID string `json:"transfer_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  st, err := svc.RecoverStuck(body.TransferID, svc.Broadcasts().clock())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, st)
	   })
	  v1.POST("/withdrawals/nonce", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   Chain string `json:"chain"`
			  From  string `json:"from"`
			   Nonce uint64 `json:"nonce"`
			  TxHash string `json:"tx_hash"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := svc.Nonces().Reserve(body.Chain, body.From, body.Nonce, body.TxHash); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"next": svc.Nonces().Next(body.Chain, body.From)})
	  })
	   v1.POST("/withdrawals/nonce/repair", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCustodySign); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Chain string `json:"chain"`
			   From  string `json:"from"`
			   Nonce uint64 `json:"nonce"`
			  TxHash string `json:"tx_hash"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.Nonces().Repair(body.Chain, body.From, body.Nonce, body.TxHash); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"next": svc.Nonces().Next(body.Chain, body.From)})
	  })
	  registerSafety(v1, rt, svc)
   }

   func validateRequest(body *struct {
	  UserID string `json:"user_id"`
	   Asset     string `json:"asset"`
	   Amount    string `json:"amount"`
	  Chain     string `json:"chain"`
	   ToAddress string `json:"to_address"`
	  Memo string `json:"memo"`
	   Tag            string `json:"tag"`
	   TagName        string `json:"tag_name"`
	  DestinationTag string `json:"destination_tag"`
	   Comment string `json:"comment"`
	   IdempotencyKey string `json:"idempotency_key"`
	   Wait          bool   `json:"wait"`
	  TradePassword string `json:"trade_password"`
	  TOTP string `json:"totp"`
	  EmailCode string `json:"email_code"`
	  SMSCode   string `json:"sms_code"`
	   TicketID  string `json:"ticket_id"`
	   DestClass string `json:"dest_class"`
	  Risk int `json:"risk"`
  }) error {
	   if as, err := validate.Asset(body.Asset); err != nil {
		  return err
	   } else {
		   body.Asset = as
	  }
	  if ch, err := validate.Chain(body.Chain); err != nil {
		  return err
	   } else {
		   body.Chain = ch
	  }
	   if _, err := validate.Idem(body.IdempotencyKey); err != nil {
		  return err
	  }
	  if _, err := validate.Amount(body.Amount); err != nil {
		  return err
	  }
	  if addr, err := validate.Address(body.Chain, body.ToAddress); err != nil {
		   return err
	   } else {
		  body.ToAddress = addr
	   }
	  return nil
  }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   gate := fundsafety.New()
	  if rt != nil {
		  gate.BindLedger(rt.Ledger)
	  }
	  v1.POST("/withdrawals/refuse-frozen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  UserID string `json:"user_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  body.UserID = uid
		  if err := gate.RefuseFrozen(c.Request.Context(), body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"ok": true, "host": "deposit-withdraw.kucoin.net"})
	  })
	   v1.POST("/withdrawals/refuse-negative", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
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
		  if _, err := validate.Amount(body.Amount); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	   _ = svc
   }

   func registerInternal(v1 *gin.RouterGroup, rt *app.Runtime) {
	  v1.POST("/withdrawals/internal", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   UserID string `json:"user_id"`
			  ToUID   string `json:"to_uid"`
			   ToEmail string `json:"to_email"`
			   ToPhone string `json:"to_phone"`
			  Asset   string `json:"asset"`
			  Amount  string `json:"amount"`
			   IdempotencyKey string `json:"idempotency_key"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
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
		   amt, err := ledger.ParseAmount(body.Amount)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  dir := rt.Users
		  svc := fundint.New(rt.Ledger,
			   dir)
		  tr, err := svc.Send(c.Request.Context(), fundint.Req{
			  UserID: body.UserID, ToUID: body.ToUID, ToEmail: body.ToEmail, ToPhone: body.ToPhone,
			   Asset: body.Asset, Amount: amt, IdempotencyKey: body.IdempotencyKey,
		  })
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, "")})
			   return
		  }
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, ""), "host": "deposit-withdraw.kucoin.net"})
	  })
	   // PostSecurityCooldown
	  // 运维写入安全/身份变更后的出金冷却。
	  // 身份原因走 MarkIdentityNormalized；其余走 Mark。
	  // 须鉴权 ActOpsAdmin；原因未归一或写账失败不得返回成功。
	   v1.POST("/security/cooldown", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  UserID string `json:"user_id"`
			  Reason string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
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
		  reason := seclock.NormalizeReason(body.Reason)
		  sec := seclock.New(rt.Ledger)
		  if seclock.IdentityReason(reason) {
			  err = sec.MarkIdentityNormalized(c.Request.Context(), body.UserID, reason)
		  } else {
			   err = sec.Mark(c.Request.Context(), body.UserID, reason)
		   }
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"ok": true, "user_id": body.UserID, "reason": reason, "host": "deposit-withdraw.kucoin.net"})
	   })
   }

  func firstNonEmpty(vals ...string) string {
	  for _, v := range vals {
		  if s := strings.TrimSpace(v); s != "" {
			   return s
		   }
	   }
	  return ""
   }
