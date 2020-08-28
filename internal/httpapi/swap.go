// 兑换 HTTP。
// 作者: Likeo
// 日期: 2020.08.27
package httpapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/swap"
	"github.com/gin-gonic/gin"
)

// registerSwap 询价、成交、订单查询；成交记钱包余额。

func registerSwap(v1 *gin.RouterGroup, rt *app.Runtime) {
	// HTTP mapping only; freeze/submit/settle lives in swap.Service.
	v1.POST("/swap/quote", func(c *gin.Context) {
		var body struct {
			FromAsset string `json:"from_asset"`
			ToAsset   string `json:"to_asset"`
			Amount    string `json:"amount"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		q, err := rt.Swap.Quote(c.Request.Context(), body.FromAsset, body.ToAsset, body.Amount)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, q)
	})
	v1.POST("/swap/execute", func(c *gin.Context) {
			Chain          string `json:"chain"`
			QuoteRate      string `json:"quote_rate"`
			Wait           *bool  `json:"wait"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.IdempotencyKey == "" {
			c.JSON(400, gin.H{"error": "idempotency_key required"})
			return
		}
		if body.UserID == "" {
			body.UserID = rt.Cfg.SeedUserID
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		async := false
		if body.Wait != nil {
			async = !*body.Wait
		}
		ord, err := rt.Swap.Execute(ctx, swap.ExecReq{
			UserID:    body.UserID,
			FromAsset: body.FromAsset, ToAsset: body.ToAsset,
			FromAmount: body.Amount, IdempotencyKey: body.IdempotencyKey, FillRatio: body.FillRatio, Chain: body.Chain,
			QuoteRate: body.QuoteRate, Async: async,
		})
		var view any
		if ord != nil {
			view = swap.ViewOrder(ord)
		}
		writeEntity(c, "order", view, err)
	})
	v1.GET("/swap/orders", func(c *gin.Context) {
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		list, err := rt.Swap.ListOrders(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		status := c.Query("status")
		out := make([]swap.OrderView, 0, len(list))
		for _, o := range list {
			if !swap.MatchOrder(o, status) {
				continue
			}
			out = append(out, swap.ViewOrder(o))
		}
		c.JSON(200, gin.H{"orders": out})
	})
	v1.GET("/swap/orders/:id", func(c *gin.Context) {
		o, err := rt.Swap.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, swap.ViewOrder(o))
	})
}
