// 进程装配：账本、钱包、兑换、行情和依赖接线。
// 作者: Likeo
// 日期: 2021.02.06
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	walletv1 "account-assets.kucoin.net/funding/api/proto/wallet/v1"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/cfgwatch"
	"account-assets.kucoin.net/funding/internal/conc"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/cryptoenc"
	"account-assets.kucoin.net/funding/internal/cvfund"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/events"
	"account-assets.kucoin.net/funding/internal/exjob"
	"account-assets.kucoin.net/funding/internal/fundmesh"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/grpcsvc"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/isobook"
	"account-assets.kucoin.net/funding/internal/kms"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/logx"
	"account-assets.kucoin.net/funding/internal/market"
	"account-assets.kucoin.net/funding/internal/meshgrpc"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/redisx"
	"account-assets.kucoin.net/funding/internal/role"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/spot"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/store"
	"account-assets.kucoin.net/funding/internal/subkey"
	"account-assets.kucoin.net/funding/internal/svcdial"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"account-assets.kucoin.net/funding/internal/swap"
	"account-assets.kucoin.net/funding/internal/trace"
	"account-assets.kucoin.net/funding/internal/tradepw"
	"account-assets.kucoin.net/funding/internal/userdir"
	"account-assets.kucoin.net/funding/internal/utabook"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/wdverify"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
)

type Runtime struct {
	Cfg *config.Config
	Log zerolog.Logger
	Wallet *wallet.Service
	Ledger *ledger.Service
	Swap *swap.Service
	Spot *spot.Service
	Market *market.Service
	Store ledger.Store
	KV    persist.KV
	SQL *persist.SQLDB
	Bus   bus.Bus
	// outboxLive 记下本进程已经交给内存总线的事件。库里的行仍留着，进程重启后还能再投一次。
	outboxLive sync.Map
	Redis *redis.Client
	Enc   *cryptoenc.Envelope
	Mem   bool
	Auth  *authn.Guard
	Durable Durable
	Registry svcreg.Registry
	CfgWatch cfgwatch.Store
	Dial     *svcdial.Client
	pubHTTP  atomic.Value
	Users    userdir.Directory
	Exports  *exjob.Book
	Convert  *cvfund.Book
	Isolated *isobook.Book
	SubKeys  *subkey.Store
	UTA      *utabook.Book
	TradePW  *tradepw.Book
	FundSurf *fundsurf.Book
	FundMesh *fundmesh.Client
	// FundPref 资金偏好簿；具体类型 *fundpref.Book，由网关偏好路由挂载（避免 app↔fundpref 环）。
	FundPref any
	SecLock  *seclock.Service
	WdGate    *wdverify.Gate
	DestAllow *destallow.Book
	// TravelThresh 旅行规则走廊门槛表；具体类型 *travelvasp.ThresholdBook，由 fundpath/travelvasp 热更新共享。
	TravelThresh any
	// TravelVASP 旅行规则报文目录；具体类型 *travelvasp.Service，由 kcfund 首次使用时挂载。
	TravelVASP any
	// TravelDesk 旅行规则出站台；与 TravelVASP 指向同一 *travelvasp.Service。
	TravelDesk any
	// FXMarks 估值/旅行名义牌价簿（Live+Static）；具体类型 *fxmark.Book。
	FXMarks any
	// Withdraw 出金编排单例；具体类型 *withdraw.Service，由 withdraw.Attach / Register 挂载（避免 app↔withdraw 环）。
	Withdraw any
	// WDRelease 出金多级放行台；具体类型 *wdrelease.Desk，由 wdrelease.Attach 挂载。
	WDRelease any
	// DestLimit 目的地动态限额引擎；具体类型 *destlimit.Engine，由 destlimit.Attach 挂载。
	DestLimit any
	// Deposit 入金编排单例；具体类型 *deposit.Service，由 deposit.Attach / Register 挂载（避免 app↔deposit 环）。
	Deposit any
	FundsRecon func(context.Context)
	// QuotaLeaseExpire worker 周期回收过期母子额度租赁；由 BindFundsRecon 挂载。
	QuotaLeaseExpire func(context.Context)
	// TravelFXRefresh worker 周期刷新旅行规则 USD 牌价（Live→Static→SQL）。
	TravelFXRefresh func(context.Context)
	// ReorgSagaTick worker 周期推进出金 reorg saga（自动 resign / 超时失败）。
	ReorgSagaTick func(context.Context)
	// FamMemoScan 扫 FamMEMO 共享地址并按观察 tag 自动归因。
	FamMemoScan func(context.Context)
	// SharedMemo FamMEMO 共享充值地址池；具体 *addrpool.SharedMemoPool。
	SharedMemo any
	// DepositAttr 入金 memo/tag 归因；具体 *deposit.Attributor。
	DepositAttr any
	// DustGuard 粉尘/灰尘攻击防护服务；具体 *dustguard.GuardService，由 dust-guard worker 挂载。
	DustGuard any
	reconCh chan string
}

// ChainBridge 把 wallet.Registry 接到账本写路径：Submit / Status / Observe / Inspect。
type ChainBridge struct{ W *wallet.Service }

  func (c ChainBridge) Submit(ctx context.Context, chain, to string, amount decimal.Decimal) (string, error) {
	   return c.W.Submit(ctx, chain, to, amount)
   }

  func (c ChainBridge) Status(ctx context.Context, chain, hash string) (wallet.TxStatus, error) {
	   return c.W.TxStatus(ctx, chain, hash)
   }

  func (c ChainBridge) Observe(ctx context.Context, chain, hash string, amount decimal.Decimal) (string, error) {
	   return c.W.Observe(ctx, chain, hash, amount)
   }

  func (c ChainBridge) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	  if obs, ok := wallet.ObservationFrom(ctx, chain, hash); ok {
		   return obs, nil
	  }
	   return c.W.Inspect(ctx, chain, hash)
  }

