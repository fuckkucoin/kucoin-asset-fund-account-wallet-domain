// @FileName: main.go
// @Summary: halt-clear 服务进程：清闸四眼协议 + 误清检测 + 再熔断 Worker 组合入口。
// 启动后运行 InspectorWorker / ReEngageExecutorWorker / AuditSealWorker，并绑定 HTTP 路由。
// @Author: Cason
// @Created: 2021.07.09

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/haltclear"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/store"
)

   func main() {
	   if err := run(); err != nil {
		   fmt.Fprintf(os.Stderr, "halt-clear: %v\n", err)
		   os.Exit(1)
	  }
  }

  func run() error {
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	   defer cancel()
	   // ---- 持久化 -------------------------------------------------------
	  var kv persist.KV
	   var caseRepo haltclear.CaseRepo
	  var legRepo haltclear.LegRepo
	  pgDSN := os.Getenv("ACCOUNT_ASSETS_PG_DSN")
	  if pgDSN == "" {
		   pgDSN = persist.PGDSNFromEnv()
	   }
	   if pgDSN != "" {
		   db, err := store.OpenPostgres(pgDSN)
		   if err != nil {
			   return fmt.Errorf("open postgres: %w", err)
		  }
		   kv = persist.NewSQLKVDialect(db,
			   persist.Postgres)
		  caseRepo = haltclear.NewSQLCaseRepo(db)
		  legRepo = haltclear.NewSQLLegRepo(db)
	   } else {
		  // 进程内内存实现（单进程演练 / ）。
		   kv = persist.NewMemKV()
		  caseRepo = haltclear.NewMemCaseRepo()
		  legRepo = haltclear.NewMemLegRepo()
	   }
	   // ---- custodyhalt Gate -----------------------------------------------
	  gate := custodyhalt.New(kv)
	   quorumKV := kv
	   quorumStore := custodyhalt.NewQuorumStore(quorumKV)
	   forcedAuditStore := custodyhalt.NewForcedAuditStore(kv)
	   // 四眼前置守卫：未装配时 Clear 被 QuorumGuard 拦截。
	   guard := custodyhalt.NewQuorumGuard(quorumStore)
	  gate.BindClearCheck(guard)
	  // ---- haltclear Service -----------------------------------------------
	  diffQuery := haltclear.NoopDiffQuerier{} // 环境替换为对账路径实现
	   detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo)
	  rollback := haltclear.NewReEngageRollback(gate, caseRepo, legRepo, diffQuery)
	  svc := haltclear.NewService(haltclear.ServiceConfig{
		   Gate: gate,
		  Quorum: quorumStore,
		   ForcedAudit: forcedAuditStore,
		   Detector: detector,
		   Rollback: rollback,
		  CaseRepo: caseRepo,
		   LegRepo:  legRepo,
	  })
	  // ---- Worker 组 -------------------------------------------------------
	  inspectInterval := envDuration("HALTCLEAR_INSPECT_INTERVAL", 60*time.Second)
	   execInterval := envDuration("HALTCLEAR_EXEC_INTERVAL", 15*time.Second)
	  sealInterval := envDuration("HALTCLEAR_SEAL_INTERVAL", 120*time.Second)
	   autoReengage := os.Getenv("HALTCLEAR_AUTO_REENGAGE") == "true"
	   onError := func(err error) {
		   fmt.Fprintf(os.Stderr, "halt-clear worker error: %v\n", err)
	  }
	   onDetect := func(r haltclear.DetectResult) {
		  if r.MisclearID != "" {
			   fmt.Printf("halt-clear: misclear detected scope=%s chain=%s asset=%s id=%s diffs=%d\n",
				   r.Scope, r.Chain, r.Asset, r.MisclearID, r.OpenDiffCount)
		   }
	  }
	  onExecute := func(legID string, proof *haltclear.RollbackProof, err error) {
		  if err != nil {
			   fmt.Fprintf(os.Stderr, "halt-clear: execute leg %s failed: %v\n", legID, err)
			   return
		   }
		  if proof != nil {
			   fmt.Printf("halt-clear: reengage proof %s leg=%s misclear=%s valid=%v\n",
				  proof.ProofID,
				  proof.LegID, proof.MisclearID, haltclear.VerifyRollbackProof(proof))
		  }
	   }
	   onSeal := func(misclearID string, valid bool) {
		   fmt.Printf("halt-clear: audit seal misclear=%s valid=%v\n", misclearID, valid)
	  }
	  inspector := haltclear.NewInspectorWorker(detector, rollback, inspectInterval, autoReengage, onDetect, onError)
	  executor := haltclear.NewReEngageExecutorWorker(rollback, caseRepo, legRepo, execInterval, onExecute, onError)
	   sealer := haltclear.NewAuditSealWorker(caseRepo, legRepo, sealInterval, onSeal, onError)
	  workerGroup := haltclear.NewWorkerGroup(inspector, executor, sealer)
	   // ---- HTTP 路由 -------------------------------------------------------
	  gin.SetMode(gin.ReleaseMode)
	  r := gin.New()
	  r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	  haltclear.Mount(v1, svc, nil, noopActorFn)
	  // 挂载 custodyhalt 原有 HTTP（保持兼容）。
	  custodyhalt.Mount(v1, gate, nil, noopCustodyActorFn)
	   addr := envStr("HALTCLEAR_ADDR", ":8084")
	   srv := &http.Server{Addr: addr, Handler: r, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	  go func() {
		   fmt.Printf("halt-clear: HTTP listening on %s\n", addr)
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  fmt.Fprintf(os.Stderr, "halt-clear: HTTP error: %v\n", err)
			  cancel()
		  }
	  }()
	  // Worker 在独立 goroutine 运行。
	   workerErr := make(chan error, 1)
	   go func() {
		   workerErr <- workerGroup.Run(ctx)
	  }()
	  <-ctx.Done()
	  fmt.Println("halt-clear: shutting down")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	  defer shutCancel()
	   _ = srv.Shutdown(shutCtx)
	   select {
	  case err := <-workerErr:
		   if err != nil && err != context.Canceled {
			   return err
		   }
	  case <-time.After(10 * time.Second):
	   }
	   return nil
  }

