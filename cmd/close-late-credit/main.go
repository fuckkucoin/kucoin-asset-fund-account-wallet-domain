// 功能：cmd/close-late-credit：关户迟贷 close_epoch 收口服务入口。
// 提供 CloseWithEpoch、CreditAssert、DisputeResolve、证明导出。
// 隔离：不改写地址池回收、Memo 工单、出箱泵 Mark。

// 作者：Draven
// 日期：2021.03.21

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/acctlifegate"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

type accountBooks struct{ s *account.Service }

  func (a accountBooks) Close(ctx context.Context, userID, book string) (acctlifegate.BookSnap, error) {
	  st, err := a.s.Close(ctx, userID, account.Book(book))
	  return acctlifegate.BookSnap{UserID: st.UserID, Book: string(st.Book), Status: string(st.Status)}, err
   }

   func (a accountBooks) Open(ctx context.Context, userID, book string) (acctlifegate.BookSnap, error) {
	   st, err := a.s.Open(ctx, userID, account.Book(book))
	   return acctlifegate.BookSnap{UserID: st.UserID, Book: string(st.Book), Status: string(st.Status)}, err
   }

   func main() {
	   zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	   acct := account.New(nil)
	  store := closelatecredit.NewMemStore()
	  cfg := closelatecredit.Config{}
	   workerCfg := closelatecredit.WorkerConfig{Interval: 30 * time.Second, BatchSize: 200}
	  router := gin.New()
	   router.Use(gin.Recovery())
	   rg := router.Group("/api/v1")
	  ctx, cancel := context.WithCancel(context.Background())
	   defer cancel()
	  mod := closelatecredit.Register(ctx, store, cfg, workerCfg, rg)
	  _ = acctlifegate.NewCloseEpoch(accountBooks{acct}, mod.Engine)
	  addr := app.HTTPListenAddr("8122")
	   srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	  go func() {
		   log.Info().Str("addr", addr).Msg("close-late-credit listening")
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("server error")
		  }
	   }()
	   quit := make(chan os.Signal, 1)
	  signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	  <-quit
	  log.Info().Msg("shutting down close-late-credit")
	   mod.Stop()
	   cancel()
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	   _ = srv.Shutdown(shutCtx)
   }