// LoadEnvFile 读 .env；已有环境变量先，不被文件覆盖。
   func LoadEnvFile(path string) {
	  b, err := os.ReadFile(path)
	   if err != nil {
		   return
	   }
	   for _, line := range strings.Split(string(b), "\n") {
		  line = strings.TrimSpace(line)
		   if line == "" || strings.HasPrefix(line, "#") {
			  continue
		   }
		   k, v, ok := strings.Cut(line, "=")
		   if !ok {
			  continue
		   }
		  if os.Getenv(k) == "" {
			   _ = os.Setenv(k, v)
		  }
	   }
  }

var (
	ErrMemForbidden = errors.New("in-memory ledger unavailable under production auth profile")
	ErrMockChain    = errors.New("chain_driver=mock unavailable; Open requires RPC endpoints from config")
	ErrMigrateMem   = errors.New("migrate requires SQL Open")
	ErrFileHSMForbidden = kms.ErrFileKeeper
)

// RefuseMem 在 iam/production 鉴权档下阻止内存账本装配。

  func RefuseMem(cfg *config.Config) error {
	   if cfg == nil {
		   return ErrMemForbidden
	  }
	  if authn.ProductionProfile(cfg.Auth.Kind, cfg.Profile) {
		   return ErrMemForbidden
	  }
	  return nil
  }

// RefuseFileHSM 档必须 PKCS#11 或云 KMS。file / filehsm 失败即关。
   func RefuseFileHSM(cfg *config.Config) error {
	  if cfg == nil {
		  return ErrFileHSMForbidden
	  }
	  return kms.RefuseProduction(cfg.Auth.Kind, cfg.Profile, cfg.KMS)
   }

  func refuseMockChain(cfg *config.Config) error {
	  if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.ChainDriver), "mock") {
		   return ErrMockChain
	   }
	   return nil
  }

   func usePostgres(cfg *config.Config) bool {
	  if cfg == nil || strings.TrimSpace(cfg.Postgres.DSN) == "" {
		  return false
	  }
	   return persist.ParseDialect(os.Getenv("ACCOUNT_ASSETS_DIALECT")) == persist.Postgres
   }

   func openProdDB(cfg *config.Config, log zerolog.Logger) (*sql.DB, persist.Dialect, error) {
	   if usePostgres(cfg) {
		   pool := persist.PoolSettings{
			  MaxOpen: cfg.Postgres.MaxOpen, MaxIdle: cfg.Postgres.MaxIdle,
			   ConnMaxLifetime: cfg.Postgres.ConnMaxLifetime, ConnMaxIdle: cfg.Postgres.ConnMaxIdle,
		   }
		   db, err := store.OpenPostgresPool(cfg.Postgres.DSN, pool)
		   if err != nil {
			  return nil, persist.Postgres, err
		   }
		   return db, persist.Postgres, nil
	   }
	  db, err := store.OpenMySQLPool(cfg.MySQLDSN, cfg.MySQL.MaxOpen, cfg.MySQL.MaxIdle, cfg.MySQL.ConnMaxLifetime)
	  if err != nil {
		   return nil, persist.MySQL, err
	   }
	   return db, persist.MySQL, nil
  }

// attachLedgerShards 按 LEDGER_SHARD_DSNS 再挂账本库。用户号哈希选片；不填则仍用原来的一个库。
func attachLedgerShards(sqlStore *ledger.SQLStore, dialect persist.Dialect, cfg *config.Config) error {
	raw := strings.TrimSpace(os.Getenv("LEDGER_SHARD_DSNS"))
	if raw == "" || sqlStore == nil || cfg == nil {
		return nil
	}
	var extra []*sql.DB
	for _, dsn := range strings.Split(raw, ",") {
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			continue
		}
		var db *sql.DB
		var err error
		if dialect == persist.Postgres {
			pool := persist.PoolSettings{
				MaxOpen: cfg.Postgres.MaxOpen, MaxIdle: cfg.Postgres.MaxIdle,
				ConnMaxLifetime: cfg.Postgres.ConnMaxLifetime, ConnMaxIdle: cfg.Postgres.ConnMaxIdle,
			}
			db, err = store.OpenPostgresPool(dsn, pool)
		} else {
			db, err = store.OpenMySQLPool(dsn, cfg.MySQL.MaxOpen, cfg.MySQL.MaxIdle, cfg.MySQL.ConnMaxLifetime)
		}
		if err != nil {
			return err
		}
		if _, _, err = applyProdSchema(context.Background(), db, dialect); err != nil {
			return err
		}
		extra = append(extra, db)
	}
	sqlStore.AddShards(extra)
	return nil
}

   func applyProdSchema(ctx context.Context, db *sql.DB, dialect persist.Dialect) (*persist.SQLDB, persist.KV, error) {
	   sqlDB := persist.OpenSQLDialect(db, dialect)
	   if err := sqlDB.ApplySchema(ctx); err != nil {
		  return nil, nil, fmt.Errorf("apply schema: %w", err)
	   }
	  if _, err := persist.ApplyOnce(ctx, db, dialect, config.FindRepoFile("migrations")); err != nil {
		   return nil, nil, fmt.Errorf("apply migrations: %w", err)
	   }
	   kv := persist.NewSQLKVDialect(db, dialect)
	   if err := kv.ApplyKVSchema(ctx); err != nil {
		  return nil, nil, fmt.Errorf("apply kv schema: %w", err)
	  }
	  return sqlDB, kv, nil
  }

// bindMemRuntime 内存装配不打公链/HSM/厂商/IAM，只给流程。

  func bindMemRuntime(cfg *config.Config) {
	   if cfg == nil {
		  return
	  }
	   cfg.ChainDriver = "mock"
	   cfg.KMS.Kind = "file"
	  cfg.KeySource = "file"
	  if strings.TrimSpace(cfg.KMS.KEKPath) == "" {
		  cfg.RSAKeyPath = config.FindRepoFile("configs/kms/kek.pem")
		  cfg.KMS.KEKPath = cfg.RSAKeyPath
	  }
	  cfg.KYT.Mode = "allow"
	  cfg.KYT.VendorURL = ""
	   cfg.Auth.Kind = "mem"
	   cfg.Auth.Enabled = true
	  cfg.Auth.AllowInsecure = false
	   cfg.Profile = "mem"
	   cfg.WalletRevealMnemonic = true
	   cfg.NodeHealth.QueryTimeout = 8 * time.Second
	   cfg.NodeHealth.ChainRPCTimeout = 8 * time.Second
	   cfg.KMS.Timeout = 8 * time.Second
	  cfg.KYT.Timeout = 8 * time.Second
	  cfg.Etcd.Endpoints = nil
	   cfg.Etcd.Env = "mem"
	  if len(cfg.Auth.BearerTokens) == 0 {
		  cfg.Auth.BearerTokens = []string{"mem-" + time.Now().UTC().Format("150405.000000")}
	  }
   }

