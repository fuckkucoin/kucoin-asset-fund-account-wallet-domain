package fundpath

import (
	"context"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/fundcircuit"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  if rt != nil {
		   svc.BindDeposit(deposit.New(rt.Ledger,
			   rt.Wallet))
		   svc.BindAccount(account.New(rt.Ledger))
		  svc.BindStore(rt.Store)
		   if rt.KV != nil {
			  svc.BindKV(rt.KV)
		  }
		  if desk := travelvasp.DeskFrom(rt); desk != nil {
			  svc.BindTravel(desk)
		  }
		  if rt.Cfg != nil {
			   svc.BindTravelCodes(rt.Cfg.Travel.OriginVASP,
				  rt.Cfg.Travel.PeerVASP)
			  book, _ := rt.TravelThresh.(*travelvasp.ThresholdBook)
			   if book == nil {
				  rows := make([]travelvasp.CorridorThreshold, 0, len(rt.Cfg.Travel.Corridors))
				   for _, c := range rt.Cfg.Travel.Corridors {
					   rows = append(rows, travelvasp.CorridorThreshold{Corridor: c.Corridor, Asset: c.Asset, MinAmount: c.MinAmount})
				   }
				  book = travelvasp.NewThresholdBook(rt.Cfg.Travel.DefaultThreshold, rows)
				  if len(rt.Cfg.Travel.UsdRates) > 0 {
					  book.BindUsdRates(rt.Cfg.Travel.UsdRates)
				  }
				   rt.TravelThresh = book
			  }
			   svc.BindTravelThresholds(book)
		   }
		   if rt.Wallet != nil {
			  svc.Pool().SetDerive(func(userID, chain string) string {
				  return rt.Wallet.AddressFor(context.Background(), userID, chain)
			   })
		   }
		  if !rt.Mem && rt.SQL != nil {
			   svc.Pool().BindSQL(rt.SQL)
			  if inv := svc.Inv(); inv != nil {
				   inv.BindSQL(rt.SQL)
			   }
			   if allow := svc.Allow(); allow != nil {
				   kind, profile := "", ""
				   if rt.Cfg != nil {
					  kind,
						  profile = rt.Cfg.Auth.Kind, rt.Cfg.Profile
				   }
				  destallow.BindProd(allow, rt.SQL, kind, profile)
			  }
		  }
		   // 资金域统一熔断总线：hop 入口只读闸，与出金 / 入金共用同一跨路径判定。
		   svc.BindFundCircuit(fundcircuit.Attach(rt).HopGate())
	   }
	   v1.POST("/fundpath/run", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body RunReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  body.UserID = uid
		   got, err := svc.Run(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/fundpath/alloc", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
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
		  got, err := svc.Allocate(body.UserID, body.Chain)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	  v1.POST("/fundpath/hop", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body HopReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  body.UserID = uid
		  id, err := svc.Hop(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"hop_id": id, "host": config.HostAccountBook})
	   })
	   v1.POST("/fundpath/egress", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body EgressReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid,
			  err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  body.UserID = uid
		   steps,
			  err := svc.Egress(c.Request.Context(), body)
		   if err != nil {
			   c.JSON(200, gin.H{"steps": steps, "allow": false, "error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"steps": steps, "allow": true})
	  })
	  v1.POST("/fundpath/halt", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  On bool `json:"on"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   svc.SetHalt(body.On)
		   c.JSON(200, gin.H{"halt": svc.Halted()})
	  })
	   v1.GET("/fundpath/recent", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{
			   "paths": svc.List(c.Query("user_id"), 50),
			  "host":  config.HostDepositWithdraw,
			  "region": persist.DualRegion(),
		   })
	  })
   }
