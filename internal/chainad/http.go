package chainad

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   reg := NewRegistry()
	   _ = rt
	  v1.GET("/chain/families", func(c *gin.Context) {
		   c.JSON(200, gin.H{"families": reg.Families(), "host": "deposit-withdraw.kucoin.net"})
	   })
	   v1.POST("/chain/observe", func(c *gin.Context) {
		   var body struct {
			  Chain  string `json:"chain"`
			  Hash   string `json:"hash"`
			   To     string `json:"to"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  in, err := reg.Observe(c.Request.Context(), body.Chain, body.Hash, body.To, body.Amount)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, in)
	  })
	   v1.POST("/chain/submit", func(c *gin.Context) {
		  var body Outbound
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   rec, err := reg.Submit(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, rec)
	  })
	  v1.POST("/chain/seed", func(c *gin.Context) {
		   var body struct {
			  Chain string `json:"chain"`
			  Hash  string `json:"hash"`
			   Asset  string `json:"asset"`
			   Amount string `json:"amount"`
			   To     string `json:"to"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if _, err := validate.Chain(body.Chain); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  in,
			   err := reg.SeedInbound(body.Chain, body.Hash, body.Asset, body.Amount, body.To)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, in)
	   })
	  tokens := NewTokenBook()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		  tokens.BindSQL(rt.SQL)
	  }
	  pols := NewPolicies()
	  v1.GET("/chain/memo", func(c *gin.Context) {
		   c.JSON(200, gin.H{"memo": MemoSpecs()})
	   })
	   v1.POST("/chain/memo/check", func(c *gin.Context) {
		   var body struct {
			   Chain string `json:"chain"`
			   Memo string `json:"memo"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if err := CheckMemo(body.Chain, body.Memo); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true})
	   })
	   v1.GET("/chain/tokens", func(c *gin.Context) {
		  c.JSON(200, gin.H{"tokens": tokens.List()})
	   })
	   v1.POST("/chain/tokens", func(c *gin.Context) {
		   var body Token
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if err := tokens.Put(body); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, body)
	   })
	  v1.GET("/chain/policies", func(c *gin.Context) {
		   c.JSON(200, gin.H{"policies": pols.List()})
	   })
	  v1.GET("/chain/inspect", func(c *gin.Context) {
		   chain := c.DefaultQuery("chain", "ETH")
		   hash := c.Query("hash")
		   obs, err := reg.Inspect(c.Request.Context(), chain, hash)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, obs)
	  })
  }