// Open 装配钱包/账本/兑换/行情到一个进程，日志、指标、追踪从这里挂上。
  func Open() (*Runtime, error) {
	  LoadEnvFile(".env")
	   cfg, err := config.Load()
	  if err != nil {
		   return nil, err
	   }
	  if err := refuseMockChain(cfg); err != nil {
		   return nil, err
	  }
	   if err := RefuseProduction(cfg); err != nil {
		   return nil, err
	  }
	  log := logx.New(cfg.LogLevel)
	   db, dialect, err := openProdDB(cfg, log)
	   if err != nil {
		   return nil, err
	   }
	   sqlDB,
		   kv, err := applyProdSchema(context.Background(), db, dialect)
	  if err != nil {
		   return nil, err
	  }
	  rdb, err := store.OpenRedisPool(cfg.RedisAddr, cfg.RedisPool.PoolSize, cfg.RedisPool.MinIdleConns)
	  if err != nil {
		   return nil, err
	   }
	  enc, err := cryptoenc.Open(cfg)
	  if err != nil || enc == nil {
		   if err == nil {
			   err = fmt.Errorf("kms envelope required")
		  }
		  return nil, err
	  }
	   sqlStore := ledger.NewSQLStoreDialect(db, dialect)
	   sqlStore.SetSeal(enc)
	   if err := attachLedgerShards(sqlStore, dialect, cfg); err != nil {
		   return nil, err
	   }
	   if err := persist.EnsureLedgerGuards(context.Background(), db, dialect); err != nil {
		   return nil, err
	   }
	  books := ledger.BookAdapter{S: sqlStore}
	  wallets := wallet.NewService(cfg, rdb, books, wallet.NewSQLRepoDialect(db, enc, dialect), enc, wallet.NewSQLHistoryDialect(db, dialect))
	  wallets.BindRegistered(func(ctx context.Context, chain string) ([]wallet.RegisteredToken, error) {
		  rows, err := sqlrepo.NewTokens(sqlDB).List(ctx)
		  if err != nil {
			  return nil, err
		  }
		  chain = strings.ToUpper(strings.TrimSpace(chain))
		  out := make([]wallet.RegisteredToken, 0)
		  for _, t := range rows {
			  if !strings.EqualFold(t.Chain, chain) {
				  continue
			  }
			  out = append(out, wallet.RegisteredToken{Asset: t.Asset, Contract: t.Contract, Decimals: t.Decimals})
		  }
		  return out, nil
	  })
	   risk, err := ledger.NewRisk(cfg.Risk)
	  if err != nil {
		  return nil, err
	  }
	   led := ledger.NewService(sqlStore, risk, ledger.RedisLocker{R: rdb}, enc, ChainBridge{W: wallets}, log, cfg.Transfer)
	  screen, err := kyt.NewScreener(cfg.KYT)
	  if err != nil {
		  return nil, err
	   }
	   led.SetKYT(screen)
	   led.SetDepositAddress(wallets.AddressFor)
	  var b bus.Bus
	  if authn.ProductionProfile(cfg.Auth.Kind, cfg.Profile) || !cfg.KafkaOptional {
		  pc := bus.LoadProdConfig()
		  if len(pc.Brokers) == 0 {
			  pc.Brokers = append([]string(nil),
				  cfg.KafkaBrokers...)
		   }
		  pc.Optional = cfg.KafkaOptional
		  var err error
		  b, err = bus.OpenProd(pc, log)
		   if err != nil {
			  return nil, err
		   }
	   } else {
		   b = bus.Open(cfg.KafkaBrokers, cfg.KafkaOptional, log)
	  }
	  if b != nil && b.Kind() == "memory" && db != nil {
		  dbus, err := bus.OpenDurable(db, dialect, rdb, enc, log)
		  if err != nil {
			  return nil, err
		  }
		  b = dbus
	  }
	   mkt := market.New(b, rdb, cfg, log)
	  sw := swap.New(sqlStore, ledger.RedisLocker{R: rdb}, swap.CachePricer{
		   Live: func(from, to string) (decimal.Decimal, bool) {
			   return mkt.QuoteRate(from, to, cfg.Swap.QuoteMaxAge)
		  },
		   Has: mkt.HasPair,
	  }, cfg.Swap)
	   sw.OnBooks = wallets.Invalidate
	  sw.Guard = led.GuardUser
	   bridge := ChainBridge{W: wallets}
	  sw.Submit = bridge.Submit
	   sp := spot.New(sqlStore, ledger.RedisLocker{R: rdb}, swap.CachePricer{
		   Live: func(from, to string) (decimal.Decimal, bool) {
			  return mkt.QuoteRate(from, to, cfg.Swap.QuoteMaxAge)
		   },
		  Has: mkt.HasPair,
	  }, cfg.Spot)
	   sp.Guard = led.GuardUser
	   sp.OnPrint = func(symbol, price string) {
		  mkt.ApplyTick(market.Tick{Symbol: symbol, Price: price, Ts: time.Now().UnixMilli()})
	  }
	   rt := &Runtime{
		   Cfg: cfg, Log: log, Wallet: wallets, Ledger: led, Swap: sw, Spot: sp, Market: mkt,
		   Store: sqlStore, KV: kv, SQL: sqlDB, Bus: b, Redis: rdb, Enc: enc,
		  Auth: authn.NewFrom(cfg.Auth),
		  reconCh: make(chan string, 64),
	  }
	  onBooks := func(ctx context.Context, userID string) {
		  wallets.Invalidate(ctx, userID)
		   rt.hintReconcile(userID)
	  }
	   led.OnBooks = onBooks
	   sw.OnBooks = onBooks
	   sp.OnBooks = onBooks
	   attachDurable(rt)
	  attachPlatform(rt)
	   return rt, nil
   }

