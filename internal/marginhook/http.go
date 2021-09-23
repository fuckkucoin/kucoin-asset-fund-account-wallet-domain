package marginhook

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   _ = rt
	  v1.POST("/margin/collateral", func(c *gin.Context) {
		   var body struct {
			   UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			  CapBPS int    `json:"cap_bps"`
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
		   h, err := svc.OpenCollateral(OpenCollateralReq{UserID: body.UserID, Asset: body.Asset, Amount: body.Amount, CapBPS: body.CapBPS})
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, h)
	  })
	   v1.POST("/margin/borrow", func(c *gin.Context) {
		   var body struct {
			  UserID       string `json:"user_id"`
			   CollateralID string `json:"collateral_id"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			   IdempotencyKey string `json:"idempotency_key"`
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
		   r, err := svc.OpenBorrow(BorrowReq{
			  UserID: body.UserID, CollateralID: body.CollateralID, Asset: body.Asset,
			   Amount:         body.Amount,
			  IdempotencyKey: body.IdempotencyKey,
		  })
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, r)
	  })
	   v1.GET("/margin/holds", func(c *gin.Context) {
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
		  c.JSON(200, gin.H{"holds": svc.Holds(uid), "host": "account-book.kucoin.net"})
	  })
	  v1.POST("/margin/scan", func(c *gin.Context) {
		  c.JSON(200, gin.H{"job": svc.Scan(), "host": "account-book.kucoin.net"})
	  })
	  v1.POST("/margin/freeze", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			   BorrowID string `json:"borrow_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  h, err := svc.FreezeForLTV(uid, body.BorrowID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, h)
	   })
	  v1.POST("/margin/release", func(c *gin.Context) {
		  var body struct {
			  HoldID string  `json:"hold_id"`
			  LiqGen *uint64 `json:"liq_gen"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   has := body.LiqGen != nil
		   var gen uint64
		   if has {
			  gen = *body.LiqGen
		   }
		  if err := svc.ReleaseWithGen(body.HoldID, gen, has); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"released": true, "host": "account-book.kucoin.net"})
	  })
	   v1.POST("/margin/close", func(c *gin.Context) {
		  var body struct {
			   BorrowID string  `json:"borrow_id"`
			   LiqGen   *uint64 `json:"liq_gen"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  has := body.LiqGen != nil
		   var gen uint64
		   if has {
			   gen = *body.LiqGen
		   }
		   if err := svc.CloseBorrowWithGen(body.BorrowID, gen, has); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"closed": true, "host": "account-book.kucoin.net"})
	  })
   }
