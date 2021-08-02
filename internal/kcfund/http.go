// @FileName: http.go
// @Summary: 公开 Account & Funding REST 兼容面。路径一致 docs.kucoin.com Classic + UTA；落点复用现有 AA 服务。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	"context"
	  "net/http"
	   "strings"
	"account-assets.kucoin.net/funding/internal/accounttransfers"
	  "account-assets.kucoin.net/funding/internal/convertflow"
	  "account-assets.kucoin.net/funding/internal/earnflow"
	   "account-assets.kucoin.net/funding/internal/fundflow"
	"account-assets.kucoin.net/funding/internal/subaccounts"
	  "account-assets.kucoin.net/funding/internal/app"
	  "account-assets.kucoin.net/funding/internal/callerid"
	   "account-assets.kucoin.net/funding/internal/destallow"
	  "account-assets.kucoin.net/funding/internal/validate"
	  "account-assets.kucoin.net/funding/internal/earn"
	  "account-assets.kucoin.net/funding/internal/fundingbff"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	   "account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/subacct"
	    "account-assets.kucoin.net/funding/internal/withdrawrails"
	   "github.com/gin-gonic/gin"
)

const CodeOK = "200000"

type API struct {
	rt       *app.Runtime
	earnSvc  *earn.Service
	   subSvc   *subacct.Service
	bff        *fundingbff.Client
	   wdNotify   *withdrawrails.Notifier
	xferNotify *accounttransfers.Notifier
	subNotify  *subaccounts.Notifier
	  ffNotify   *fundflow.Notifier
	    cvNotify   *convertflow.Notifier
	    earnNotify *earnflow.Notifier
}

  func Register(r *gin.Engine, rt *app.Runtime) {
	       if r == nil || rt == nil {
	    	  return
	  }
	     bff := fundingbff.NewClient(rt.Dial, rt.Cfg, rt.Mem)
	  a := &API{
	   	  rt: rt, bff: bff,
		  wdNotify:   withdrawrails.NewNotifier(bff, rt.Log),
		  xferNotify: accounttransfers.NewNotifier(bff, rt.Log),
	      	  subNotify:  subaccounts.NewNotifier(bff, rt.Log),
		  ffNotify:   fundflow.NewNotifier(bff, rt.Log),
	   	  cvNotify:   convertflow.NewNotifier(bff, rt.Log),
	     	  earnNotify: earnflow.NewNotifier(bff, rt.Log),
	       	  earnSvc:    bindEarnService(rt),
		  subSvc:  subacct.NewWithKV(rt.KV),
	  }
	    a.subSvc.BindBooks(rt.Ledger)
	     if !rt.Mem && rt.SQL != nil {
		   a.subSvc.BindSQL(rt.SQL)
	     }
	     BindFundsRecon(rt)
	     g := r.Group("/api")
	  g.Use(requireAuth(rt))
	      g.Use(a.bffMeshMiddleware)
	   a.accounts(g)
	   a.deposit(g)
	   a.withdraw(g)
	     a.transfer(g)
	  a.sub(g)
	      a.fee(g)
	       a.currencies(g)
	  a.convert(g)
	          a.earn(g)
	  a.registerTravel(g)
	   a.uta(g)
	    a.registerUTABorrowRead(g)
	          a.userSurface(g)
	   a.registerAccountConfig(g)
	  a.registerLedgerExport(g)
	   a.travelRule(g)
	   a.registerP2POccupancy(g)
	        a.fundOrders(g)
	      if rt != nil && rt.DestAllow != nil && rt.KV != nil {
	   	   destallow.BindRuntime(rt.DestAllow, rt.KV)
	       }
  }

   func requireAuth(rt *app.Runtime) gin.HandlerFunc {
	  return func(c *gin.Context) {
	    	  // 终端登录在外层网关完成。这里只收账户号，供后面的资金读写使用。
	   	  _ = rt
	       	  callerid.Prepare(c)
		  if uid := strings.TrimSpace(c.GetHeader("X-User-Id")); uid != "" {
	    		  if _, err := validate.UserID(uid); err != nil {
	    			  fail(c, 400, err.Error())
				  c.Abort()
				  return
			  }
	    	  }
	  	  c.Next()
	      }
  }

  func ok(c *gin.Context, data any) {
	     c.JSON(200, gin.H{"code": CodeOK, "data": data})
  }

   func okAcceptedOrder(c *gin.Context, orderID, clientOid string) {
	      if strings.TrimSpace(orderID) == "" {
	       	  fail(c, 400, "transfer not accepted")
		  return
	       }
	      ok(c, transferOrderJSON(orderID, clientOid, "SUCCESS"))
   }

   func fail(c *gin.Context, status int, msg string) {
	   if status <= 0 {
	      	  status = 400
	        }
	      code := "400000"
	   if status == 401 {
		  code = "400001"
	        }
	   if status == 404 {
		  code = "404000"
	  }
	    c.JSON(status, gin.H{"code": code, "msg": msg})
   }

  func failCode(c *gin.Context, status int, code, msg string) {
	     if status <= 0 {
		  status = 400
	      }
	       if strings.TrimSpace(code) == "" {
	       	   fail(c, status, msg)
	    	   return
	          }
	   c.JSON(status, gin.H{"code": code, "msg": msg})
   }

 func caller(c *gin.Context, rt *app.Runtime) string {
	     return callerid.Resolve(c, idOpt(rt))
   }

   func idOpt(rt *app.Runtime) callerid.Opt {
	   if rt == nil || rt.Cfg == nil {
	    	  return callerid.Opt{}
	      }
	  return callerid.FromRuntime(rt.Mem,
		   rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
  }

   func (a *API) user(c *gin.Context) string {
	   return caller(c,
		   a.rt)
  }

   func (a *API) note(ctx context.Context, key, kind, payload string) {
	  if a == nil || a.rt == nil || a.rt.KV == nil {
	   	   return
	     }
	    _ = notify.FromConfig(a.rt.KV, a.rt.Cfg).Publish(ctx, key, kind, payload)
	   a.emitWithdrawFromNote(ctx, kind, payload)
   }

  func (a *API) requireUser(c *gin.Context) (string, bool) {
	      uid := a.user(c)
	       if strings.TrimSpace(uid) == "" {
		  fail(c, 401, "caller user_id required")
	  	  return "", false
	  }
	      return uid, true
   }

   func (a *API) surf() *fundsurf.Book {
	         if a != nil && a.rt != nil && a.rt.FundSurf != nil {
		  return a.rt.FundSurf
	   }
	  if a == nil || a.rt == nil {
	  	   return fundsurf.New(nil, nil, nil)
	     }
	   return fundsurf.New(a.rt.KV, a.rt.Ledger, a.rt.Cfg).BindSecurity(a.rt.SecLock, a.rt.TradePW, a.rt.WdGate)
  }

   func (a *API) userSurface(g *gin.RouterGroup) {
	   g.GET("/v1/accounts/holds", a.frozenAssets)
	        g.GET("/v1/withdrawals/lock", a.withdrawLock)
	      g.POST("/v1/deposits/memo-recover", a.memoRecover)
	       g.POST("/v1/deposits/network-recover", a.networkRecover)
	      g.GET("/v1/deposits/network-recover/:id", a.getNetworkRecover)
	     g.GET("/v1/withdrawals/preview", a.previewWithdraw)
	    g.GET("/ua/v2/withdrawals/quotas", a.getWithdrawQuotas)
	      g.GET("/v1/accounts/transfers", a.listTransfers)
  }

   func (a *API) frozenAssets(c *gin.Context) {
	   if a.forwardMeshFundSurface(c, http.MethodGet, "/api/v1/accounts/holds", nil) {
	    	   return
	     }
	   uid, okU := a.requireUser(c)
	    if !okU {
		   return
	   }
	  rows,
		  err := a.surf().Frozen(c.Request.Context(), uid)
	   if err != nil {
	    	   fail(c, 400, err.Error())
	      	   return
	   }
	   ok(c, gin.H{"items": rows})
  }

  func (a *API) withdrawLock(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	     if !okU {
		   return
	   }
	      lock, err := a.surf().Lock(c.Request.Context(), uid)
	      if err != nil {
		  fail(c, 400, err.Error())
		   return
	     }
	       ok(c, withdrawLockJSON(lock))
   }

  func (a *API) memoRecover(c *gin.Context) {
	         var body struct {
	   	   Chain string `json:"chain"`
	    	  TxHash  string `json:"txHash"`
		  Memo    string `json:"memo"`
	  	  Address string `json:"address"`
	      }
	   if err := c.ShouldBindJSON(&body); err != nil {
	    	  fail(c, 400, err.Error())
	      	  return
	    }
	       uid, okU := a.requireUser(c)
	  if !okU {
	  	  return
	    }
	     t, err := a.surf().OpenMemoTicket(c.Request.Context(), uid, body.Chain, body.TxHash, body.Memo, body.Address)
	     if err != nil {
	       	  fail(c, 400, err.Error())
		   return
	     }
	      ok(c, t)
  }

  func (a *API) networkRecover(c *gin.Context) {
	  var body struct {
	  	   Chain         string `json:"chain"`
	    	  TxHash        string `json:"txHash"`
	  	  ExpectedChain string `json:"expectedChain"`
	    	   Address       string `json:"address"`
	  }
	       if err := c.ShouldBindJSON(&body); err != nil {
	     	   fail(c, 400, err.Error())
	   	   return
	     }
	       uid, okU := a.requireUser(c)
	     if !okU {
	   	  return
	    }
	       t,
		   err := a.surf().OpenNetworkTicket(c.Request.Context(), uid, body.Chain, body.TxHash, body.ExpectedChain, body.Address)
	       if err != nil {
		  fail(c, 400, err.Error())
	   	  return
	      }
	         ok(c, t)
   }

 func (a *API) getNetworkRecover(c *gin.Context) {
	   if _, okU := a.requireUser(c); !okU {
	    	  return
	      }
	     t, err := a.surf().GetNetworkTicket(c.Request.Context(), c.Param("id"))
	   if err != nil {
		  fail(c, 404, err.Error())
	   	   return
	       }
	        ok(c, t)
   }