// ---- 辅助 -----------------------------------------------------------

   func envDuration(key string, def time.Duration) time.Duration {
	   v := os.Getenv(key)
	   if v == "" {
		  return def
	   }
	  d, err := time.ParseDuration(v)
	  if err != nil {
		   return def
	   }
	  return d
   }

   func envStr(key, def string) string {
	   if v := os.Getenv(key); v != "" {
		   return v
	   }
	  return def
  }

// noopActorFn / noopCustodyActorFn：环境替换为业务 authn 实现。

   func noopActorFn(c *gin.Context) (haltclear.ActorChecker, bool) {
	  id := c.GetHeader("X-Ops-Actor")
	  if id == "" {
		  c.JSON(http.StatusUnauthorized, gin.H{"error": "X-Ops-Actor header required"})
		   c.Abort()
		   return nil, false
	  }
	   return simpleActor(id), true
  }

   func noopCustodyActorFn(c *gin.Context) (custodyhalt.ActorChecker, bool) {
	   id := c.GetHeader("X-Ops-Actor")
	  if id == "" {
		   c.JSON(http.StatusUnauthorized, gin.H{"error": "X-Ops-Actor header required"})
		   c.Abort()
		  return nil, false
	   }
	   return simpleCustodyActor(id), true
  }

type simpleActor string

  func (a simpleActor) ID() string      { return string(a) }
   func (a simpleActor) CheckOps() error { return nil }

type simpleCustodyActor string

   func (a simpleCustodyActor) ID() string      { return string(a) }
   func (a simpleCustodyActor) CheckOps() error { return nil }
