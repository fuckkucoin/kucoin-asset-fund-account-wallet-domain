// 现货下单 HTTP。
// 作者: Span
// 日期: 2020.08.27
package httpapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/spot"
	"github.com/gin-gonic/gin"
)

func registerSpot(v1 *gin.RouterGroup, rt *app.Runtime) {
	// 现货 HTTP：下单、撤单、深度、成交。
	v1.GET("/spot/symbols", func(c *gin.Context) {
		if rt.Spot == nil {
			c.JSON(503, gin.H{"error": "spot not wired"})
			return
		}
		c.JSON(200, gin.H{"symbols": rt.Spot.Symbols(), "matching": "clob"})
	})
	v1.GET("/spot/depth", func(c *gin.Context) {
		if rt.Spot == nil {
			c.JSON(503, gin.H{"error": "spot not wired"})
			return
		}
		sym := c.DefaultQuery("symbol", "ETH-USDT")
		c.JSON(200, rt.Spot.Depth(sym))
	})
	v1.GET("/spot/trades", func(c *gin.Context) {
		if rt.Spot == nil {
			c.JSON(503, gin.H{"error": "spot not wired"})
			return
		}
		c.JSON(200, gin.H{"trades": rt.Spot.Trades(queryLimit(c, 20, 100))})
	})
			Symbol         string `json:"symbol"`
			Side           string `json:"side"`
			Type           string `json:"type"`
			Size           string `json:"size"`
			Price          string `json:"price"`
			IdempotencyKey string `json:"idempotency_key"`
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
		ord, err := rt.Spot.Place(ctx, spot.PlaceReq{
			UserID: body.UserID,
			Symbol: body.Symbol, Side: body.Side, Type: body.Type,
			Size: body.Size, Price: body.Price, IdempotencyKey: body.IdempotencyKey,
		})
		writeEntity(c, "order", spot.ViewOrder(ord), err)
	})
	v1.GET("/spot/orders", func(c *gin.Context) {
		if rt.Spot == nil {
			c.JSON(503, gin.H{"error": "spot not wired"})
			return
		}
		user := c.DefaultQuery("user_id", rt.Cfg.SeedUserID)
		list, err := rt.Spot.List(c.Request.Context(), user)
		if err != nil {
			writeErr(c, err)
			return
		}
		out := make([]spot.OrderView, 0, len(list))
		for _, o := range list {
			out = append(out, spot.ViewOrder(o))
		}
		c.JSON(200, gin.H{"orders": out})
	})
	v1.GET("/spot/orders/:id", func(c *gin.Context) {
		if rt.Spot == nil {
			c.JSON(503, gin.H{"error": "spot not wired"})
			return
		}
		ord, err := rt.Spot.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, gin.H{"order": spot.ViewOrder(ord)})
	})
	v1.DELETE("/spot/orders/:id", func(c *gin.Context) {
		if rt.Spot == nil {
			c.JSON(503, gin.H{"error": "spot not wired"})
			return
		}
		ctx, cancel := writeTimeout(c, rt)
		defer cancel()
		ord, err := rt.Spot.Cancel(ctx, c.Param("id"))
		writeEntity(c, "order", spot.ViewOrder(ord), err)
	})
}
