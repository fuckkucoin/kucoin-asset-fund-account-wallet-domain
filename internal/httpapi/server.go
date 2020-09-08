// HTTP 路由和中间件。
// 作者: Span
// 日期: 2020.08.26
package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/address"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/dustguard"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/balproof"
	"account-assets.kucoin.net/funding/internal/bookbill"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/depclaim"
	"account-assets.kucoin.net/funding/internal/depoccup"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/destlimit"
	"account-assets.kucoin.net/funding/internal/dustpath"
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
	"account-assets.kucoin.net/funding/internal/feebumpgate"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
	"account-assets.kucoin.net/funding/internal/earn"
	"account-assets.kucoin.net/funding/internal/exjob"
	"account-assets.kucoin.net/funding/internal/fiatadd"
	"account-assets.kucoin.net/funding/internal/fundpath"
	"account-assets.kucoin.net/funding/internal/fundread"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/gateway"
	"account-assets.kucoin.net/funding/internal/hotbound"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/lending"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/opsapi"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/p2pgate"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/por"
	"account-assets.kucoin.net/funding/internal/solvency"
	"account-assets.kucoin.net/funding/internal/stmthttp"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/sweeprun"
	"account-assets.kucoin.net/funding/internal/trace"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/wdcomp"
	"account-assets.kucoin.net/funding/internal/wdpath"
	"account-assets.kucoin.net/funding/internal/wdrelease"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"account-assets.kucoin.net/funding/internal/wrongcredit"
	"github.com/gin-gonic/gin"
)

  func loopbackOnly() gin.HandlerFunc {
	  return func(c *gin.Context) {
		   if !remoteIsLoopback(c.Request.RemoteAddr) {
			  c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "pprof is loopback only"})
			   return
		   }
		  c.Next()
	  }
  }

  func remoteIsLoopback(remote string) bool {
	  host, _, err := net.SplitHostPort(remote)
	  if err != nil {
		   host = remote
	  }
	   ip := net.ParseIP(host)
	  return ip != nil && ip.IsLoopback()
   }