// OpenMem 以内存账本、钱包、锁与总线装配运行时（无持久化存储）。

  func OpenMem() (*Runtime, error) {
	  LoadEnvFile(".env")
	  cfg, err := config.Load()
	   if err != nil {
		  return nil, err
	   }
	  bindMemRuntime(cfg)
	  log := logx.New(cfg.LogLevel)
	  enc, err := cryptoenc.Open(cfg)
	   if err != nil {
		   enc = nil
	   }
	   mem := ledger.NewMemStore()
	  ledger.SeedBooks(mem)
	  books := ledger.BookAdapter{S: mem}
	   wallets := wallet.NewService(cfg, nil, books, wallet.NewMemRepo(), enc, wallet.NewMemHistory())
	   risk, err := ledger.NewRisk(cfg.Risk)
	  if err != nil {
		   return nil, err
	  }
	  lock := &ledger.MutexLocker{}
	   led := ledger.NewService(mem,
		   risk, lock, enc, ChainBridge{W: wallets}, log, cfg.Transfer)
	   screen, err := kyt.NewScreener(cfg.KYT)
	   if err != nil {
		  return nil, err
	  }
	   led.SetKYT(screen)
	  led.SetDepositAddress(wallets.AddressFor)
	   b := bus.NewMemory()
	   mkt := market.New(b, nil, cfg, log)
	   mkt.SeedTicks()
	  sw := swap.New(mem, lock, swap.CachePricer{
		  Live: func(from, to string) (decimal.Decimal, bool) {
			   return mkt.QuoteRate(from, to, cfg.Swap.QuoteMaxAge)
		   },
		   Has: mkt.HasPair,
	   }, cfg.Swap)
	   sw.Guard = led.GuardUser
	   sw.Submit = ChainBridge{W: wallets}.Submit
	  sp := spot.New(mem, lock, swap.CachePricer{
		   Live: func(from, to string) (decimal.Decimal, bool) {
			   return mkt.QuoteRate(from, to, cfg.Swap.QuoteMaxAge)
		  },
		   Has: mkt.HasPair,
	  }, cfg.Spot)
	  sp.Guard = led.GuardUser
	   sp.OnPrint = func(symbol, price string) {
		  mkt.ApplyTick(market.Tick{Symbol: symbol, Price: price, Ts: time.Now().UnixMilli()})
	   }
	  rt := &Runtime{
		  Cfg: cfg, Log: log, Wallet: wallets, Ledger: led, Swap: sw, Spot: sp, Market: mkt,
		  Store: mem, KV: persist.NewMemKV(), Bus: b, Enc: enc, Mem: true,
		  Auth: authn.NewFrom(cfg.Auth),
		  reconCh: make(chan string, 64),
	   }
	   onBooks := func(ctx context.Context, userID string) {
		   wallets.Invalidate(ctx, userID)
		  rt.hintReconcile(userID)
	   }
	   led.OnBooks = onBooks
	   sw.OnBooks = onBooks
	  sp.OnBooks = onBooks
	  attachDurable(rt)
	  attachPlatform(rt)
	  return rt, nil
  }

// attachPlatform 挂注册中心、配置中心、服务拨号与资金邻接簿。etcd 拨号失败时记日志重试，不伪造成功。

  func attachPlatform(rt *Runtime) {
	  if rt == nil {
		   return
	   }
	  rt.Registry = svcreg.Open(rt.Cfg, rt.Log)
	   rt.CfgWatch = cfgwatch.Open(rt.Cfg, rt.Log)
	  rt.Dial = svcdial.New(rt.Registry, rt.Log)
	  rt.FundMesh = fundmesh.New(rt.Dial, rt.Registry, rt.Cfg, rt.Log)
	   rt.BindMiddlewareAlerts()
	  rt.Users = userdir.NewKV(rt.KV)
	   rt.Exports = exjob.New()
	   rt.Convert = cvfund.New()
	   rt.Isolated = isobook.New()
	   rt.SubKeys = subkey.New(rt.Enc).BindKV(rt.KV)
	  rt.UTA = utabook.New().BindKV(rt.KV)
	  rt.DestAllow = destallow.New()
	  rt.SecLock = seclock.New(rt.Ledger)
	   rt.TradePW = tradepw.New(rt.KV).BindCooldown(rt.SecLock)
	   var wdCombos [][]string
	   if rt.Cfg != nil {
		  wdCombos = config.ParseWithdrawCombos(rt.Cfg.Funds.WithdrawCombos)
	   }
	   rt.WdGate = wdverify.New(rt.KV, rt.TradePW, wdCombos).BindCooldown(rt.SecLock)
	   rt.WdGate.BindAllow(func(ctx context.Context, userID, chain, addr string) error {
		   if rt.DestAllow == nil {
			  return wdverify.ErrAllow
		  }
		  if !rt.DestAllow.Allowed(userID, chain, addr) {
			  return wdverify.ErrAllow
		  }
		  return nil
	  })
	   rt.FundSurf = fundsurf.New(rt.KV, rt.Ledger, rt.Cfg).BindSecurity(rt.SecLock, rt.TradePW, rt.WdGate)
	   rt.Exports.BindKV(rt.KV)
	  rt.Exports.BindLedger(rt.Ledger)
	  if rt.SQL != nil {
		   rt.Exports.BindSQL(rt.SQL)
	   }
	   rt.Convert.BindKV(rt.KV)
	   rt.Convert.BindLedger(rt.Ledger)
	  marks := fxmark.New(rt.KV)
	   staticFX := fxmark.NewStaticFallback(nil)
	  if rt.Cfg != nil {
		  marks.BindQuoteURL(rt.Cfg.Funds.ConvertQuoteURL)
		   if u := strings.TrimSpace(rt.Cfg.Funds.ConvertQuoteURL); u != "" {
			   marks.BindLive(fxmark.NewHTTPFeed(u))
		   }
		  if len(rt.Cfg.Travel.UsdRates) > 0 {
			   staticFX = fxmark.NewStaticFallback(rt.Cfg.Travel.UsdRates)
		   }
		  rt.Convert.BindStrict(authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile), rt.Cfg.Funds.ConvertQuoteTTL)
	  }
	  marks.BindStaticTravel(staticFX)
	  rt.Convert.BindMarks(marks)
	  rt.FXMarks = marks
	   _ = marks.SeedStables(context.Background())
	   if rt.SQL != nil {
		  rt.Isolated.BindSQL(rt.SQL)
		  rt.SubKeys.BindSQL(rt.SQL)
		  rt.UTA.BindSQL(rt.SQL)
		  destallow.BindProd(rt.DestAllow, rt.SQL, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	   }
	  iso := rt.Isolated
	  utabook.SetIsolatedReader(func(userID, symbol string) []isobook.Pair {
		  if iso == nil {
			   return nil
		   }
		   // 只读已开仓对。List 在空户返回内存预览，不能灌进 UTA 负债与账户视图。
		   return iso.ListExisting(userID, symbol)
	  })
   }

