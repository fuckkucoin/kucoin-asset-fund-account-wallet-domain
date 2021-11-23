package snap

import (
	"strconv"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New(rt.Ledger, rt.Store)
	  registerRoutes(v1, rt, svc)
   }

   func registerRoutes(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  v1.POST("/snapshots", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
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
		  uid, err = validate.UserID(body.UserID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   snap, err := svc.Capture(c.Request.Context(), uid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, snap)
	  })
	   v1.GET("/snapshots", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   uid,
			   err := validate.UserID(rawUID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   limit := validate.Limit(atoi(c.Query("limit")), 20, 200)
		   list,
			   err := svc.List(c.Request.Context(), uid, limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "snapshots": list, "host": "query-gateway.kucoin.net"})
	  })
	   v1.GET("/snapshots/latest", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   uid, err := validate.UserID(rawUID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  snap, err := svc.Latest(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, snap)
	   })
	  v1.GET("/snapshots/:id", func(c *gin.Context) {
		   snap, err := svc.Get(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, snap)
	  })
	  v1.GET("/snapshots/:id/check", func(c *gin.Context) {
		  if err := svc.CheckLive(c.Request.Context(), c.Param("id")); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"ok": true, "id": c.Param("id")})
	  })
	  v1.GET("/snapshots/diff", func(c *gin.Context) {
		   from := c.Query("from")
		  to := c.Query("to")
		  if from == "" || to == "" {
			   c.JSON(400, gin.H{"error": "from and to required"})
			  return
		   }
		  diff, err := svc.Diff(c.Request.Context(), from, to)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, diff)
	   })
  }

  func atoi(s string) int {
	   n, _ := strconv.Atoi(s)
	  return n
   }
