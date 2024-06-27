package kcfund

import (
	       "strings"

	      "github.com/gin-gonic/gin"
)

// currencies G-07：币种网络列表，复用 fundsurf.Networks。
func (a *API) currencies(g *gin.RouterGroup) {
	g.GET("/v1/currencies/networks", a.currencyNetworks)
}

   func (a *API) currencyNetworks(c *gin.Context) {
	cur := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	       if cur == "" {
		fail(c, 400, "currency required")
	      	return
	}
	raw := a.surf().Networks(cur)
	nets := make([]gin.H, 0, len(raw))
	for _, n := range raw {
	   	nets = append(nets, networkJSON(n))
	   }
	   ok(c, gin.H{"currency": cur, "networks": nets})
}