// Migrate 对已 Open 的 SQL 连接执行 ApplySchema + ApplyOnce。内存装配拒绝迁移。
   func (rt *Runtime) Migrate() error {
	  if rt == nil || rt.Mem || rt.SQL == nil || rt.SQL.DB == nil {
		  return ErrMigrateMem
	   }
	  ctx := context.Background()
	  if err := rt.SQL.ApplySchema(ctx); err != nil {
		  return err
	   }
	  done, err := persist.ApplyOnce(ctx, rt.SQL.DB, rt.SQL.Dialect, config.FindRepoFile("migrations"))
	   if err != nil {
		   return err
	   }
	   for _, m := range done {
		   rt.Log.Info().Int("version", m.Version).Str("name", m.Name).Msg("migration applied")
	  }
	   return nil
  }

// StartBackground 按进程角色挂探活、行情消费/、worker 循环。

  func (rt *Runtime) StartBackground(ctx context.Context) {
	   p := role.Parse(rt.Cfg.ProcessRole)
	   if p.Probe {
		  rt.Wallet.ProbeLoop(ctx,
			  rt.Cfg.NodeHealth.ProbeInterval)
	   }
	  if p.Consume {
		  rt.Market.Consume(ctx)
	   }
	  if p.Produce {
		   rt.Market.ProduceLoop(ctx, rt.Cfg.Market.ProduceInterval)
	  }
	   if p.Worker {
		   go rt.RunWorker(ctx)
	  }
	  if rt.Exports != nil {
		  rt.Exports.RunExpireLoop(ctx, rt.Log)
	   }
   }

// hintReconcile 入账后丢一个对账 hint，满了就丢，不堵热路径。

   func (rt *Runtime) hintReconcile(userID string) {
	  if rt == nil || rt.reconCh == nil || userID == "" {
		  return
	   }
	  select {
	  case rt.reconCh <- userID:
	   default:
	  }
  }

// RunWorker 确认入账、出箱、对账循环；卡住的冻结单在这里收尾。

  func (rt *Runtime) RunWorker(ctx context.Context) {
	  if !rt.claimWorker(ctx) {
		  return
	  }
	  rt.EnsureFundingTopics(ctx)
	  rt.StartRegistryLeaseGuard(ctx)
	  // 出箱领导泵与 drainOutbox 并存：泵走路由/毒丸/lag；drain 保留原确认路径。
	   rt.StartOutboxLeaderPump(ctx)
	   tick := rt.Cfg.Worker.Tick
	  if tick <= 0 {
		   tick = rt.Cfg.Transfer.MockConfirmAfter
	  }
	  if tick <= 0 {
		  tick = 400 * time.Millisecond
	   }
	   t := time.NewTicker(tick)
	  defer t.Stop()
	  rt.ReconcileUsers(ctx)
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case uid := <-rt.reconCh:
			  rt.reconcileOne(ctx, uid)
		   case <-t.C:
			   rt.workerTick(ctx)
		   }
	   }
   }

   func (rt *Runtime) claimWorker(ctx context.Context) bool {
	   if rt == nil || rt.Redis == nil {
		  return true
	   }
	  for {
		  ok, err := rt.Redis.SetNX(ctx, redisx.KeyWorkerLeader, "1", 8*time.Second).Result()
		  if err == nil && ok {
			   go rt.keepWorker(ctx)
			  return true
		  }
		  select {
		   case <-ctx.Done():
			  return false
		  case <-time.After(2 * time.Second):
		  }
	  }
  }

   func (rt *Runtime) keepWorker(ctx context.Context) {
	  t := time.NewTicker(3 * time.Second)
	   defer t.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   _ = rt.Redis.Del(context.Background(), redisx.KeyWorkerLeader).Err()
			  return
		   case <-t.C:
			  _ = rt.Redis.Expire(ctx, redisx.KeyWorkerLeader, 8*time.Second).Err()
		  }
	  }
   }