// Router 只提供 HTTP。状态码由 apierr 映射，handler 不得按业务文案拼 status。
   func Router(rt *app.Runtime) *gin.Engine {
	  gin.SetMode(gin.ReleaseMode)
	  r := gin.New()
	  r.Use(gin.Recovery())
	   r.Use(mw.RequireHost(mw.HostSuffix))
	   r.Use(trace.HTTP(rt.Log))
	  r.Use(accessLog(rt))
	   r.Use(opsfc.MiddlewareHeaders())
	   registerCatalog(r, rt)
	   if rt.Cfg.Pprof {
		  dbg := r.Group("/debug/pprof")
		  dbg.Use(loopbackOnly())
		   dbg.GET("/", gin.WrapF(pprof.Index))
		  dbg.GET("/cmdline", gin.WrapF(pprof.Cmdline))
		  dbg.GET("/profile", gin.WrapF(pprof.Profile))
		   dbg.GET("/symbol", gin.WrapF(pprof.Symbol))
		   dbg.GET("/trace", gin.WrapF(pprof.Trace))
		  dbg.GET("/heap", gin.WrapH(pprof.Handler("heap")))
		   dbg.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		   dbg.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
		   dbg.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
		  dbg.GET("/block", gin.WrapH(pprof.Handler("block")))
	   }
	   v1 := r.Group("/v1")
	  v1.Use(requireAuth(rt))
	  v1.Use(opsfc.MiddlewareHeaders())
	  registerWallet(v1, rt)
	  registerLedger(v1, rt)
	  registerSwap(v1, rt)
	   registerSpot(v1, rt)
	  registerMarket(v1, rt)
	  registerOps(v1, rt)
	  registerDelegateCatalog(v1, rt)
	  account.Register(v1, rt)
	  gateway.RegisterFunding(v1, rt)
	   deposit.Register(v1, rt)
	   depoccup.Register(v1, rt)
	   withdraw.Register(v1, rt)
	  address.Register(v1, rt)
	  custody.Register(v1, rt)
	   kytsvc.Register(v1, rt)
	  travelvasp.Register(v1, rt)
	  // 出金广播前挂旅行规则闸；travelvasp 不得反向 import withdraw。
	  if desk := travelvasp.DeskFrom(rt); desk != nil {
		   if wd := withdraw.FromRuntime(rt); wd != nil {
			   wd.BindTravelGate(desk.AllowBroadcastOptional)
		  }
	   }
	  opsapi.Register(v1, rt)
	wrongcredit.Register(v1, rt)
	   subacct.Register(v1, rt)
	  fundpath.Register(v1, rt)
	   addrpool.Register(v1, rt)
	  dustguard.RegisterGuardRoutes(v1, rt)
	  bookbill.Register(v1, rt)
	   depclaim.Register(v1, rt)
	  wdrelease.Register(v1, rt)
	   if wd := withdraw.FromRuntime(rt); wd != nil {
		  if desk := wdrelease.DeskFrom(rt); desk != nil {
			  wd.BindRelease(desk)
		   }
	   }
	  wdpath.Register(v1, rt)
	   destlimit.Register(v1, rt)
	  wdcomp.Register(v1, rt)
	   hotbound.Register(v1, rt)
	   sweeprun.Register(v1, rt)
	  dustpath.Register(v1, rt)
	   dustquotegate.Register(v1, rt)
	   cerdecidegate.Register(v1, rt)
	  feebumpgate.Register(v1, rt)
	  earn.Register(v1, rt)
	  p2pgate.Register(v1, rt)
	  p2pgate.RegisterFunds(v1, rt)
	   p2pgate.RegisterOccupancyRead(v1, rt)
	   lending.Register(v1, rt)
	  fiatadd.Register(v1, rt)
	   stmthttp.Register(v1, rt)
	  kcfund.Register(r, rt)
	   h := fundsurf.HTTP{Surf: rt.FundSurf, PW: rt.TradePW, Gate: rt.WdGate, Sec: rt.SecLock, Mem: rt.Mem}
	   if rt.Cfg != nil {
		  h.ID = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	   }
	  if h.Surf == nil {
		  h.Surf = fundsurf.New(rt.KV, rt.Ledger, rt.Cfg).BindSecurity(rt.SecLock, rt.TradePW, rt.WdGate)
	   }
	   if h.PW == nil {
		  h.PW = rt.TradePW
	   }
	   if h.Gate == nil {
		   h.Gate = rt.WdGate
	   }
	   if h.Sec == nil {
		   h.Sec = rt.SecLock
	   }
	   h.MemoDep = deposit.MemoRecoverAdapter{Svc: deposit.New(rt.Ledger, rt.Wallet)}
	   h.Send = func(ctx context.Context, userID, factor, code string) error {
		  return notify.FromConfig(rt.KV, rt.Cfg).Publish(ctx, "wd-ch:"+userID+":"+factor, notify.KindWithdrawChallenge, factor)
	  }
	  if h.Gate != nil && h.Send != nil {
		   h.Gate.BindSender(h.Send)
	   }
	  fundsurf.Register(v1, h)
	   var matDB = (*sql.DB)(nil)
	   matDialect := persist.MySQL
	   if rt.SQL != nil {
		   matDB = rt.SQL.DB
		  matDialect = rt.SQL.Dialect
	  }
	   matRead := fundread.NewWired(matDB,
		  matDialect, rt.Ledger)
	  fundread.Mount(v1, matRead, h.ID)
	  if h.Surf != nil {
		  h.Surf.BindProjection(matRead)
	  }
	   exjob.RegisterOps(v1, rt.Exports)
	  por.Register(v1, rt)
	   balproof.Register(v1, rt)
	   solvency.Register(v1, rt)
	  return r
   }

   func Listen(ctx context.Context, rt *app.Runtime) error {
	  h := Router(rt)
	  if rt != nil {
		  rt.SetPublicHTTP(h)
	  }
	  srv := &http.Server{Addr: rt.Cfg.HTTPAddr, Handler: h}
	   go func() {
		   <-ctx.Done()
		   sh, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		  defer cancel()
		   _ = srv.Shutdown(sh)
	   }()
	   rt.Log.Info().Str("addr", rt.Cfg.HTTPAddr).Msg("HTTP API")
	  err := srv.ListenAndServe()
	   if errors.Is(err, http.ErrServerClosed) {
		  return nil
	  }
	   return err
   }
