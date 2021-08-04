package kcfund

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/address"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/wdverify"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"account-assets.kucoin.net/funding/internal/xferloop"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ErrNotFavorite 白名单模式下地址簿未命中；公开 REST 业务码 260325（大小写敏感全等）。

var ErrNotFavorite = errors.New("Already set withdraw whitelist, this address is not favorite address")

const CodeNotFavorite = "260325"

   func (a *API) deposit(g *gin.RouterGroup) {
	   g.POST("/v3/deposit-address/create", a.addDepositAddress)
	   g.POST("/v1/deposit-addresses", a.addDepositAddress)
	  g.POST("/v1/deposit-addresses/assign", a.assignDepositAddress)
	   g.POST("/v3/deposit-addresses/assign", a.assignDepositAddress)
	   g.POST("/v1/deposit-addresses/release", a.releaseDepositAddress)
	   g.POST("/v3/deposit-addresses/release", a.releaseDepositAddress)
	   g.POST("/v1/deposit-addresses/recycle", a.recycleDepositAddress)
	   g.POST("/v3/deposit-addresses/recycle", a.recycleDepositAddress)
	   g.POST("/v1/deposit-addresses/reclaim", a.reclaimDepositAddress)
	   g.POST("/v3/deposit-addresses/reclaim", a.reclaimDepositAddress)
	   g.POST("/v1/deposit-addresses/assign-fair", a.assignDepositFair)
	   g.POST("/v3/deposit-addresses/assign-fair", a.assignDepositFair)
	  g.POST("/v1/deposit-addresses/assign-sticky", a.assignDepositSticky)
	   g.POST("/v3/deposit-addresses/assign-sticky", a.assignDepositSticky)
	  g.GET("/v3/deposit-addresses", a.getDepositAddress)
	   g.GET("/v2/deposit-addresses", a.getDepositAddress)
	   g.GET("/v1/deposit-addresses", a.getDepositAddress)
	   g.GET("/v3/deposit-addresses/assigned", a.listAssignedDeposit)
	  g.GET("/v1/deposit-addresses/assigned", a.listAssignedDeposit)
	   g.GET("/v1/deposit-addresses/item", a.getAssignedDeposit)
	   g.GET("/v1/deposits", a.getDepositHistory)
	  g.GET("/v1/hist-deposits", a.getDepositHistory)
  }

   func (a *API) withdraw(g *gin.RouterGroup) {
	  g.GET("/v1/withdrawals/quotas", a.getWithdrawQuotas)
	  g.POST("/v3/withdrawals", a.withdrawV3)
	   g.POST("/v1/withdrawals", a.withdrawV3)
	  g.DELETE("/v1/withdrawals/:withdrawalId", a.cancelWithdraw)
	   g.GET("/v1/withdrawals", a.getWithdrawHistory)
	   g.GET("/v1/hist-withdrawals", a.getWithdrawHistory)
	   g.GET("/v1/withdrawals/:withdrawalId", a.getWithdrawByID)
	  g.GET("/v1/withdrawals/addresses", a.listWithdrawAddresses)
	  g.GET("/v1/withdrawals/addresses/item", a.getWithdrawAddress)
	   g.GET("/v1/withdrawals/addresses/check", a.checkWithdrawDest)
	   g.GET("/v1/withdrawals/whitelist-pref", a.getWhitelistPref)
	   g.GET("/v1/withdrawals/destallow", a.listDestAllow)
	   g.GET("/v1/withdrawals/destallow/export", a.exportDestAllow)
	   g.GET("/v1/withdrawals/addresses/audit", a.listWithdrawAudit)
	   g.GET("/v1/withdrawals/addresses/export", a.exportWithdrawAddresses)
	  g.GET("/v1/withdrawals/addresses/quota", a.getAddressQuota)
	  g.POST("/v1/withdrawals/addresses/batch", a.batchWithdrawAddresses)
	   g.POST("/v1/withdrawals/addresses/import", a.importWithdrawAddresses)
	   g.GET("/v1/withdrawals/addresses/anomaly", a.getAddressAnomaly)
	  g.GET("/v1/withdrawals/destallow/snapshot", a.destAllowSnapshot)
	  g.GET("/v1/withdrawals/addresses/dups", a.listAddressDups)
	  g.POST("/v1/withdrawals/addresses", a.addWithdrawAddress)
	   g.PUT("/v1/withdrawals/addresses", a.updateWithdrawAddress)
	  g.POST("/v1/withdrawals/addresses/disable", a.disableWithdrawAddress)
	   g.POST("/v1/withdrawals/addresses/enable", a.enableWithdrawAddress)
	  g.POST("/v1/withdrawals/addresses/restore", a.restoreWithdrawAddress)
	   g.DELETE("/v1/withdrawals/addresses", a.deleteWithdrawAddress)
	   g.POST("/v1/withdrawals/destallow", a.addDestAllow)
	  g.POST("/v1/withdrawals/destallow/temp", a.addDestAllowTemp)
	  g.POST("/v1/withdrawals/destallow/precheck-batch", a.precheckDestAllowBatch)
	   g.POST("/v1/withdrawals/destallow/import", a.importDestAllow)
	   g.DELETE("/v1/withdrawals/destallow", a.deleteDestAllow)
	   g.POST("/v1/withdrawals/whitelist-pref", a.setWhitelistPref)
	   // UTA Account & Funding 出金轨（公开 docs-new /api/ua/v2/asset/withdrawal）。
	   g.POST("/ua/v2/asset/withdrawal", a.withdrawV3)
	  g.GET("/ua/v2/asset/withdrawals", a.getWithdrawHistory)
	   g.GET("/ua/v2/asset/withdrawals/:withdrawalId", a.getWithdrawByID)
	   g.DELETE("/ua/v2/asset/withdrawals/:withdrawalId", a.cancelWithdraw)
	  g.POST("/ua/v2/asset/withdrawal/cancel", a.cancelWithdrawBody)
	   // docs-new UTA Cancel Withdrawal 官方 path（withdraw/cancel + withdrawId）。
	   g.POST("/ua/v2/asset/withdraw/cancel", a.cancelWithdrawUTA)
   }

   func (a *API) addDepositAddress(c *gin.Context) {
	   var body struct {
		   Currency string `json:"currency"`
		   Chain    string `json:"chain"`
		  To       string `json:"to"`
		  Memo string `json:"memo"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
		  return
	   }
	  chain, err := validate.Chain(body.Chain)
	   if err != nil {
		  fail(c, 400, err.Error())
		   return
	  }
	  uid, okU := a.requireUser(c)
	   if !okU {
		   return
	  }
	   to := strings.ToUpper(strings.TrimSpace(body.To))
	  if to == "" {
		   to = "MAIN"
	  }
	   memo := strings.TrimSpace(body.Memo)
	   if memo == "" {
		   memo = fundsurf.MemoFor(uid, chain)
	   }
	   if g := a.mesh(); g != nil {
		   view, err := g.DepositAddress(c.Request.Context(), a.meshOpts(c), uid, chain)
		  if err != nil {
			  if a.rt != nil {
				  a.rt.Log.Warn().Err(err).Str("user", uid).Str("chain", chain).Msg("mesh deposit address")
			  }
			  fail(c, 503, "downstream unavailable")
			  return
		  }
		  if strings.TrimSpace(view.Address) != "" {
			   _ = a.surf().SetDepositTo(c.Request.Context(), uid, chain, to)
			   g.NotifySidecar(c.Request.Context(), a.meshOpts(c), "deposit", "/v1/deposits/observe/ping")
			   ok(c, a.depositAddrView(uid, chain, view.Address, body.Currency, to))
			  return
		   }
	  }
	  view, err := a.bookSvc().AssignDeposit(uid, body.Currency, chain, memo)
	  if err != nil {
		   fail(c, 400, err.Error())
		  return
	   }
	  addr := strings.TrimSpace(view.Address)
	   if addr == "" {
		   fail(c, 400, "deposit address unavailable")
		   return
	   }
	   _ = a.surf().SetDepositTo(c.Request.Context(), uid, chain, to)
	   a.sideCall(c, "address-custody", "GET", "/v1/addresses/deposit")
	  ok(c, a.depositAddrView(uid, chain, addr, body.Currency, to))
   }

  func (a *API) getDepositAddress(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		  return
	   }
	   raw := strings.TrimSpace(c.Query("chain"))
	   chains := []string{}
	  if raw != "" {
		  chain, err := validate.Chain(raw)
		   if err != nil {
			   fail(c, 400, err.Error())
			  return
		   }
		  chains = []string{chain}
	   } else if a.rt != nil && a.rt.Cfg != nil {
		  for ch := range a.rt.Cfg.Chains {
			   if got, err := validate.Chain(ch); err == nil {
				   chains = append(chains, got)
			   }
		   }
	  }
	   if len(chains) == 0 {
		   fail(c, 400, "chain required")
		  return
	  }
	   svc := address.New(a.rt.Ledger, a.rt.Wallet)
	   out := make([]gin.H, 0, len(chains))
	  for _, chain := range chains {
		  addr := ""
		   got, okAddr, errMesh := a.meshDepositAddress(c, uid, chain)
		   if errMesh != nil {
			   fail(c, 503, "downstream unavailable")
			   return
		   }
		   if okAddr {
			  addr = got
		  }
		  if addr == "" {
			  got, err := svc.DepositAddress(c.Request.Context(), uid, chain)
			   if err != nil || got == "" {
				   continue
			  }
			   addr = got
		  }
		   to := a.surf().DepositTo(c.Request.Context(), uid, chain)
		  out = append(out, a.depositAddrView(uid, chain, addr, c.Query("currency"), to))
	  }
	  ok(c, out)
  }

  func (a *API) depositAddrView(uid, chain, addr, currency, to string) gin.H {
	   cur := strings.ToUpper(strings.TrimSpace(currency))
	   contract := ""
	  if a.rt != nil && a.rt.Cfg != nil {
		  if spec, okc := a.rt.Cfg.Chains[chain]; okc && spec.Tokens != nil {
			   if tok, okc := spec.Tokens[cur]; okc {
				   contract = tok.Contract
			   }
		  }
	   }
	  memo := fundsurf.MemoFor(uid,
		   chain)
	   tagName := fundsurf.MemoTagName(chain)
	  // FamMEMO 共享地址领用时自动绑 memo/tag 归因，供扫链观察认领。
	  if fundsurf.NeedsMemo(chain) && memo != "" && a.rt != nil {
		  svc := address.New(a.rt.Ledger, a.rt.Wallet)
		  if a.rt.SQL != nil && !a.rt.Mem {
			   svc.BindSQL(a.rt.SQL)
		  }
		  if a.rt.SharedMemo != nil {
			  if shared, ok := a.rt.SharedMemo.(*addrpool.SharedMemoPool); ok {
				  svc.BindSharedMemo(shared, nil)
			   }
		   }
		  if a.rt.DepositAttr != nil {
			   if attr, ok := a.rt.DepositAttr.(*deposit.Attributor); ok {
				   svc.BindSharedMemo(svc.SharedPool(), deposit.AsMemoBinder(attr))
			   } else if binder, ok := a.rt.DepositAttr.(addrpool.MemoBinder); ok {
				   svc.BindSharedMemo(svc.SharedPool(), binder)
			   }
		   }
		   if b, err := svc.BindFamMemo(uid, chain, memo); err == nil && b.Address != "" {
			   addr = b.Address
		  }
	   }
	   view := gin.H{
		   "address": addr, "memo": memo, "chain": chain,
		   "to": to, "currency": cur, "contractAddress": contract,
		   "needMemo": fundsurf.NeedsMemo(chain), "tagName": tagName,
		   "shared": fundsurf.NeedsMemo(chain),
		   "format": wallet.FormatSpec(chain),
	   }
	  // XRP 公开字段 destinationTag 与 memo 同值；其他备注链保留 memo。
	   if tagName == "destination_tag" && memo != "" {
		   view["destinationTag"] = memo
	  }
	   if tagName == "comment" && memo != "" {
		  view["comment"] = memo
	  }
	   return view
  }

   func (a *API) getDepositHistory(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	  if !okU {
		   return
	   }
	   page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
	   if page <= 0 {
		  page, _ = strconv.Atoi(strings.TrimSpace(c.Query("page")))
	  }
	  pageSize, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
	   startAt := parseUnixOrRFC(c.Query("startAt"))
	   endAt := parseUnixOrRFC(c.Query("endAt"))
	  csv := strings.EqualFold(c.Query("format"), "csv")
	   if csv {
		  var err error
		  startAt,
			  endAt, err = applyExportWindow(startAt, endAt)
		   if err != nil {
			   fail(c, 400, err.Error())
			  return
		   }
		   if pageSize <= 0 {
			   pageSize = ledger.DepositHistMaxPage
		   }
	   }
	  q := ledger.DepositHistQuery{
		  UserID: uid, Page: page, PageSize: pageSize,
		  Start: startAt, End: endAt,
		  Currency: strings.ToUpper(strings.TrimSpace(c.Query("currency"))),
		  Status:   strings.ToUpper(strings.TrimSpace(c.Query("status"))),
	  }
	  q.SetInnerMode(c.Query("isInner"))
	   cols := []string{"id", "currency", "chain", "amount", "status", "isInner", "fromAddress", "memo", "tag", "tagName", "createdAt", "confirmations", "requiredConfirmations"}
	  if csv {
		   // 大窗 CSV 翻页流式写出，超越单次 MaxPage 截断。
		  streamHistCSV(c, cols, func(pg, psz int) ([]gin.H, int, error) {
			  qq := q
			   qq.Page, qq.PageSize = pg, psz
			  list, total, err := a.rt.Ledger.ListDepositsMerged(c.Request.Context(), qq)
			   if err != nil {
				   return nil, 0, err
			  }
			  items := make([]gin.H, 0, len(list))
			   for _, tr := range list {
				   if tr == nil {
					   continue
				  }
				   items = append(items, a.depositHistRow(tr, tr.Direction == ledger.SpotSend))
			   }
			  return items, total, nil
		  })
		  return
	  }
	  // 双源统一按 created_at 降序合并分页；status/currency/isInner 在 SQL 层过滤。
	   list, total, err := a.rt.Ledger.ListDepositsMerged(c.Request.Context(), q)
	   if err != nil {
		  fail(c, 400, err.Error())
		   return
	  }
	  items := make([]gin.H, 0, len(list))
	  for _, tr := range list {
		  if tr == nil {
			  continue
		   }
		   incomingInner := tr.Direction == ledger.SpotSend
		  row := a.depositHistRow(tr, incomingInner)
		  items = append(items, row)
	   }
	   if q.Page <= 0 {
		   q.Page = 1
	  }
	   if q.PageSize <= 0 {
		   q.PageSize = ledger.DepositHistDefaultPage
	  }
	   totalPage := 1
	  if q.PageSize > 0 && total > 0 {
		   totalPage = (total + q.PageSize - 1) / q.PageSize
	  }
	   ok(c, gin.H{
		   "currentPage": q.Page, "pageSize": q.PageSize, "totalNum": total, "totalPage": totalPage,
		  "items": items, "retentionDays": int(ledger.DepositHistRetention / (24 * time.Hour)),
		  "merged": true,
	  })
  }

  func (a *API) depositHistRow(tr *ledger.Transfer, incomingInner bool) gin.H {
	  need := a.confirmNeed(tr.Chain)
	  conf := 0
	   if tr.State == ledger.StateCredited || tr.State == ledger.StateConfirmed {
		  conf = need
	   }
	  inner := incomingInner || ledger.IsInternalTxHash(tr.TxHash) || tr.Direction == ledger.SpotSend
	   if inner {
		  need, conf = 0, 0
	   }
	   row := gin.H{
		   "currency": tr.Asset, "chain": tr.Chain, "amount": tr.Amount.String(),
		  "walletTxId": tr.TxHash, "status": railStatus(tr), "id": tr.ID,
		   "isInner": inner, "address": tr.ToAddressHint,
		  "fromAddress": tr.FromAddressHint,
		  "memo":        tr.MemoHint,
		  "tag":         tr.TagHint,
		   "tagName": tr.TagNameHint,
		  "createdAt":     tr.CreatedAt.UnixMilli(),
		   "confirmations": conf, "requiredConfirmations": need,
		   "isEarlyCredit": !inner && tr.State == ledger.StateCredited && need > a.confirmFloor(),
	  }
	   if tr.State == ledger.StateCredited || tr.State == ledger.StateConfirmed {
		  // 已入账行显式带 fromAddress（可为空串，表示链上未回填）
		   row["fromAddress"] = tr.FromAddressHint
	  }
	  return row
   }

   func (a *API) getWithdrawQuotas(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		  return
	  }
	   cur := strings.ToUpper(strings.TrimSpace(bffParam(c, "currency")))
	   chain := strings.ToUpper(strings.TrimSpace(bffParam(c, "chain")))
	  lim := "0"
	   if a.rt.Cfg != nil {
		  lim = a.rt.Cfg.Risk.DailyWithdrawLimit
	   }
	  avail := a.withdrawable(c, uid, cur)
	   enabled := true
	  reason := ""
	  if forbid, err := a.surf().Forbidden(c.Request.Context(), uid); err == nil && forbid {
		  enabled, reason = false, "USER_FORBIDDEN"
	  }
	   accountFrozen := false
	   if frozen, err := a.rt.Ledger.Frozen(c.Request.Context(), uid); err == nil && frozen {
		  enabled, reason = false, "ACCOUNT_FROZEN"
		   accountFrozen = true
	   }
	  lock, err := a.surf().Lock(c.Request.Context(), uid)
	  if err == nil && lock.Locked {
		  enabled, reason = false, "SECURITY_LOCK"
	  }
	   usedQuota, usedBTC := a.withdrawUsedToday(c, uid)
	   limDec,
		  _ := decimal.NewFromString(lim)
	  if !limDec.IsPositive() {
		   limDec = decimal.Zero
	   }
	   remainQuota := limDec.Sub(usedQuota)
	  if remainQuota.IsNegative() {
		   remainQuota = decimal.Zero
	  }
	   limitBTC, usedBTCAmt := a.quotaBTCAmounts(limDec, usedQuota, usedBTC)
	   remainBTC := limitBTC.Sub(usedBTCAmt)
	  if remainBTC.IsNegative() {
		   remainBTC = decimal.Zero
	  }
	   fee := a.withdrawFee()
	  resp := gin.H{
		   "currency": cur, "chain": chain,
		  "availableAmount": avail, "remainAmount": remainQuota.String(),
		  "limitQuotaCurrencyAmount": lim, "quotaCurrency": "USDT",
		  "isWithdrawEnabled": enabled,
		   "withdrawMinFee":    fee, "innerWithdrawMinFee": "0",
		   "withdrawMinSize": a.minAmt(), "precision": 8, "lockedAmount": "0",
		  "reason": reason, "lockRemainSeconds": lock.RemainSec, "lockUntil": lock.Until,
		  "limitBTCAmount": limitBTC.String(), "usedBTCAmount": usedBTCAmt.String(),
		   "usedQuotaCurrencyAmount": usedQuota.String(),
		   "remainingQuotaAmount": remainQuota.String(),
		  "availableWithdrawAmount": avail,
		  "minWithdrawFee":          fee, "minInnerWithdrawFee": "0",
		  "remainBTCAmount": remainBTC.String(),
	   }
	   a.patchWithdrawQuotasJSON(resp, lim, usedQuota, remainQuota, accountFrozen)
	   ok(c, resp)
  }

// withdrawUsedToday 统计当日未失败出金占用的计价币（USDT）与 BTC 用量。
  func (a *API) withdrawUsedToday(c *gin.Context, uid string) (quotaUSDT, btc decimal.Decimal) {
	   if a == nil || a.rt == nil || a.rt.Ledger == nil {
		   return decimal.Zero, decimal.Zero
	  }
	   list,
		  err := a.rt.Ledger.List(c.Request.Context(), uid, 500)
	   if err != nil {
		   return decimal.Zero, decimal.Zero
	   }
	  now := time.Now().UTC()
	  dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	  for _, tr := range list {
		   if tr == nil || tr.Direction != ledger.WalletWithdraw {
			   continue
		  }
		  if tr.State == ledger.StateFailed || tr.State == ledger.StateRolledBack {
			  continue
		   }
		   if tr.CreatedAt.Before(dayStart) {
			   continue
		  }
		   usdt := a.assetToUSDT(tr.Asset, tr.Amount)
		  quotaUSDT = quotaUSDT.Add(usdt)
		   btc = btc.Add(a.assetToBTC(tr.Asset,
			   tr.Amount))
	  }
	  return quotaUSDT, btc
  }

  func (a *API) quotaBTCAmounts(limitUSDT, usedUSDT, usedBTC decimal.Decimal) (limitBTC, adjUsedBTC decimal.Decimal) {
	  if rate, ok := a.usdtBTCRate(); ok && rate.IsPositive() {
		  limitBTC = limitUSDT.Mul(rate)
		  // 先用流水折算的 BTC；若无流水则用计价占用反推。
		  if usedBTC.IsPositive() {
			  adjUsedBTC = usedBTC
		   } else if usedUSDT.IsPositive() {
			   adjUsedBTC = usedUSDT.Mul(rate)
		  }
		   return limitBTC, adjUsedBTC
	  }
	   return decimal.Zero, usedBTC
   }

  func (a *API) usdtBTCRate() (decimal.Decimal, bool) {
	   if a == nil || a.rt == nil || a.rt.Market == nil {
		  return decimal.Zero, false
	  }
	   // Rate(from,to)：1 USDT 可兑多少 BTC。
	   return a.rt.Market.Rate("USDT", "BTC")
   }

   func (a *API) assetToUSDT(asset string, amt decimal.Decimal) decimal.Decimal {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if !amt.IsPositive() {
		  return decimal.Zero
	   }
	  if asset == "USDT" || asset == "USD" {
		  return amt
	   }
	  if a == nil || a.rt == nil || a.rt.Market == nil {
		  return decimal.Zero
	   }
	  if rate, ok := a.rt.Market.Rate(asset, "USDT"); ok {
		  return amt.Mul(rate)
	  }
	  return decimal.Zero
  }

   func (a *API) assetToBTC(asset string, amt decimal.Decimal) decimal.Decimal {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if !amt.IsPositive() {
		   return decimal.Zero
	   }
	   if asset == "BTC" {
		   return amt
	  }
	  if a == nil || a.rt == nil || a.rt.Market == nil {
		   return decimal.Zero
	  }
	  if rate, ok := a.rt.Market.Rate(asset, "BTC"); ok {
		   return amt.Mul(rate)
	   }
	  return decimal.Zero
  }

  func (a *API) withdrawable(c *gin.Context, uid, cur string) string {
	   rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	   if err != nil {
		   return "0"
	  }
	   total := decimal.Zero
	  for _, r := range rows {
		   kc := bookToKC(r.Subject, r.Book)
		   if kc != "main" && kc != "trade" {
			  continue
		  }
		   if cur != "" && strings.ToUpper(r.Asset) != cur {
			   continue
		  }
		   amt,
			  err := decimal.NewFromString(r.Available)
		   if err != nil {
			   continue
		  }
		  total = total.Add(amt)
	  }
	  return total.String()
   }

  func (a *API) withdrawFee() string {
	  if a.rt == nil || a.rt.Cfg == nil {
		  return "0"
	  }
	   bps := a.rt.Cfg.Transfer.WithdrawFeeBPS
	  if bps <= 0 {
		  return "0"
	  }
	  return decimal.NewFromInt(int64(bps)).Div(decimal.NewFromInt(10000)).String()
   }

  func (a *API) minAmt() string {
	  if a.rt.Cfg != nil && a.rt.Cfg.Risk.MinAmount != "" {
		  return a.rt.Cfg.Risk.MinAmount
	  }
	  return "0.00000001"
  }

   func (a *API) withdrawV3(c *gin.Context) {
	   var body struct {
		  Currency  string `json:"currency"`
		  ToAddress string `json:"toAddress"`
		  Amount    string `json:"amount"`
		  Chain string `json:"chain"`
		   WithdrawType string `json:"withdrawType"`
		   IsInner      bool   `json:"isInner"`
		   Remark       string `json:"remark"`
		  Memo string `json:"memo"`
		   Tag string `json:"tag"`
		  DestinationTag string `json:"destinationTag"`
		   Comment        string `json:"comment"`
		   ClientOid      string `json:"clientOid"`
		   ToUID          string `json:"toUid"`
		  ToEmail       string `json:"toEmail"`
		   ToPhone       string `json:"toPhone"`
		   TradePassword string `json:"tradePassword"`
		   AccountType   string `json:"accountType"`
		   TOTPCode      string `json:"totpCode"`
		   EmailCode       string `json:"emailCode"`
		   SMSCode         string `json:"smsCode"`
		   TravelMessageID string `json:"travelMessageId"`
		   BeneficiaryVasp string `json:"beneficiaryVasp"`
		  VASP string `json:"vasp"`
		  TravelRule struct {
			   VASP string `json:"vasp"`
			   DID  string `json:"did"`
			  LEI             string `json:"lei"`
			  Type            string `json:"type"`
			  BeneficiaryName string `json:"beneficiaryName"`
		  } `json:"travelRule"`
	  }
	  if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
		   fail(c, 400, err.Error())
		  return
	  }
	  body.Currency = strings.TrimSpace(firstNonEmpty(body.Currency, bffParam(c, "currency")))
	  body.Chain = strings.TrimSpace(firstNonEmpty(body.Chain, bffParam(c, "chain")))
	  body.ToAddress = strings.TrimSpace(firstNonEmpty(body.ToAddress, firstNonEmpty(bffParam(c, "toAddress"), bffParam(c, "address"))))
	  body.Amount = strings.TrimSpace(firstNonEmpty(body.Amount, bffParam(c, "amount")))
	  body.ClientOid = strings.TrimSpace(firstNonEmpty(body.ClientOid, firstNonEmpty(bffParam(c, "clientOid"), bffParam(c, "client_oid"))))
	  body.Memo = strings.TrimSpace(firstNonEmpty(body.Memo, bffParam(c, "memo")))
	  uid, okU := a.requireUser(c)
	   if !okU {
		  return
	  }
	  presented := wdverify.Presented{
		   TradePassword: body.TradePassword, TOTP: body.TOTPCode, Email: body.EmailCode, SMS: body.SMSCode,
	  }
	  if err := a.surf().RequireWithdrawSecure(c.Request.Context(), uid, body.Chain, body.ToAddress, presented); err != nil {
		  fail(c, 403, err.Error())
		  return
	   }
	  if typ := strings.ToUpper(strings.TrimSpace(body.AccountType)); typ != "" && typ != "MAIN" && typ != "TRADE" && typ != "FUNDING" && typ != "TRADING" {
		   fail(c, 400, "withdraw only from MAIN or TRADE")
		  return
	  }
	   amt,
		   err := ledger.ParseAmount(body.Amount)
	   if err != nil {
		  fail(c, 400, err.Error())
		  return
	  }
	   idem := strings.TrimSpace(body.ClientOid)
	   if idem == "" {
		   fail(c, 400, "clientOid required")
		  return
	   }
	   a.sideCall(c, "risk-kyt", "POST", "/v1/kyt/screen")
	  wt := strings.ToUpper(strings.TrimSpace(body.WithdrawType))
	  if body.IsInner || wt == "UID" || wt == "INTERNAL" || wt == "MAIL" || wt == "PHONE" {
		  toUID, toEmail, toPhone := body.ToUID, body.ToEmail, body.ToPhone
		  switch wt {
		  case "UID", "INTERNAL":
			   toUID = firstNonEmpty(toUID, body.ToAddress)
		   case "MAIL":
			  toEmail = firstNonEmpty(toEmail, body.ToAddress)
		   case "PHONE":
			  toPhone = firstNonEmpty(toPhone, body.ToAddress)
		   default:
			   toUID = firstNonEmpty(toUID, body.ToAddress)
		  }
		   svc := fundint.New(a.rt.Ledger, a.rt.Users).
			   BindGate(func(ctx context.Context, userID string) error {
				  return a.surf().RequireWithdraw(ctx,
					  userID)
			  }).
			  BindMinAmount(a.minAmt())
		  tr, err := svc.Send(c.Request.Context(), fundint.Req{
			   UserID: uid, ToUID: toUID,
			  ToEmail: toEmail, ToPhone: toPhone,
			   Asset: body.Currency, Amount: amt, IdempotencyKey: idem,
			  Remark: body.Remark,
		  })
		   if err != nil {
			  fail(c, 400, err.Error())
			  return
		  }
		  a.note(c.Request.Context(), "wd-inner:"+tr.ID, notify.KindWithdrawBroadcast, tr.ID)
		   ok(c, gin.H{"withdrawalId": tr.ID})
		  return
	   }
	   chain,
		   err := validate.Chain(body.Chain)
	  if err != nil {
		  fail(c, 400, err.Error())
		   return
	  }
	  addr, err := validate.Address(chain, body.ToAddress)
	   if err != nil {
		   fail(c, 400, err.Error())
		   return
	   }
	  if err := a.requireWhitelist(c, uid, chain, addr); err != nil {
		   if errors.Is(err, ErrNotFavorite) {
			  failCode(c, 400, CodeNotFavorite, err.Error())
			   return
		   }
		   fail(c, 400, err.Error())
		  return
	  }
	   peer := firstNonEmpty(firstNonEmpty(body.TravelRule.VASP, body.VASP), body.BeneficiaryVasp)
	   travelFlds := travelWithdrawFields{TravelMessageID: body.TravelMessageID, BeneficiaryVasp: peer}
	   if err := a.enforceTravelWithdraw(uid, chain, addr, body.Currency, body.Amount, travelFlds); err != nil {
		  if errors.Is(err, travelvasp.ErrTravelNeed) {
			   failCode(c, 400, "411000", err.Error())
			   return
		  }
		  fail(c, 400, err.Error())
		   return
	   }
	   if err := a.ensureTravelOutbound(c, uid, chain, addr, body.Currency, body.Amount, idem, peer, body.TravelRule.DID, body.TravelRule.LEI, body.TravelRule.Type, body.TravelRule.BeneficiaryName); err != nil {
		   if code := travelvasp.CodeOf(err); code != "" {
			   failCode(c, 403, code, err.Error())
			   return
		  }
		   fail(c, 403, err.Error())
		  return
	  }
	   if raw, err := json.Marshal(body); err == nil {
		  if a.forwardMeshDepositWithdraw(c, http.MethodPost, "/v1/withdrawals", raw) {
			  if g := a.mesh(); g != nil {
				   g.NotifySidecar(c.Request.Context(), a.meshOpts(c), "withdraw", "/v1/withdrawals/observe/ping")
			   }
			  return
		   }
	  }
	  svc := a.withdrawSvc()
	  if svc == nil {
		  fail(c, 503, "withdraw service unavailable")
		  return
	  }
	   // 出金申请落 memo_hint/tag_name_hint；公开 memo/destinationTag/comment 别名。
	   memo := firstNonEmpty(firstNonEmpty(body.Memo, body.Tag), firstNonEmpty(body.DestinationTag, body.Comment))
	   // 请求未带备注时，从出金簿回填（一致地址簿 memo 面）。
	   if memo == "" {
		   book := address.New(a.rt.Ledger, a.rt.Wallet)
		  if m, _, err := book.BookMemo(c.Request.Context(), uid, chain, addr); err == nil && m != "" {
			  memo = m
		  }
	  }
	   tr,
		  err := svc.Request(c.Request.Context(), withdraw.Request{
		  UserID: uid, Asset: body.Currency, Amount: amt, Chain: chain,
		  ToAddress: addr, Memo: memo, Tag: body.Tag, IdempotencyKey: idem,
	   })
	   if err != nil {
		  fail(c, 400, err.Error())
		  return
	  }
	   a.bindTravelWithdraw(c.Request.Context(), tr.ID, uid, chain, addr, body.Currency, body.Amount, travelFlds)
	   a.rebindTravelMovement(idem,
		   tr.ID)
	  a.note(c.Request.Context(), "wd:"+tr.ID, notify.KindWithdrawBroadcast, tr.ID)
	   view := gin.H{
		  "withdrawalId": tr.ID,
		  "memo":    tr.MemoHint,
		  "tagName": tr.TagNameHint,
		  "needMemo": fundsurf.NeedsMemo(chain),
	  }
	   if cid, ok := c.Get("travel_case_id"); ok {
		   view["travelRule"] = gin.H{"caseId": cid, "status": c.GetString("travel_case_status")}
	   }
	  ok(c, view)
  }

// withdrawSvc 复用 Runtime 上的出金单例；未挂载时按配置 KYT 构造并 Attach。
  func (a *API) withdrawSvc() *withdraw.Service {
	   if a == nil || a.rt == nil {
		   return nil
	   }
	  if svc := withdraw.FromRuntime(a.rt); svc != nil {
		  return svc
	  }
	   screen := kyt.Screener(kyt.Static{Mode: kyt.Allow})
	  if a.rt.Cfg != nil && a.rt.Cfg.KYT.Mode != "" {
		   if sc, err := kyt.NewScreener(a.rt.Cfg.KYT); err == nil {
			   screen = sc
		  }
	   }
	   return withdraw.AttachWith(a.rt, compliance.New(screen, policy.NewRegistry(policy.DefaultV1())))
   }

  func (a *API) requireWhitelist(c *gin.Context, uid, chain, addr string) error {
	  if a.rt == nil || a.rt.FundSurf == nil || !a.rt.FundSurf.WhitelistOnly(c.Request.Context(), uid) {
		  return nil
	  }
	  svc := a.bookSvc()
	  ok,
		   err := svc.Allowlisted(c.Request.Context(), uid, chain, addr)
	  if err != nil {
		   return err
	   }
	  if ok {
		  _, _ = svc.TouchLastUsed(c.Request.Context(), uid, chain, addr, "")
		  return nil
	   }
	  if a.rt.DestAllow != nil {
		  if err := destallow.PrecheckWithdraw(c.Request.Context(), a.rt.DestAllow, a.surf(), uid, chain, addr); err == nil {
			   return nil
		  }
	  }
	   return ErrNotFavorite
  }

  func (a *API) cancelWithdraw(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		   return
	  }
	   wid := c.Param("withdrawalId")
	  if a.mesh() != nil {
		   if a.forwardMeshDepositWithdraw(c, http.MethodDelete, "/v1/withdrawals/"+wid, nil) {
			   return
		   }
		   fail(c, 503, "fundmesh deposit-withdraw unavailable")
		   return
	  }
	   tr,
		   err := a.rt.Ledger.Get(c.Request.Context(), wid)
	  if err != nil || tr == nil || tr.UserID != uid {
		   fail(c, 404, "withdrawal not found")
		   return
	   }
	   svc := a.withdrawSvc()
	  if svc == nil {
		   fail(c, 503, "withdraw service unavailable")
		   return
	   }
	  if err := svc.Cancel(c.Request.Context(), c.Param("withdrawalId"), "cancel"); err != nil {
		   fail(c, 400, err.Error())
		  return
	  }
	   tr2, _ := a.rt.Ledger.Get(c.Request.Context(), c.Param("withdrawalId"))
	  if tr2 == nil {
		   tr2 = tr
	   }
	   a.emitWithdrawStatus(c.Request.Context(), tr2, "CANCELLED")
	   ok(c, withdrawByIDView(a, tr2))
  }

  func (a *API) getWithdrawHistory(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		  return
	  }
	  page, _ := strconv.Atoi(strings.TrimSpace(bffParam(c, "currentPage", "page")))
	  if page <= 0 {
		   page, _ = strconv.Atoi(strings.TrimSpace(bffParam(c, "page")))
	  }
	   pageSize, _ := strconv.Atoi(strings.TrimSpace(bffParam(c, "pageSize")))
	  startAt := parseUnixOrRFC(bffParam(c, "startAt"))
	  endAt := parseUnixOrRFC(bffParam(c, "endAt"))
	  csv := strings.EqualFold(bffParam(c, "format"), "csv")
	   if csv {
		  var err error
		  startAt, endAt, err = applyExportWindow(startAt, endAt)
		   if err != nil {
			  fail(c, 400, err.Error())
			  return
		  }
		   if pageSize <= 0 {
			   pageSize = ledger.WithdrawHistMaxPage
		   }
	   }
	  q := ledger.WithdrawHistQuery{
		  UserID: uid, Page: page, PageSize: pageSize,
		   Start: startAt, End: endAt,
		  Currency: strings.ToUpper(strings.TrimSpace(bffParam(c, "currency"))),
		  Status: strings.ToUpper(strings.TrimSpace(bffParam(c, "status"))),
	  }
	   q.SetInnerFilter(bffParam(c, "isInner"))
	  cols := []string{"id", "currency", "chain", "amount", "status", "isInner", "fee", "receiveAmount", "memo", "tagName", "createdAt"}
	  buildWD := func(tr *ledger.Transfer) gin.H {
		   return withdrawListItemJSON(a, tr)
	   }
	  if csv {
		   // 大窗 CSV 翻页流式写出，超越单次 MaxPage 截断。
		  streamHistCSV(c, cols, func(pg, psz int) ([]gin.H, int, error) {
			   qq := q
			   qq.Page, qq.PageSize = pg, psz
			  list,
				  total, err := a.rt.Ledger.ListWithdraws(c.Request.Context(), qq)
			  if err != nil {
				  return nil, 0, err
			  }
			   items := make([]gin.H, 0, len(list))
			  for _, tr := range list {
				  if tr == nil {
					  continue
				  }
				  items = append(items, buildWD(tr))
			   }
			   return items, total, nil
		   })
		  return
	   }
	  // status/isInner 在 SQL 层过滤，页内不再事后丢弃导致 totalNum 漂移。
	  list, total, err := a.rt.Ledger.ListWithdraws(c.Request.Context(), q)
	  if err != nil {
		  fail(c, 400, err.Error())
		  return
	   }
	   items := make([]gin.H, 0, len(list))
	  for _, tr := range list {
		   if tr == nil {
			   continue
		  }
		  items = append(items, buildWD(tr))
	  }
	   if q.Page <= 0 {
		  q.Page = 1
	  }
	   if q.PageSize <= 0 {
		  q.PageSize = ledger.WithdrawHistDefaultPage
	  }
	  totalPage := 1
	  if q.PageSize > 0 && total > 0 {
		  totalPage = (total + q.PageSize - 1) / q.PageSize
	  }
	  ok(c, gin.H{
		  "currentPage": q.Page, "pageSize": q.PageSize, "totalNum": total, "total": total, "totalPage": totalPage,
		  "items": items, "retentionDays": int(ledger.WithdrawHistRetention / (24 * time.Hour)),
	   })
  }

   func (a *API) getWithdrawByID(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		   return
	  }
	  tr, err := a.rt.Ledger.Get(c.Request.Context(), c.Param("withdrawalId"))
	  if err != nil || tr == nil || tr.UserID != uid {
		   fail(c, 404, "withdrawal not found")
		  return
	   }
	  ok(c, withdrawByIDView(a, tr))
   }

// cancelWithdrawBody 一致 UTA 取消出金：body 携带 withdrawalId（兼容别名）。

   func (a *API) cancelWithdrawBody(c *gin.Context) {
	  var body struct {
		   WithdrawalID string `json:"withdrawalId"`
		   WithdrawID   string `json:"withdrawId"`
	   }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		  return
	  }
	   id := strings.TrimSpace(body.WithdrawalID)
	  if id == "" {
		  id = strings.TrimSpace(body.WithdrawID)
	  }
	  if id == "" {
		  fail(c, 400, "withdrawalId required")
		   return
	   }
	   c.Params = append(c.Params, gin.Param{Key: "withdrawalId", Value: id})
	  a.cancelWithdraw(c)
  }

// cancelWithdrawUTA docs-new POST /api/ua/v2/asset/withdraw/cancel（withdrawId）。
// 官方 path 与既有 withdrawal/cancel 别名并存；字段 withdrawId 先，兼收 withdrawalId。
// 已广播不可取消，落点仍走 withdraw.Cancel。
  func (a *API) cancelWithdrawUTA(c *gin.Context) {
	  var body struct {
		   WithdrawID string `json:"withdrawId"`
		  WithdrawalID string `json:"withdrawalId"`
	  }
	   if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		  return
	   }
	   id := strings.TrimSpace(body.WithdrawID)
	   if id == "" {
		  id = strings.TrimSpace(body.WithdrawalID)
	  }
	  if id == "" {
		  fail(c, 400, "withdrawId required")
		   return
	  }
	  c.Params = append(c.Params, gin.Param{Key: "withdrawalId", Value: id})
	   a.cancelWithdraw(c)
  }

   func withdrawByIDView(a *API, tr *ledger.Transfer) gin.H {
	   fee := a.withdrawFeeAmt(tr.Amount)
	  if tr.Direction == ledger.SpotSend {
		   fee = decimal.Zero
	   }
	  st := railStatus(tr)
	  cancelType := "NON_CANCELABLE"
	  if st == "PENDING" || st == "PROCESSING" {
		  cancelType = "CANCELABLE"
	  }
	   chain := strings.TrimSpace(tr.Chain)
	  tx := tr.TxHash
	  var txAny any = tx
	  if strings.TrimSpace(tx) == "" {
		  txAny = nil
	  }
	   retStatus, retAmt, retCur := withdrawReturnFields(tr)
	  memo := strings.TrimSpace(tr.MemoHint)
	   if memo == "" {
		  memo = strings.TrimSpace(tr.TagHint)
	  }
	   tagName := strings.TrimSpace(tr.TagNameHint)
	  if tagName == "" {
		  tagName = fundsurf.MemoTagName(chain)
	  }
	   out := gin.H{
		  "id": tr.ID, "uid": tr.UserID, "currency": tr.Asset, "currencyName": tr.Asset,
		   "chainId": strings.ToLower(chain), "chainName": chain, "chain": chain,
		  "status": st, "failureReason": tr.FailReason, "failureReasonMsg": nil,
		   "address": tr.ToAddressHint, "memo": memo, "tagName": tagName,
		  "needMemo": fundsurf.NeedsMemo(chain), "isInner": tr.Direction == ledger.SpotSend,
		   "amount": tr.Amount.String(), "fee": fee.String(), "walletTxId": txAny,
		  "addressRemark": "", "remark": "", "createdAt": tr.CreatedAt.UnixMilli(),
		   "cancelType": cancelType, "taxes": nil, "taxDescription": nil, "txId": txAny,
		  "returnStatus": retStatus, "returnAmount": retAmt, "returnCurrency": retCur,
	   }
	  if tagName == "destination_tag" && memo != "" {
		   out["destinationTag"] = memo
	   }
	   if tagName == "comment" && memo != "" {
		   out["comment"] = memo
	  }
	   // By-ID 透出剩余确认数与手续费抬价面（一致公开状态机 + 托管广播跟踪，非地址簿臆造）。
	   // 有广播跟踪用实时确认；否则按链确认深度与终态推算；feeBump 仅在已记录抬价时出现。
	   // 不得捏造未文档化的仅地址簿字段；无跟踪时 remaining 可为 0。
	  conf, need, remain := withdrawConfirmSurface(a, tr)
	  out["confirmations"] = conf
	   out["requiredConfirmations"] = need
	   out["remainingConfirmations"] = remain
	  if bump,
		  ok := withdrawFeeBumpSurface(a, tr); ok {
		  out["feeBump"] = bump
	  }
	   applyWithdrawDetailJSON(out, tr)
	  return out
   }

// withdrawConfirmSurface 出金 By-ID 确认数/剩余确认。

  func withdrawConfirmSurface(a *API, tr *ledger.Transfer) (conf, need, remain int) {
	  if tr == nil {
		  return 0, 0, 0
	  }
	   if tr.Direction == ledger.SpotSend {
		  return 0, 0, 0
	  }
	   need = 0
	  if a != nil {
		  need = a.confirmNeed(tr.Chain)
	  }
	   if svc := a.withdrawSvc(); svc != nil {
		  if bc, ok := svc.Broadcasts().Get(tr.ID); ok {
			  conf = bc.Confirmations
			   if bc.Need > 0 {
				   need = bc.Need
			  }
			  remain = need - conf
			   if remain < 0 {
				   remain = 0
			  }
			   if bc.Status == withdraw.BcConfirmed || stDone(tr) {
				   remain = 0
				   if conf < need {
					  conf = need
				  }
			   }
			  return conf, need, remain
		  }
	  }
	   if stDone(tr) {
		   return need, need, 0
	   }
	  if tr.State == ledger.StateChainSubmitted || tr.State == ledger.StateConfirmed {
		   return 0, need, need
	  }
	  return 0, need, need
  }

   func stDone(tr *ledger.Transfer) bool {
	   if tr == nil {
		  return false
	   }
	   switch tr.State {
	  case ledger.StateCredited, ledger.StateConfirmed, ledger.StateFailed, ledger.StateRolledBack:
		   return true
	  default:
		   return railStatus(tr) == "SUCCESS" || railStatus(tr) == "FAILURE"
	  }
  }

// withdrawFeeBumpSurface 已登记手续费抬价/RBF 回显（公开 walletTxId 变更邻接）。

   func withdrawFeeBumpSurface(a *API, tr *ledger.Transfer) (gin.H, bool) {
	   if a == nil || tr == nil {
		  return nil, false
	  }
	  svc := a.withdrawSvc()
	   if svc == nil {
		  return nil, false
	  }
	  row, ok := svc.Bumper().Get(tr.ID)
	   if !ok {
		  return nil, false
	   }
	   out := gin.H{
		   "oldHash": row.OldHash, "newHash": row.NewHash,
		  "oldFee": row.OldFee, "newFee": row.NewFee,
		  "replaced": row.Replaced, "at": row.At.UnixMilli(),
	   }
	  // 抬价独立编解码面回显 destination_tag（与主广播同形）。
	   if row.DestinationTag != "" {
		  out["destinationTag"] = row.DestinationTag
	   }
	   if row.Memo != "" {
		  out["memo"] = row.Memo
	   }
	   if row.TagName != "" {
		   out["tagName"] = row.TagName
	   }
	   return out, true
  }

// withdrawReturnFields 按出金终态映射公开 returnStatus/returnAmount。
// 取消或回滚成功后资金已回到可用，须标 RETURN 与原币金额；进行中/成功出金为 NOT_RETURN。
// 不得写死 KCS；已广播失败未回账不得谎报 RETURN。
   func withdrawReturnFields(tr *ledger.Transfer) (string, any, string) {
	   if tr == nil {
		   return "NOT_RETURN", nil, ""
	  }
	   cur := strings.ToUpper(strings.TrimSpace(tr.Asset))
	   switch tr.State {
	  case ledger.StateRolledBack:
		  return "RETURN", tr.Amount.String(), cur
	   case ledger.StateFailed:
		   reason := strings.ToLower(tr.FailReason)
		   if strings.Contains(reason, "cancel") || strings.Contains(reason, "user_cancel") || strings.Contains(reason, "rollback") {
			  return "RETURN", tr.Amount.String(), cur
		   }
		   return "NOT_RETURN", nil, cur
	  default:
		   return "NOT_RETURN", nil, cur
	   }
  }

  func (a *API) previewWithdraw(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	   if !okU {
		   return
	   }
	  cur := strings.ToUpper(strings.TrimSpace(bffParam(c, "currency")))
	  chain := strings.ToUpper(strings.TrimSpace(bffParam(c, "chain")))
	   inner := strings.EqualFold(bffParam(c, "isInner"), "true")
	   amtRaw := bffParam(c, "amount")
	   if amtRaw == "" {
		   amtRaw = "0"
	   }
	   amt, err := ledger.ParseAmount(amtRaw)
	   if err != nil {
		  fail(c, 400, err.Error())
		  return
	  }
	  fee := decimal.Zero
	  if !inner {
		  fee = a.withdrawFeeAmt(amt)
	  }
	  recv := amt.Sub(fee)
	   if !recv.IsPositive() {
		   recv = amt
	  }
	   // 预览附带 needMemo/tagName/format，一致出金页备注 UX。
	   tagName := fundsurf.MemoTagName(chain)
	   ok(c, a.withdrawPreviewJSON(c.Request.Context(), uid, cur, chain, a.withdrawable(c, uid, cur), inner, amt, fee, recv, tagName, wallet.FormatSpec(chain)))
   }

   func railStatus(tr *ledger.Transfer) string {
	  if tr == nil {
		  return "FAILURE"
	   }
	   if st, ok := xferloop.Rail(tr); ok && st != "" {
		   return st
	   }
	   switch tr.State {
	  case ledger.StateCredited:
		   return "SUCCESS"
	   case ledger.StateChainSubmitted, ledger.StateConfirmed:
		   return "PROCESSING"
	   case ledger.StateFailed, ledger.StateRolledBack:
		  return "FAILURE"
	  default:
		  return "PENDING"
	  }
   }

  func (a *API) confirmFloor() int {
	   if a != nil && a.rt != nil && a.rt.Cfg != nil && a.rt.Cfg.Transfer.ConfirmDepth > 0 {
		   return a.rt.Cfg.Transfer.ConfirmDepth
	   }
	   return 1
  }

  func (a *API) confirmNeed(chain string) int {
	   return ledger.ConfirmNeed(chain, a.confirmFloor())
  }

  func (a *API) withdrawFeeAmt(amt decimal.Decimal) decimal.Decimal {
	   if a == nil || a.rt == nil || a.rt.Cfg == nil || a.rt.Cfg.Transfer.WithdrawFeeBPS <= 0 {
		   return decimal.Zero
	  }
	  fee := amt.Mul(decimal.NewFromInt(int64(a.rt.Cfg.Transfer.WithdrawFeeBPS))).Div(decimal.NewFromInt(10000)).Truncate(18)
	   if fee.GreaterThanOrEqual(amt) {
		   return decimal.Zero
	   }
	  return fee
  }

   func matchHist(c *gin.Context, tr *ledger.Transfer, row gin.H, wantCur, wantSt string, innerOnly, chainOnly bool) bool {
	   if tr == nil || !inHistoryWindow(c, tr.CreatedAt) {
		  return false
	  }
	  if wantCur != "" && strings.ToUpper(tr.Asset) != wantCur {
		   return false
	   }
	   st, _ := row["status"].(string)
	  if wantSt != "" && !strings.EqualFold(st, wantSt) {
		   return false
	   }
	   inner, _ := row["isInner"].(bool)
	  if innerOnly && !inner {
		   return false
	  }
	   if chainOnly && inner {
		  return false
	   }
	  return true
   }

// applyExportWindow 充提 CSV 导出窗口与支持中心 100 天一致（merged 历史同口径）。
// 缺省近 100 天；超窗拒绝；列表 JSON 仍走 365d 保留窗。
// 不得用保留窗绕过导出日窗；createdAt 已在行内。

  func applyExportWindow(start, end time.Time) (time.Time, time.Time, error) {
	   now := time.Now().UTC()
	  if start.IsZero() && end.IsZero() {
		   end = now
		   start = end.Add(-100 * 24 * time.Hour)
	   }
	  if err := fundsurf.CheckExportWindow(start, end); err != nil {
		  return time.Time{}, time.Time{}, err
	   }
	   if start.IsZero() {
		  start = now.Add(-100 * 24 * time.Hour)
	  }
	  if end.IsZero() {
		   end = now
	   }
	  if err := fundsurf.CheckExportWindow(start, end); err != nil {
		   return time.Time{}, time.Time{}, err
	   }
	  return start, end, nil
  }

  func writeHistCSV(c *gin.Context, items []gin.H, cols []string) {
	  var b strings.Builder
	   b.WriteString(strings.Join(cols, ","))
	   b.WriteByte('\n')
	  for _, row := range items {
		  for i, col := range cols {
			   if i > 0 {
				  b.WriteByte(',')
			   }
			   b.WriteString(strings.ReplaceAll(fmtVal(row[col]), ",", " "))
		  }
		  b.WriteByte('\n')
	   }
	  c.Header("Content-Type", "text/csv")
	  c.String(200, b.String())
   }

// streamHistCSV 按 MaxPage 翻页流式写出充提 CSV，覆盖 100 天大窗全量。
// 首行表头一次；页内用 DepositHistMaxPage；total 驱动停页；单页失败整单失败。
// 不得在无窗校验时绕过 applyExportWindow；内存只持一页。
  func streamHistCSV(c *gin.Context, cols []string, fetch func(page, pageSize int) ([]gin.H, int, error)) {
	   pageSize := ledger.DepositHistMaxPage
	  if pageSize <= 0 {
		   pageSize = 100
	  }
	   c.Header("Content-Type", "text/csv")
	  c.Status(200)
	  w := c.Writer
	   _, _ = io.WriteString(w, strings.Join(cols, ","))
	   _, _ = io.WriteString(w, "\n")
	  if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
	total := -1
	for page := 1; ; page++ {
		items, tot, err := fetch(page, pageSize)
		if err != nil {
			_, _ = io.WriteString(w, "# error: "+strings.ReplaceAll(err.Error(), "\n", " ")+"\n")
			return
		}
		if total < 0 {
			total = tot
		}
		for _, row := range items {
			for i, col := range cols {
				if i > 0 {
					_, _ = io.WriteString(w, ",")
				}
				_, _ = io.WriteString(w, strings.ReplaceAll(fmtVal(row[col]), ",", " "))
			}
			_, _ = io.WriteString(w, "\n")
		}
		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}
		if len(items) == 0 {
			return
		}
		if total >= 0 && page*pageSize >= total {
			return
		}
		if len(items) < pageSize {
			return
		}
		// 防护：异常超大窗硬顶 500 页
		if page >= 500 {
			return
		}
	}
}

   func fmtVal(v any) string {
	   switch t := v.(type) {
	  case string:
		   return t
	  case bool:
		   if t {
			   return "true"
		   }
		  return "false"
	   case int:
		  return decimal.NewFromInt(int64(t)).String()
	  case int64:
		   return decimal.NewFromInt(t).String()
	   case int32:
		   return decimal.NewFromInt(int64(t)).String()
	  case float64:
		  return decimal.NewFromFloat(t).String()
	   case nil:
		  return ""
	  default:
		  return fmt.Sprint(t)
	  }
  }

   func inHistoryWindow(c *gin.Context, at time.Time) bool {
	   start := parseUnixOrRFC(bffParam(c, "startAt"))
	   end := parseUnixOrRFC(bffParam(c, "endAt"))
	   if err := fundsurf.CheckExportWindow(start, end); err != nil {
		   return false
	   }
	   if !start.IsZero() && at.Before(start) {
		  return false
	  }
	  if !end.IsZero() && at.After(end) {
		  return false
	   }
	  return true
   }

  func parseUnixOrRFC(s string) time.Time {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  return time.Time{}
	  }
	  if t, err := time.Parse(time.RFC3339, s); err == nil {
		  return t
	   }
	   if n, err := decimal.NewFromString(s); err == nil && n.GreaterThan(decimal.Zero) {
		   return time.Unix(n.IntPart(), 0).UTC()
	  }
	  return time.Time{}
   }