// workerTick 一轮：确认已提交、收尾冻结、扫充值、出箱、对账。

  func (rt *Runtime) workerTick(ctx context.Context) {
	  limit := rt.Cfg.Worker.ScanLimit
	  if limit <= 0 {
		  limit = 200
	   }
	   n := rt.Cfg.Worker.ConfirmConcurrency
	   if n <= 0 {
		   n = 8
	   }
	  list, err := rt.Store.ListByState(ctx, ledger.StateChainSubmitted, limit)
	   if err != nil {
		  rt.Log.Warn().Err(err).Msg("list submitted transfers")
	  } else {
		  rt.confirmSubmitted(ctx, list, n)
	   }
	   failed, err := rt.Store.ListByState(ctx, ledger.StateFailed, limit)
	  if err != nil {
		   rt.Log.Warn().Err(err).Msg("list failed transfers")
	  } else {
		   conc.For(ctx, n, failed, func(ctx context.Context, tr *ledger.Transfer) {
			  if err := rt.Ledger.FailAndRollback(ctx, tr.ID, "worker rollback of failed transfer"); err != nil {
				  rt.Log.Debug().Err(err).Str("id", tr.ID).Msg("rollback failed")
			   }
		  })
	  }
	   rt.Ledger.RecoverStuck(ctx)
	   rt.Swap.RecoverFrozen(ctx, rt.Cfg.Transfer.StuckFrozen)
	  if rt.Spot != nil {
		  rt.Spot.FillOpen(ctx)
	   }
	  rt.ScanDeposits(ctx)
	   rt.Ledger.SweepHot(ctx, rt.Cfg.Transfer.HotMax)
	  rt.drainOutbox(ctx)
	  rt.ReconcileUsers(ctx)
	   if rt.FundsRecon != nil {
		  rt.FundsRecon(ctx)
	   } else if rt.Ledger != nil {
		   if _, err := rt.Ledger.SweepStrandedCovers(ctx, limit); err != nil {
			   rt.Log.Warn().Err(err).Msg("cover sweep")
		  }
	   }
	  if rt.QuotaLeaseExpire != nil {
		  rt.QuotaLeaseExpire(ctx)
	  }
	   if rt.TravelFXRefresh != nil {
		   rt.TravelFXRefresh(ctx)
	  }
	   if rt.ReorgSagaTick != nil {
		   rt.ReorgSagaTick(ctx)
	  }
	  if rt.FamMemoScan != nil {
		   rt.FamMemoScan(ctx)
	   }
  }

// ScanDeposits 扫托管地址的已确认入账哈希，补记 wallet_deposit。

  func (rt *Runtime) ScanDeposits(ctx context.Context) {
	  if rt == nil || rt.Wallet == nil || rt.Ledger == nil {
		   return
	  }
	   seen := map[string]struct{}{}
	  users := rt.Wallet.CustodyUsers(ctx)
	  if rt.Cfg != nil && rt.Cfg.SeedUserID != "" {
		  users = append([]string{rt.Cfg.SeedUserID}, users...)
	   }
	   for _, uid := range users {
		   uid = strings.TrimSpace(uid)
		  if uid == "" {
			   continue
		   }
		   if _, ok := seen[uid]; ok {
			   continue
		   }
		  seen[uid] = struct{}{}
		  for _, chain := range rt.Wallet.Chains() {
			  addr := rt.Wallet.AddressFor(ctx, uid, chain)
			  if addr == "" {
				  continue
			  }
			  txs, err := rt.Wallet.InboundConfirmed(ctx, chain, addr)
			  if err != nil {
				   continue
			   }
			   for _, tx := range txs {
				  if tx.Hash == "" || tx.Amount.IsZero() {
					  continue
				  }
				  asset := tx.Asset
				  amt := tx.Amount
				  if obs, err := rt.Wallet.Inspect(ctx, chain, tx.Hash); err == nil && obs.Asset != "" && obs.Amount.IsPositive() {
					  asset = obs.Asset
					  amt = obs.Amount
				  }
				   _, _ = rt.Ledger.Create(ctx, ledger.CreateReq{
					   UserID: uid, Direction: ledger.WalletDeposit, Asset: asset, Amount: amt,
					   Chain: chain, TxHash: tx.Hash, IdempotencyKey: "scan:" + chain + ":" + tx.Hash,
					   WaitConfirm: true,
				   })
			  }
		   }
	   }
  }

// confirmSubmitted 按链批量 Inspect，再 ConfirmAndCredit；内部 hash 直接确认。

   func (rt *Runtime) confirmSubmitted(ctx context.Context, list []*ledger.Transfer, n int) {
	   if len(list) == 0 {
		   return
	   }
	   batch := rt.Cfg.Worker.InspectBatch
	  if batch <= 0 {
		   batch = 32
	  }
	  byChain := map[string][]*ledger.Transfer{}
	  var local []*ledger.Transfer
	  for _, tr := range list {
		  if tr == nil {
			   continue
		   }
		   if tr.TxHash == "" || ledger.IsInternalTxHash(tr.TxHash) || tr.Chain == "" {
			  local = append(local, tr)
			  continue
		   }
		  byChain[tr.Chain] = append(byChain[tr.Chain], tr)
	   }
	   run := func(ctx context.Context, tr *ledger.Transfer) {
		  start := time.Now()
		   err := rt.Ledger.ConfirmAndCredit(ctx, tr.ID)
		  metrics.WorkerConfirm.Observe(time.Since(start).Seconds())
		   if err != nil {
			   if errors.Is(err, ledger.ErrPending) {
				   return
			  }
			   rt.Log.Debug().Err(err).Str("id", tr.ID).Str("trace_id", trace.ID(ctx)).Msg("confirm")
		  }
	  }
	  conc.For(ctx, n, local, run)
	  for chain, group := range byChain {
		   for _, part := range conc.Chunk(group, batch) {
			  hashes := make([]string, 0, len(part))
			   for _, tr := range part {
				  hashes = append(hashes,
					   tr.TxHash)
			   }
			   obs, err := rt.Wallet.InspectBatch(ctx, chain, hashes)
			  if err != nil {
				   rt.Log.Debug().Err(err).Str("chain", chain).Msg("inspect batch")
				  conc.For(ctx, n, part, run)
				   continue
			   }
			   conc.For(ctx, n, part, func(ctx context.Context, tr *ledger.Transfer) {
				   cctx := ctx
				   if o, ok := obs[tr.TxHash]; ok {
					  cctx = wallet.WithObservation(ctx,
						   chain, tr.TxHash, o)
				  }
				   run(cctx, tr)
			   })
		  }
	  }
   }

