package invcheck

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/treasury"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil {
		  svc.BindStore(rt.Store)
		   svc.BindCustody(custody.New(rt.Store, rt.Ledger, nil, map[string]string{"USDT": "500000"}))
		  svc.BindTreasury(treasury.New(nil, nil))
	  }
	  v1.POST("/invcheck/run", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got,
			  err := svc.Run(c.Request.Context(), body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"report": got, "host": "custody-boundary-lab.kucoin.net"})
	   })
	   v1.GET("/invcheck/last", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   got, err := svc.Last()
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  v1.GET("/invcheck/list", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"reports": svc.List(50)})
	   })
	   v1.GET("/invcheck/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   got, err := svc.Get(c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
   }
