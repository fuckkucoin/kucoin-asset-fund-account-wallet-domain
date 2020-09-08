// 行情 HTTP 和 websocket。
// 作者: Likeo
// 日期: 2020.08.25
package httpapi

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// registerMarket ticker / K 线 / 推送滞后，与现货同源。

func registerMarket(v1 *gin.RouterGroup, rt *app.Runtime) {
	v1.GET("/market/ticker", func(c *gin.Context) {
		c.JSON(200, gin.H{"tickers": rt.Market.Cache()})
	})
	v1.GET("/market/klines", func(c *gin.Context) {
		sym := c.DefaultQuery("symbol", "ETH-USDT")
		c.JSON(200, gin.H{"symbol": sym, "interval": "1m", "klines": rt.Market.Klines(sym)})
	})
	v1.GET("/market/lag", func(c *gin.Context) {
		c.JSON(200, rt.Market.Lag())
	})
	v1.GET("/market/ws", func(c *gin.Context) {
		rt.Market.ServeWS(c.Writer, c.Request)
	})
}