// 作者: jakson
// drainOutbox 把与账本同一事务写入的出箱事件投到总。
// 总线为内存时不标已投。进程内按事件号去重，进程重启后从出箱重新投出。
// 总线为库表或外部总线时，投出成功才标已投。投出失败严禁标已投。
// 库表写入成功之后、标已投之前若进程退出，出箱会再次投出一条。消费侧必须按聚合号去重。
   func (rt *Runtime) drainOutbox(ctx context.Context) {
	  limit := rt.Cfg.Worker.OutboxLimit
	  if limit <= 0 {
		  limit = 100
	  }
	   list, err := rt.Store.ListOutbox(ctx, true, limit)
	  if err != nil || len(list) == 0 {
		   return
	  }
	  n := 8
	   if rt.Cfg.Worker.ConfirmConcurrency > 0 {
		  n = rt.Cfg.Worker.ConfirmConcurrency
	  }
	   var mu sync.Mutex
	  ids := make([]string,
		   0, len(list))
	   conc.For(ctx, n, list, func(ctx context.Context, e ledger.OutboxEvent) {
		   b, err := json.Marshal(e)
		   if err != nil {
			   return
		  }
		  if rt.Bus != nil && rt.Bus.Kind() == "memory" {
			  if _, loaded := rt.outboxLive.LoadOrStore(e.ID, struct{}{}); loaded {
				  return
			  }
		  }
		  if pubErr := rt.Bus.Publish(ctx, events.TopicMovements, []byte(e.AggregateID), b); pubErr != nil {
			  rt.outboxLive.Delete(e.ID)
			  rt.Log.Debug().Err(pubErr).Str("id", e.ID).Msg("outbox publish")
			  return
		   }
		  if rt.Bus != nil && rt.Bus.Kind() == "memory" {
			  return
		  }
		  mu.Lock()
		  ids = append(ids, e.ID)
		  mu.Unlock()
		   metrics.OutboxEvents.WithLabelValues("published").Inc()
	  })
	  if err := rt.Store.MarkOutboxPublished(ctx, ids); err != nil {
		   rt.Log.Debug().Err(err).Msg("outbox mark")
	   }
  }

// InflightFrozen 汇总 Swap/现货在途占用，对账时加进冻结侧。

   func (rt *Runtime) InflightFrozen(ctx context.Context, userID string) (map[string]decimal.Decimal, []ledger.Mismatch, error) {
	   extra, mm, err := rt.Swap.FrozenInflight(ctx, userID)
	  if err != nil {
		   return nil, nil, err
	   }
	  if rt.Spot != nil {
		  e2, m2, err := rt.Spot.FrozenInflight(ctx, userID)
		   if err != nil {
			  return nil, nil, err
		  }
		   extra = ledger.AddFrozen(extra, e2)
		  mm = append(mm, m2...)
	   }
	   if rt.KV != nil {
		   if rows, err := holdcat.New(rt.KV).List(ctx, userID, true); err == nil && len(rows) > 0 {
			   holds := make([]ledger.ParkedHold, 0, len(rows))
			  for _, r := range rows {
				   if !holdcat.ParkedOnBridge(r.Reason) {
					   continue
				   }
				   holds = append(holds, ledger.ParkedHold{
					  Asset: r.Asset, Book: r.Book, Reason: r.Reason, Ref: r.Ref, Amount: r.Amount,
				   })
			  }
			   mm = append(mm, ledger.HoldcatMismatches(ctx, rt.Store, userID, holds)...)
		   }
	  }
	   return extra, mm, nil
   }

// reconcileOne 对一个用户：在途冻结 vs 两本账，对不平拦住新划转。
  func (rt *Runtime) reconcileOne(ctx context.Context, userID string) {
	  extra, mm, err := rt.InflightFrozen(ctx, userID)
	  if err != nil {
		   rt.Log.Warn().Err(err).Str("user", userID).Msg("swap inflight")
		  return
	  }
	   rep,
		  err := ledger.ReconcileWith(ctx, rt.Store, userID, extra)
	  if err != nil {
		  rt.Log.Warn().Err(err).Str("user", userID).Msg("reconcile")
		  return
	  }
	  ledger.MergeMismatches(rep, mm)
	  rt.Ledger.NoteReconcile(ctx, rep)
	  if !rep.OK {
		   rt.Log.Warn().Str("user", userID).Int("mismatches", len(rep.Mismatches)).Msg("reconcile mismatch; blocking new movements")
	   }
   }

// Connectivity 进程连通性与资金链路快照，供排障。
   func (rt *Runtime) Connectivity() map[string]any {
	   role := rt.Cfg.ProcessRole
	  if role == "" {
		  role = "all"
	   }
	   return map[string]any{
		  "disclaimer":   slo.ProcessDisclaimer,
		  "process_role": role,
		   "ids": map[string]string{
			   "trace_id": "X-Trace-Id / ctx span",
			  "movement_id":  "transfer.id or swap.order.id",
			  "submit_nonce": "same as movement_id via wallet.WithSubmitNonce",
			   "user_lock": "lock:user:{user_id}",
			  "chain_tx": "(chain, tx_hash) unique for on-chain rows",
		   },
		   "chains": []string{
			   "wallet create/import → addresses → ledger allowlist / withdraw dest",
			   "ledger freeze → wallet Submit/Observe (RPC outside tx, chain timeout) → worker ConfirmAndCredit",
			   "swap freeze → same ChainBridge Submit → persist hash → settle or async RecoverFrozen",
			  "market ticks → QuoteRate (memory then Redis) → swap execRate",
			   "node failover + hedged reads Registry wraps Submit/Observe/Status/InspectBatch",
			  "outbox same-tx → wallet.movements bus → drainOutbox (bounded publish)",
			   "OnBooks invalidate L1+Redis + reconcile hint",
		  },
		  "limits": map[string]string{
			   "worker_pool": "bounded goroutines in-process; not a partitioned job queue",
			  "outbox":      "poll + mark published; no relay HA",
			  "sql_pool":      "configurable max_open/idle; not per-shard routing",
			  "chain_timeout": "write path uses chain_rpc_timeout",
			   "process_role":  "api skips produce+worker so marketd/worker can run separately",
			   "fencing_token": "lease token persisted as fence_epoch/fence_token on transfer and swap rows",
			  "kms": "DEK unwrapped per op, never a struct field",
			   "reorg":   "confirmation depth + unwind credited deposits",
			  "kyt":     "fail-closed screen before withdraw broadcast",
			   "kyc":     "none/basic/advanced on user_controls",
			  "indexer": "ScanDeposits reads inbound history",
			   "hot_cold": "system.bridge excess to system.cold",
			  "hosted_hsm": "per-op encrypt/decrypt/sign over configured keeper",
			   "inspect_batch": "worker groups submitted hashes per chain; ConfirmAndCredit reuses ctx observation",
			  "hedged_reads": "two endpoints raced for balance/history/inspect; submit stays failover-only",
			  "l1_cache":     "process TTL + singleflight in front of Redis bal:{user}:{chain}",
			   "pprof":        "loopback RemoteAddr only when pprof: true",
		  },
	  }
  }

