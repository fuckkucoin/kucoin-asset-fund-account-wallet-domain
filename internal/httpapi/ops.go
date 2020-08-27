package httpapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
	"strings"
)

// registerOps 注册运营辅助路由：KYC、热冷调拨、KMS 探活、入金注入。
func registerOps(v1 *gin.RouterGroup, rt *app.Runtime) {
	v1.GET("/futures/contracts", func(c *gin.Context) {
		c.JSON(200, gin.H{"contracts": []any{}})
	})
	v1.GET("/ops/kms", func(c *gin.Context) {
		if opsfc.Reject(c, perm.ActOpsAdmin) {
			return
		}
		kind := "none"
		if rt.Enc != nil {
			kind = rt.Enc.Kind()
		}
		c.JSON(200, gin.H{"keeper": kind})
	})
	v1.GET("/ops/users/kyc", func(c *gin.Context) {
		if opsfc.Reject(c, perm.ActOpsAdmin) {
			return
		}
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		level,
			err := rt.Ledger.KYC(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"user_id": user, "kyc_level": level})
	})
	v1.POST("/ops/users/kyc", func(c *gin.Context) {
		if !requireAct(c, perm.ActOpsAdmin) {
			return
		}
		var body struct {
			UserID string `json:"user_id"`
			Level  string `json:"kyc_level"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.UserID == "" {
			body.UserID = rt.Cfg.SeedUserID
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		if err := rt.Ledger.SetKYC(ctx, body.UserID, body.Level); err != nil {
			writeErr(c, err)
			return
		}
		got, _ := rt.Ledger.KYC(ctx, body.UserID)
		c.JSON(200, gin.H{"user_id": body.UserID, "kyc_level": got})
	})
	v1.POST("/ops/sweep", func(c *gin.Context) {
		if !requireAct(c, perm.ActCustodySweep) {
			return
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		rt.Ledger.SweepHot(ctx, rt.Cfg.Transfer.HotMax)
		c.JSON(200, gin.H{"ok": true})
	})
	v1.POST("/ops/inbound", func(c *gin.Context) {
		if !requireAct(c, perm.ActDepositObserve) {
			return
		}
		var body struct {
			UserID string `json:"user_id"`
			Chain  string `json:"chain"`
			Asset  string `json:"asset"`
			Amount string `json:"amount"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.UserID == "" {
			body.UserID = rt.Cfg.SeedUserID
		}
		chain := strings.ToUpper(strings.TrimSpace(body.Chain))
		if chain == "" {
			chain = "ETH"
		}
		amt, err := ledger.ParseAmount(body.Amount)
		if err != nil {
			writeErr(c, err)
			return
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		to := rt.Wallet.AddressFor(ctx, body.UserID, chain)
		hash, err := rt.Wallet.InjectInbound(chain, to, body.Asset, amt)
		if err != nil {
			writeErr(c, err)
			return
		}
		rt.ScanDeposits(ctx)
		c.JSON(200, gin.H{
			"hash": hash, "address": to, "chain": chain, "user_id": body.UserID,
		})
	})
}
