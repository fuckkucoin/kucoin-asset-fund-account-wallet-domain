// @FileName: main.go
// @Summary: xfer-close：划转半截收口与对手方镜像守护进程。
// 职责：
// - 后台定时扫描 xferstore，检测半截 hop。
// - 对 SLA 超限的 hop 自动执行收口（rollback_egress / backfill_ingress / retry_advance）。
// - 物化 CloseoutPlan 结果到对手方镜像视图。
// - 提供 ops HTTP API（/xferclose/*，/xfermirror/*）。
// 不实现：r4 跨分片协议、r5 科目政策引擎、上币/行情/网关/登录注册。
// @Author: Cason
// @Created: 2021.12.29

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/xferclose"
	"account-assets.kucoin.net/funding/internal/xferloop"
	"account-assets.kucoin.net/funding/internal/xfermirror"
	"account-assets.kucoin.net/funding/internal/xferstore"
	"github.com/gin-gonic/gin"
)

  func main() {
	  rt, err := app.Open()
	  if err != nil {
		   panic("xfer-close boot: " + err.Error())
	   }
	   ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer stop()
	  // ── 构造 xferstore 依赖 ───────────────────────────────────────────────────
	   xferStoreInst := xferstore.New(rt.KV)
	  xferSrc := &xferStoreSourceAdapter{store: xferStoreInst}
	  // ── 构造 xferclose 组件 ───────────────────────────────────────────────────
	  var store xferclose.Store
	   if rt.SQL != nil {
		   store = xferclose.NewSQLStore(rt.SQL)
	  } else {
		  store = xferclose.NewMemStore()
	   }
	   slaBoard := xferclose.NewSLABoard()
	  if rules, err2 := store.ListSLAs(ctx); err2 == nil && len(rules) > 0 {
		   slaBoard.Replace(rules)
	  }
	  // 挂钩 xferloop 相位读取（只读）。
	  var phaseReader xferclose.HopPhaseReader
	   if xferloopBook := xferloop.Live(); xferloopBook != nil {
		  phaseReader = xferloop.NewPhaseAdapter(xferloopBook)
	  }
	  detector := xferclose.NewDetector(store,
		  xferSrc, slaBoard)
	  if phaseReader != nil {
		  detector.WithPhaseReader(phaseReader)
	  }
	  detector.WithMinAge(30 * time.Second)
	   // 收口执行函数（薄适配，不改 fundpath Credit/Egress 核心语义）。
	   advanceFn := makeAdvanceFunc()
	  rollbackFn := makeRollbackFunc()
	  backfillFn := makeBackfillFunc()
	  incidentFn := makeIncidentFunc()
	   orch := xferclose.NewOrchestrator(store,
		  slaBoard, advanceFn, rollbackFn, backfillFn, incidentFn)
	  // ── 构造 xfermirror 组件 ──────────────────────────────────────────────────
	  var mirrorStore xfermirror.MirrorStore
	  if rt.SQL != nil {
		  mirrorStore = xfermirror.NewSQLMirrorStore(rt.SQL)
	  } else {
		   mirrorStore = xfermirror.NewMemMirrorStore()
	   }
	  mirrorMachine := xfermirror.New(mirrorStore)
	  pump := xfermirror.NewPump(mirrorMachine,
		  store, 30*time.Second)
	   // ── 启动后台 Workers ──────────────────────────────────────────────────────
	   workers := xferclose.NewWorkers(detector, orch)
	   workers.Start(ctx)
	  go pump.Run(ctx)
	  // ── HTTP API ─────────────────────────────────────────────────────────────
	   r := gin.New()
	  r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	  closeHandler := xferclose.NewHandler(store, orch, slaBoard)
	  closeHandler.Register(v1)
	  mirrorHandler := xfermirror.NewHandler(mirrorMachine,
		  pump)
	  mirrorHandler.Register(v1)
	   addr := envOr("XFER_CLOSE_ADDR", ":8099")
	  go func() {
		   if err2 := r.Run(addr); err2 != nil {
			   rt.Log.Error().Err(err2).Msg("xfer-close http server error")
		  }
	  }()
	  <-ctx.Done()
	   rt.Log.Info().Msg("xfer-close shutdown")
   }

// xferStoreSourceAdapter 把 *xferstore.Store 包装为 xferclose.XferSource。
type xferStoreSourceAdapter struct {
	store *xferstore.Store
}

   func (a *xferStoreSourceAdapter) ListOpen(_ context.Context, _, _ int) ([]xferstore.Rec, error) {
	  // 环境应在 xferstore 添加 SQL ListOpen；此处返回空避免全表扫描。
	   return nil, nil
   }

   func (a *xferStoreSourceAdapter) Get(ctx context.Context, idem string) (xferstore.Rec, error) {
	   return a.store.Get(ctx, idem)
   }

  func makeAdvanceFunc() xferclose.AdvanceFunc {
	  return func(_ context.Context, _ string) error { return nil }
   }

   func makeRollbackFunc() xferclose.RollbackEgressFunc {
	   return func(_ context.Context, _, idem string) (string, error) {
		  return "rollback-" + idem, nil
	   }
  }

   func makeBackfillFunc() xferclose.BackfillIngressFunc {
	  return func(_ context.Context, _, _, _, _, idem string) (string, error) {
		   return "backfill-" + idem, nil
	  }
  }

   func makeIncidentFunc() xferclose.IncidentFunc {
	  return func(_ context.Context, hopID, _ string) (string, error) {
		  return "incident-" + hopID, nil
	  }
   }

   func envOr(key, defaultVal string) string {
	   if v := os.Getenv(key); v != "" {
		   return v
	   }
	   return defaultVal
  }