// ReconcileUsers 扫在途单用户做防漂对账；报告脏了就拦住新动作。
  func (rt *Runtime) ReconcileUsers(ctx context.Context) {
	   users := map[string]struct{}{rt.Cfg.SeedUserID: {}}
	   for _, st := range []ledger.State{
		  ledger.StateSpotFrozen, ledger.StateWalletLocked,
		   ledger.StateChainSubmitted, ledger.StateConfirmed, ledger.StateFailed,
		  ledger.StateHold, ledger.StateReorged,
	   } {
		  list, err := rt.Store.ListByState(ctx, st, 200)
		   if err != nil {
			   rt.Log.Warn().Err(err).Str("state", string(st)).Msg("reconcile list")
			   continue
		  }
		   for _, tr := range list {
			   users[tr.UserID] = struct{}{}
		  }
	  }
	  uids := make([]string, 0, len(users))
	  for u := range users {
		  uids = append(uids, u)
	  }
	  n := rt.Cfg.Worker.ConfirmConcurrency
	  if n <= 0 {
		  n = 8
	  }
	   conc.For(ctx, n, uids, func(ctx context.Context, u string) {
		  rt.reconcileOne(ctx, u)
	  })
  }

// Ready 依赖探活：MySQL/Redis 挂了不就绪；单条链 RPC 挂了仍可降级。
// 且拒种子 Ensure 时，FamMEMO 共享地址零登记 → 503。
// 就绪面暴露 missing 清单；有登记但未覆盖全链记 degraded。
// 零登记不得假装可充 FamMEMO；OpenMem 仍装配。
   func (rt *Runtime) Ready(ctx context.Context) (body map[string]any, code int) {
	  code = 200
	   mysql := "ok"
	  if !rt.Mem && rt.Enc == nil {
		  code = 503
	   }
	  if rt.Mem {
		   mysql = "mem"
	  } else if pinger, ok := rt.Store.(interface{ Ping(context.Context) error }); ok {
		   if err := pinger.Ping(ctx); err != nil {
			  mysql = err.Error()
			   code = 503
		   }
	   }
	  redis := "ok"
	   if rt.Mem {
		   redis = "mem"
	   } else if rt.Redis != nil {
		  if err := rt.Redis.Ping(ctx).Err(); err != nil {
			  redis = err.Error()
			   code = 503
		  }
	   }
	  down := rt.Wallet.Registry().Health().DownCount()
	  degraded := down > 0
	  famReg := 0
	   famMissing := []string{}
	   famZero := false
	  type famSharedReady interface {
		  RegisteredCount() int
		   MissingFamChains() []string
		  SeedEnsureRefused() bool
	  }
	   if rt != nil && rt.SharedMemo != nil {
		   if pool, ok := rt.SharedMemo.(famSharedReady); ok && pool != nil {
			  famReg = pool.RegisteredCount()
			   famMissing = pool.MissingFamChains()
			  if pool.SeedEnsureRefused() && famReg == 0 {
				   famZero = true
				   if rt.Cfg != nil && authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile) && !rt.Mem {
					   code = 503
				  }
			   }
			  if len(famMissing) > 0 && famReg > 0 {
				   degraded = true
			   }
		  }
	   } else if rt != nil && rt.Cfg != nil && authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile) && !rt.Mem {
		  // 未挂共享池：视为零登记
		  famZero = true
		   famMissing = wallet.FamMemoChains()
		   code = 503
	   }
	  body = map[string]any{
		  "ok":       code == 200,
		  "degraded": degraded,
		   "mysql": mysql,
		   "redis":      redis,
		   "nodes_down": down,
		  "fam_shared_registered": famReg,
		  "fam_shared_missing":    famMissing,
		  "fam_shared_zero": famZero,
		   "service":         "account-assets",
		  "slo":        "wallet-core availability design target " + slo.AvailabilityTarget,
		   "disclaimer": slo.ProcessDisclaimer,
	   }
	  return body, code
  }

// SetPublicHTTP 记下本进程 HTTP 路由。内部 gRPC 收到调用后仍进这些路由。
func (rt *Runtime) SetPublicHTTP(h http.Handler) {
	if rt == nil || h == nil {
		return
	}
	rt.pubHTTP.Store(h)
}

func (rt *Runtime) servePublicHTTP(w http.ResponseWriter, r *http.Request) {
	if rt == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	h, _ := rt.pubHTTP.Load().(http.Handler)
	if h == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	h.ServeHTTP(w, r)
}

// ServeGRPC 挂钱包/划转/兑换，并挂内部调用。内部调用进入本进程 HTTP 路由。
  func (rt *Runtime) ServeGRPC(ctx context.Context) error {
	   lis, err := net.Listen("tcp", rt.Cfg.GRPCAddr)
	  if err != nil {
		  return err
	  }
	   s := grpc.NewServer(grpc.UnaryInterceptor(trace.Unary(rt.Log)))
	  walletv1.RegisterWalletServiceServer(s,
		   grpcsvc.New(rt.Wallet, rt.Ledger, rt.Swap))
	  meshgrpc.Mount(s, http.HandlerFunc(rt.servePublicHTTP))
	   go func() {
		   <-ctx.Done()
		  s.GracefulStop()
	  }()
	  rt.Log.Info().Str("addr", rt.Cfg.GRPCAddr).Msg("gRPC wallet/ledger/swap")
	  return s.Serve(lis)
   }
