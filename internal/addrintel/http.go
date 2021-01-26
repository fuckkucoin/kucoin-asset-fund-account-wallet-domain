package addrintel

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		   svc.BindSQL(rt.SQL)
	  }
	   v1.POST("/intel/observe", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Chain string `json:"chain"`
			  Address string `json:"address"`
			   UserID  string `json:"user_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  body.UserID = uid
		   sc, err := svc.Observe(body.Chain, body.Address, body.UserID, time.Now().UTC())
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, sc)
	   })
	   v1.GET("/intel/score", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  sc, err := svc.Book().ScoreOf(c.Query("chain"), c.Query("address"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, sc)
	   })
	  v1.POST("/intel/tag", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Chain string `json:"chain"`
			  Address string `json:"address"`
			  Tag     string `json:"tag"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := svc.Tags().Tag(body.Chain, body.Address, body.Tag); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true, "host": "address.kucoin.net"})
	  })
	  v1.GET("/intel/cluster", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, svc.Tags().Cluster(c.Query("tag")))
	  })
	   v1.POST("/intel/screen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  Chain   string `json:"chain"`
			  Address string `json:"address"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.Screen(body.Chain, body.Address); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  kind, err := svc.Class().KindOf(body.Chain, body.Address)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true, "kind": kind})
	   })
	  v1.POST("/intel/batch", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  Rows []struct {
				  Chain   string `json:"chain"`
				  Address string `json:"address"`
			  } `json:"rows"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  in := make([]struct{ Chain, Address string }, 0, len(body.Rows))
		  for _, r := range body.Rows {
			  in = append(in, struct{ Chain, Address string }{r.Chain, r.Address})
		  }
		  c.JSON(200, gin.H{"rows": svc.ScreenBatch(in)})
	  })
	  v1.POST("/intel/sanctions", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Chain string `json:"chain"`
			   Address string `json:"address"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := svc.Sanctions().Add(body.Chain, body.Address); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"ok": true})
	  })
  }
