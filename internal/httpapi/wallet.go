// 钱包 HTTP：创建导入、余额流水、地址簿。
// 作者: Likeo
// 日期: 2020.08.27
package httpapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
	"fmt"
	"github.com/gin-gonic/gin"
	"strings"
)

// registerWallet 钱包 HTTP：创建/导入、多链余额流水、节点健康。

func registerWallet(v1 *gin.RouterGroup, rt *app.Runtime) {
	v1.GET("/wallet/balances", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		rows, err := rt.Wallet.Balances(c.Request.Context(), user, c.Query("chain"))
		if err != nil {
			writeErr(c, err)
			return
		}
		if c.Query("view") == "unified" {
			c.JSON(200, gin.H{"user_id": user, "view": "unified", "balances": wallet.UnifyBalances(rows)})
			return
		}
		c.JSON(200, gin.H{"user_id": user, "balances": rows})
	})
	v1.GET("/wallet/tx-history", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		items, err := rt.Wallet.TxHistory(c.Request.Context(), user, c.Query("chain"), queryLimit(c, 20, 50))
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"items": items})
	})
	v1.GET("/wallet/nodes/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"endpoints": rt.Wallet.NodeHealth(c.Query("chain"))})
	})
	v1.GET("/wallet/deposit-address", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		chain := strings.ToUpper(c.DefaultQuery("chain", "ETH"))
		addr := rt.Wallet.AddressFor(c.Request.Context(), user, chain)
		if addr == "" || wallet.ValidChainAddress(chain, addr) != nil {
			writeErr(c, fmt.Errorf("deposit address unavailable for %s", chain))
			return
		}
		c.JSON(200, gin.H{
			"user_id": user, "chain": chain, "address": addr, "memo": "",
			"note": "充值地址按用户密钥派生",
		})
	})
	v1.GET("/wallet/chains", func(c *gin.Context) {
		c.JSON(200, gin.H{"chains": rt.Wallet.Chains()})
	})
	v1.POST("/wallet/create", func(c *gin.Context) {
		var body struct {
			UserID string `json:"user_id"`
			Label  string `json:"label"`
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
		w, err := rt.Wallet.CreateWallet(ctx, body.UserID, body.Label)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, stripMnemonic(rt, w))
	})
	v1.POST("/wallet/import", func(c *gin.Context) {
		var body struct {
			UserID string `json:"user_id"`
			Label  string `json:"label"`
			Secret string `json:"mnemonic"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.UserID == "" {
			body.UserID = rt.Cfg.SeedUserID
		}
		ctx,
			cancel := writeTimeout(c, rt)
		defer cancel()
		w, err := rt.Wallet.ImportWallet(ctx, body.UserID, body.Label, body.Secret)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200,
			stripMnemonic(rt, w))
	})
	v1.GET("/wallet/list", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		list, err := rt.Wallet.ListWallets(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"wallets": list})
	})
	v1.GET("/wallet/address-book", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		list, err := rt.Ledger.ListAddressBook(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"user_id": user, "entries": list})
	})
	v1.POST("/wallet/address-book", func(c *gin.Context) {
		var body ledger.AddressBookEntry
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.UserID == "" {
			body.UserID = rt.Cfg.SeedUserID
		}
		ctx,
			cancel := writeTimeout(c, rt)
		defer cancel()
		got, err := rt.Ledger.AddAddressBook(ctx, body)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"entry": got})
	})
	v1.DELETE("/wallet/address-book", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		chain := c.Query("chain")
		addr := c.Query("address")
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		if err := rt.Ledger.RemoveAddressBook(ctx, user, chain, addr); err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"ok": true})
	})
}

func stripMnemonic(rt *app.Runtime, w *wallet.Wallet) *wallet.Wallet {
	if w == nil {
		return w
	}
	if rt == nil || rt.Cfg == nil || authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile) || !rt.Cfg.WalletRevealMnemonic {
		w.Mnemonic = ""
	}
	return w
}
