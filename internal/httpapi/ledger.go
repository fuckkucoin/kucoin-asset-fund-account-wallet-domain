// 账本 HTTP：划转、充提和对账。
// 作者: Likeo
// 日期: 2020.08.25
package httpapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/swap"
	"account-assets.kucoin.net/funding/internal/trace"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/gin-gonic/gin"
)

// registerLedger 现货↔钱包划转、充提确认、对账与账单。
func registerLedger(v1 *gin.RouterGroup, rt *app.Runtime) {
	// 出入金/划转 HTTP 只做校验和映射；冻账、广播、入账都在 ledger.Service。
	v1.POST("/ledger/transfers", func(c *gin.Context) {
		if !requireAct(c, perm.ActAccountMove) {
			return
		}
		var body struct {
			UserID         string `json:"user_id"`
			Direction      string `json:"direction"`
			Asset          string `json:"asset"`
			Amount         string `json:"amount"`
			Chain          string `json:"chain"`
			ToAddress      string `json:"to_address"`
			TxHash         string `json:"tx_hash"`
			IdempotencyKey string `json:"idempotency_key"`
			Wait           bool   `json:"wait"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.UserID == "" {
			body.UserID = rt.Cfg.SeedUserID
		}
		if body.IdempotencyKey == "" {
			c.JSON(400, gin.H{"error": "idempotency_key required"})
			return
		}
		amt, err := ledger.ParseAmount(body.Amount)
		if err != nil {
			writeErr(c, err)
			return
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
			UserID: body.UserID, Direction: ledger.Direction(body.Direction),
			Asset: body.Asset, Amount: amt, Chain: body.Chain, ToAddress: body.ToAddress,
			TxHash: body.TxHash, IdempotencyKey: body.IdempotencyKey, WaitConfirm: body.Wait,
		})
		writeEntity(c, "transfer", publicTransfer(c.Request.Context(), rt, tr), err)
	})
	v1.GET("/ledger/transfers", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		list, err := rt.Ledger.List(c.Request.Context(), user, queryLimit(c, 20, 100))
		if err != nil {
			writeErr(c, err)
			return
		}
		status, direction, state := c.Query("status"), c.Query("direction"), c.Query("state")
		out := make([]ledger.TransferView, 0, len(list))
		for _, tr := range list {
			if !ledger.MatchTransfer(tr, status, direction, state) {
				continue
			}
			out = append(out, ledger.ViewTransfer(tr, trace.ID(c.Request.Context())))
		}
		c.JSON(200, gin.H{"transfers": out})
	})
	v1.GET("/ledger/transfers/:id", func(c *gin.Context) {
		tr, err := rt.Ledger.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"transfer": publicTransfer(c.Request.Context(), rt, tr)})
	})
	v1.POST("/ledger/transfers/:id/confirm", func(c *gin.Context) {
		if !requireAct(c, perm.ActLedgerWrite) {
			return
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		if err := rt.Ledger.ConfirmAndCredit(ctx, c.Param("id")); err != nil {
			tr, _ := rt.Ledger.Get(ctx, c.Param("id"))
			writeEntity(c, "transfer", publicTransfer(c.Request.Context(), rt, tr), err)
			return
		}
		tr, _ := rt.Ledger.Get(ctx, c.Param("id"))
		c.JSON(200, gin.H{"transfer": publicTransfer(c.Request.Context(), rt, tr)})
	})
	// 运营回滚；FeeDebit 已入账走 ReverseFee（冲正提前赎回罚金等），其余 FailAndRollback。
	// 复用既有 /ledger/transfers/:id/rollback 面，不新开公开口。
	// 非 FeeDebit 的 credited 单 FailAndRollback 幂等空操作，须走 Unwind/确认路径。
	v1.POST("/ledger/transfers/:id/rollback", func(c *gin.Context) {
		if !requireAct(c, perm.ActOpsAdmin) {
			return
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		id := c.Param("id")
		cur,
			err := rt.Ledger.Get(ctx, id)
		if err != nil {
			writeErr(c, err)
			return
		}
		if cur != nil && cur.Direction == ledger.FeeDebit {
			if err := rt.Ledger.ReverseFee(ctx, id); err != nil {
				writeErr(c, err)
				return
			}
		} else if err := rt.Ledger.FailAndRollback(ctx, id, "manual_rollback"); err != nil {
			writeErr(c, err)
			return
		}
		tr, _ := rt.Ledger.Get(ctx, id)
		c.JSON(200, gin.H{"transfer": publicTransfer(c.Request.Context(), rt, tr)})
	})
	v1.POST("/ledger/transfers/:id/cancel", func(c *gin.Context) {
		if !requireAct(c, perm.ActAccountMove) {
			return
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		if err := rt.Ledger.Cancel(ctx, c.Param("id"), "user_cancel"); err != nil {
			tr, _ := rt.Ledger.Get(ctx, c.Param("id"))
			writeEntity(c, "transfer", publicTransfer(c.Request.Context(), rt, tr), err)
			return
		}
		tr, _ := rt.Ledger.Get(ctx, c.Param("id"))
		c.JSON(200, gin.H{"transfer": publicTransfer(c.Request.Context(), rt, tr)})
	})
	v1.GET("/ledger/bills", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		list, err := rt.Ledger.ListBills(c.Request.Context(), user, queryLimit(c, 50, 200))
		if err != nil {
			writeErr(c, err)
			return
		}
		if list == nil {
			list = []ledger.Bill{}
		}
		c.JSON(200, gin.H{"user_id": user, "bills": list})
	})
	v1.POST("/ops/users/freeze", func(c *gin.Context) {
		if !requireAct(c, perm.ActAccountFreeze) {
			return
		}
		var body struct {
			UserID  string `json:"user_id"`
			Frozen  *bool  `json:"frozen"`
			Cascade bool   `json:"cascade"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.UserID == "" {
			body.UserID = rt.Cfg.SeedUserID
		}
		if body.Frozen == nil {
			c.JSON(400, gin.H{"error": "frozen required"})
			return
		}
		if rt.Durable.Recon != nil {
			if err := recon.GuardFreezeChange(recon.WrapBook(rt.Durable.Recon, 0), body.UserID); err != nil {
				writeErr(c, err)
				return
			}
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		if err := rt.Ledger.SetFrozenGate(ctx, body.UserID, *body.Frozen, body.Cascade && !*body.Frozen); err != nil {
			writeErr(c, err)
			return
		}
		got, _ := rt.Ledger.Frozen(ctx, body.UserID)
		c.JSON(200, gin.H{"user_id": body.UserID, "frozen": got})
	})
	v1.GET("/ledger/reconcile", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		extra, mm, err := rt.InflightFrozen(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		rep, err := ledger.ReconcileWith(c.Request.Context(), rt.Store, user, extra)
		if err != nil {
			writeErr(c, err)
			return
		}
		ledger.MergeMismatches(rep, mm)
		rt.Ledger.NoteReconcile(c.Request.Context(), rep)
		c.JSON(200, rep)
	})
	// 一张单看现货+钱包余额、在途划转/兑换、对账结果，用来核对两边是否对得上。
	v1.GET("/ledger/statement", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		rows, err := rt.Wallet.Balances(c.Request.Context(), user, "")
		if err != nil {
			writeErr(c, err)
			return
		}
		list, err := rt.Ledger.List(c.Request.Context(), user, queryLimit(c, 50, 200))
		if err != nil {
			writeErr(c, err)
			return
		}
		openTransfers := make([]ledger.TransferView, 0)
		for _, tr := range list {
			if tr.State.Open() {
				openTransfers = append(openTransfers, ledger.ViewTransfer(tr, trace.ID(c.Request.Context())))
			}
		}
		orders, err := rt.Swap.ListOrders(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		openSwaps := make([]swap.OrderView, 0)
		for _, o := range orders {
			if o != nil && o.Status.Open() {
				openSwaps = append(openSwaps, swap.ViewOrder(o))
			}
		}
		extra, mm, err := rt.InflightFrozen(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		rep, err := ledger.ReconcileWith(c.Request.Context(), rt.Store, user, extra)
		if err != nil {
			writeErr(c, err)
			return
		}
		ledger.MergeMismatches(rep, mm)
		rt.Ledger.NoteReconcile(c.Request.Context(), rep)
		books, err := rt.Ledger.Accounts(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{
			"user_id":        user,
			"books_match":    rep.OK,
			"balances":       wallet.UnifyBalances(rows),
			"accounts":       books,
			"fees_paid":      swap.FeesPaid(orders),
			"open_transfers": openTransfers,
			"open_swaps":     openSwaps,
			"reconcile":      rep,
			"view":           "funding+wallet+inflight",
		})
	})
	v1.GET("/ledger/audit", func(c *gin.Context) {
		entityType := c.Query("entity_type")
		entityID := c.Query("entity_id")
		if entityType == "" && entityID == "" {
			c.JSON(400, gin.H{"error": "entity_type or entity_id required"})
			return
		}
		list, err := rt.Ledger.ListAudit(c.Request.Context(), entityType, entityID, queryLimit(c, 50, 200))
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"events": list})
	})
	v1.GET("/ledger/accounts", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		books, err := rt.Ledger.Accounts(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		orders, err := rt.Swap.ListOrders(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{
			"user_id":   user,
			"accounts":  books,
			"fees_paid": swap.FeesPaid(orders),
		})
	})
}
