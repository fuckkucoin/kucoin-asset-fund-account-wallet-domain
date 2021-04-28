// @FileName: main.go
// @Summary: 手续费报价租约漂移封堵引擎：独立服务入口。
// 启动参数（环境变量）：
// FEE_LEASE_ADDR — HTTP 监听地址（默认 :8097）
// FEE_LEASE_DSN — MySQL DSN（空则使用内存模式）
// FEE_LEASE_EXPIRE_INT — 过期 Worker 间隔（秒，默认 30）
// FEE_LEASE_EXPORT_INT — 证明导出 Worker 间隔（秒，默认 60）
// @Author: Jett
// @Created: 2021.04.27

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/internal/feequotelease"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

   func main() {
	   addr := envOr("FEE_LEASE_ADDR", ":8097")
	   dsn := os.Getenv("FEE_LEASE_DSN")
	  expireInt := envDur("FEE_LEASE_EXPIRE_INT", 30)
	   exportInt := envDur("FEE_LEASE_EXPORT_INT", 60)
	   book := feeeng.DefaultBook()
	   adapter := feequotelease.NewFeeEngAdapter(book)
	  var store feequotelease.Store
	   if dsn != "" {
		   db, err := sql.Open("mysql", dsn)
		  if err != nil {
			   panic("open mysql: " + err.Error())
		   }
		  db.SetMaxOpenConns(16)
		  db.SetMaxIdleConns(4)
		  db.SetConnMaxLifetime(5 * time.Minute)
		  sqldb := &persist.SQLDB{DB: db, Dialect: persist.MySQL}
		   store = feequotelease.NewSQLStore(sqldb)
		  book.BindSQL(sqldb)
	  } else {
		  store = feequotelease.NewInMemStore()
	  }
	   svc := feequotelease.NewWithStore(store, adapter)
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	  defer cancel()
	  // 过期 Worker
	   expireWorker := feequotelease.NewExpireWorker(svc, expireInt)
	  go expireWorker.Run(ctx)
	   // 证明导出 Worker
	  exportWorker := feequotelease.NewProofExportWorker(svc, exportInt, feequotelease.NoopSink{})
	   go exportWorker.Run(ctx)
	   r := gin.New()
	   r.Use(gin.Recovery())
	   v1 := r.Group("/v1")
	  // 直接注册路由（不通过 app.Runtime 以支持独立部署）
	   registerRoutes(v1, svc, book)
	  srv := &http.Server{Addr: addr, Handler: r}
	   go func() {
		   <-ctx.Done()
		   shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		  defer cancel()
		   _ = srv.Shutdown(shutdownCtx)
	   }()
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		   panic("listen: " + err.Error())
	  }
   }

  func registerRoutes(v1 *gin.RouterGroup, svc *feequotelease.Service, book *feeeng.Book) {
	  g := v1.Group("/fees")
	  g.POST("/quote-lease", func(c *gin.Context) {
		   var body struct {
			   Kind string `json:"kind"`
			  Asset string `json:"asset"`
			  Gross string `json:"gross"`
			   Volume string `json:"volume"`
			  TTLSeconds     int    `json:"ttl_seconds"`
			  IdempotencyKey string `json:"idempotency_key"`
			   Tiers []struct {
				  Notional string `json:"notional"`
				   BPS int `json:"bps"`
			  } `json:"tiers"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		  tiers := make([]feequotelease.Tier, 0, len(body.Tiers))
		  for _, t := range body.Tiers {
			  tiers = append(tiers, feequotelease.Tier{Notional: t.Notional, BPS: t.BPS})
		   }
		  req := feequotelease.QuoteLeaseReq{
			   Kind:  body.Kind,
			   Asset: body.Asset,
			  Gross: body.Gross,
			  Volume:         body.Volume,
			   TTLSeconds:     body.TTLSeconds,
			   IdempotencyKey: body.IdempotencyKey,
		  }
		   lease, err := svc.QuoteLease(c.Request.Context(), req, tiers)
		   if err != nil {
			  c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(http.StatusOK, lease)
	   })
	   g.POST("/apply-bound", func(c *gin.Context) {
		  var body struct {
			   LeaseID        string `json:"lease_id"`
			   IdempotencyKey string `json:"idempotency_key"`
			  Tiers []struct {
				   Notional string `json:"notional"`
				   BPS      int    `json:"bps"`
			   } `json:"tiers"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			  return
		  }
		   tiers := make([]feequotelease.Tier, 0, len(body.Tiers))
		   for _, t := range body.Tiers {
			  tiers = append(tiers,
				  feequotelease.Tier{Notional: t.Notional, BPS: t.BPS})
		   }
		   req := feequotelease.ApplyBoundReq{
			   LeaseID: body.LeaseID,
			  IdempotencyKey: body.IdempotencyKey,
		   }
		   result, err := svc.ApplyBound(c.Request.Context(), req, tiers)
		   if err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, result)
	   })
	  g.GET("/leases/:id", func(c *gin.Context) {
		   id := c.Param("id")
		  lease, err := svc.GetLease(c.Request.Context(), id)
		   if err != nil {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(http.StatusOK, lease)
	   })
	   g.GET("/drift-proofs", func(c *gin.Context) {
		  proofs, err := svc.ExportProofs(c.Request.Context())
		  if err != nil {
			  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"proofs": proofs, "count": len(proofs)})
	  })
   }

   func envOr(key, def string) string {
	  if v := os.Getenv(key); v != "" {
		  return v
	   }
	  return def
  }

  func envDur(key string, defSec int) time.Duration {
	   if v := os.Getenv(key); v != "" {
		   if n, err := strconv.Atoi(v); err == nil && n > 0 {
			  return time.Duration(n) * time.Second
		   }
	   }
	  return time.Duration(defSec) * time.Second
  }
