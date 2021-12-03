package svcboot

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/logx"
	"account-assets.kucoin.net/funding/internal/meshgrpc"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"account-assets.kucoin.net/funding/internal/trace"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type RegisterFunc func(v1 *gin.RouterGroup, rt *app.Runtime)

type EngineFunc func(r *gin.Engine, rt *app.Runtime)

  func Main(name, host string, register RegisterFunc) {
	  MainEngine(name, host, register, nil)
   }

   func MainEngine(name, host string, register RegisterFunc, extra EngineFunc) {
	  flag.Parse()
	  rt, err := app.Open()
	   if err != nil {
		   lg := logx.New("error")
		   lg.Fatal().Err(err).Msg("boot")
	   }
	  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer stop()
	  rt.StartBackground(ctx)
	  inst := svcreg.Instance{Name: name, Host: host, Addr: config.ServiceDialURL(host, rt.Cfg.HTTPAddr)}
	  go KeepRegister(ctx, rt, inst)
	   defer func() {
		   sh,
			   cancel := context.WithTimeout(context.Background(), 2*time.Second)
		   defer cancel()
		   if rt.Registry != nil {
			  if err := rt.Registry.Deregister(sh, inst); err != nil {
				  rt.Log.Error().Err(err).Str("service", name).Msg("registry deregister")
			  }
		   }
	   }()
	  h := RouterExtra(name, host, rt, register, extra)
	  rt.SetPublicHTTP(h)
	  go func() {
		  if err := meshgrpc.Listen(ctx, config.GRPCListen(rt.Cfg.HTTPAddr), h); err != nil && ctx.Err() == nil {
			  rt.Log.Error().Err(err).Str("addr", config.GRPCListen(rt.Cfg.HTTPAddr)).Msg("mesh grpc")
		  }
	  }()
	  srv := &http.Server{Addr: rt.Cfg.HTTPAddr, Handler: h}
	  go func() {
		  <-ctx.Done()
		  sh, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		  defer cancel()
		  _ = srv.Shutdown(sh)
	  }()
	  rt.Log.Info().Str("service", name).Str("host", host).Str("addr", rt.Cfg.HTTPAddr).Msg("listen")
	  if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		   rt.Log.Fatal().Err(err).Msg("http")
	   }
   }

  func Router(name, host string, rt *app.Runtime, register RegisterFunc) *gin.Engine {
	  return RouterExtra(name, host, rt, register, nil)
   }

   func RouterExtra(name, host string, rt *app.Runtime, register RegisterFunc, extra EngineFunc) *gin.Engine {
	   gin.SetMode(gin.ReleaseMode)
	   r := gin.New()
	  r.Use(gin.Recovery())
	  r.Use(mw.RequireHost(mw.HostSuffix))
	  r.Use(trace.HTTP(rt.Log))
	  r.GET("/", func(c *gin.Context) {
		  c.JSON(200, gin.H{
			  "service": name,
			  "host":       host,
			  "disclaimer": slo.ProcessDisclaimer,
		  })
	   })
	   r.GET("/healthz", func(c *gin.Context) {
		  c.JSON(200, gin.H{"ok": true, "service": name, "host": host})
	   })
	   r.GET("/readyz", func(c *gin.Context) {
		   body, code := rt.Ready(c.Request.Context())
		   if app.ShouldProbeJavaDelegate(name) {
			   app.MergeJavaDelegateReady(c.Request.Context(), rt, body, &code)
		  }
		  c.JSON(code, body)
	  })
	  r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	   v1 := r.Group("/v1")
	   v1.Use(requireAuth(rt))
	   if register != nil {
		  register(v1, rt)
	   }
	   if extra != nil {
		   extra(r, rt)
	  }
	   return r
  }

   func KeepRegister(ctx context.Context, rt *app.Runtime, inst svcreg.Instance) {
	  if rt == nil || rt.Registry == nil {
		  return
	   }
	   attempt := 0
	   for {
		   if err := ctx.Err(); err != nil {
			  return
		   }
		   if ka, ok := rt.Registry.(interface{ KeepAlive(context.Context) error }); ok && rt.Registry.Kind() == "etcd" {
			   if err := ka.KeepAlive(ctx); err != nil {
				   rt.Log.Error().Err(err).Str("service", inst.Name).Msg("registry keepalive")
			  }
		   }
		   if err := rt.Registry.Register(ctx, inst); err != nil {
			   rt.Log.Error().Err(err).Str("service", inst.Name).Int("attempt", attempt+1).Msg("registry register")
			   attempt++
			   select {
			  case <-ctx.Done():
				  return
			  case <-time.After(svcreg.SleepBackoff(attempt)):
			   }
			   continue
		   }
		   attempt = 0
		  select {
		  case <-ctx.Done():
			   return
		  case <-time.After(8 * time.Second):
		   }
	  }
   }

  func requireAuth(rt *app.Runtime) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  kind, profile := "", ""
		  if rt != nil && rt.Cfg != nil {
			  kind, profile = rt.Cfg.Auth.Kind, rt.Cfg.Profile
		  }
		  var g *authn.Guard
		  if rt != nil {
			  g = rt.Auth
		  }
		  skip, deny := g.Gate(c.Request.Method, authn.ProductionProfile(kind, profile))
		  if deny {
			   c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
			   return
		  }
		   if skip {
			   c.Next()
			  return
		   }
		   body, err := io.ReadAll(c.Request.Body)
		  if err != nil {
			  c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
			  return
		   }
		   c.Request.Body = io.NopCloser(bytes.NewReader(body))
		  if err := rt.Auth.Authenticate(c.Request, body); err != nil {
			   c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
			  return
		   }
		   c.Next()
	  }
   }
