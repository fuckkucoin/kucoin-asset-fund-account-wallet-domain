// 职责：
// 1. 从配置（JSON 文件 / FEE_ORACLE_CONFIG 环境变量）加载 oracle 源列表；
// 2. 对每个活跃源启动独立采集 goroutine，按源的 poll_sec 间隔轮询目标 HTTP 端点；
// 3. 解析链费率 tick，调用 feeoracle.Normalize 规范化，再调用
// feeoracle.Service.PushTick 写入聚合层；
// 4. 将 tick 通过 HTTP 转发到 fee-service 的 /v1/feeoracle/ticks 端点；
// 5. 周期执行过期报价代次清理（ExpireOld）；
// 6. 暴露 Prometheus metrics（:18091/metrics）和 /health 探活；
// 7. 监听 SIGINT/SIGTERM 优雅退出。
// 此进程不参与出金广播，不写入账本，不修改 conserve FSM。

// Author: Blaise
// Date: 2021.04.27

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/feeoracle"
	"account-assets.kucoin.net/funding/internal/logx"
)

// workerConfig 采集 worker 配置。通过 JSON 文件或 FEE_ORACLE_CONFIG 环境变量加载。
type workerConfig struct {
	// HTTPAddr worker 自身监听地址（Prometheus metrics + /health）。
	HTTPAddr string `json:"http_addr"`
	// FeeServiceAddr 目标 fee-service 地址，用于 HTTP 推送 tick；留空则跳过转发。
	FeeServiceAddr string `json:"fee_service_addr"`
	// Sources oracle 源配置列表。
	Sources []sourceWorkerConfig `json:"sources"`
	// ExpireIntervalSec 过期代次清理间隔秒数；0 使用默认 300 秒。
	ExpireIntervalSec int `json:"expire_interval_sec"`
	// LogLevel 日志级别（debug/info/warn/error）。
	LogLevel string `json:"log_level"`
}

// sourceWorkerConfig 单个采集源配置（叠加在 feeoracle.SourceConfig 之上）。
type sourceWorkerConfig struct {
	feeoracle.SourceConfig
	// PollSec 轮询间隔秒数；0 使用默认 30 秒。
	PollSec int `json:"poll_sec"`
	// TimeoutSec HTTP 请求超时秒数；0 使用默认 10 秒。
	TimeoutSec int `json:"timeout_sec"`
}

// --- Prometheus metrics ---

var (
	metricTickPush = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name:      "tick_push_total",
			Help: "Total tick pushes to oracle, labelled by source, chain and result.",
		},
		[]string{"source_id", "chain", "result"},
	)
	metricTickAgeSec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "fee_oracle",
			Name: "tick_age_seconds",
			Help: "Age in seconds of the last tick successfully received per source.",
		},
		[]string{"source_id", "chain"},
	)
	metricFetchErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "fetch_error_total",
			Help: "Total HTTP fetch errors per source.",
		},
		[]string{"source_id", "chain"},
	)
	metricVersionsExpired = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name:      "version_expired_total",
			Help:      "Total QuoteVersion rows expired by the cleanup cron.",
		},
	)
)

   func init() {
	  prometheus.MustRegister(metricTickPush, metricTickAgeSec, metricFetchErrors, metricVersionsExpired)
  }

  func main() {
	  cfgFile := flag.String("config", "", "JSON config file path (or FEE_ORACLE_CONFIG env)")
	   flag.Parse()
	   path := strings.TrimSpace(*cfgFile)
	  if path == "" {
		   path = strings.TrimSpace(os.Getenv("FEE_ORACLE_CONFIG"))
	   }
	   cfg := loadConfig(path)
	  log := logx.New(cfg.LogLevel)
	   log.Info().Str("http_addr", cfg.HTTPAddr).Msg("fee-oracle-worker starting")
	   // 内存 oracle（本进程持有；tick 同步到 fee-service via HTTP）。
	   o := feeoracle.NewOracle(500)
	  svc := feeoracle.New(o, feeoracle.NewVersionStore(nil), nil, nil)
	   for i := range cfg.Sources {
		   src := cfg.Sources[i]
		  if !src.Enabled {
			  continue
		   }
		  if err := o.RegisterSource(src.SourceConfig); err != nil {
			   log.Warn().Err(err).Str("id", src.ID).Msg("source registration failed; skipped")
		  }
	   }
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	  var wg sync.WaitGroup
	  for i := range cfg.Sources {
		   src := cfg.Sources[i]
		  if !src.Enabled {
			  continue
		   }
		  wg.Add(1)
		  go func(s sourceWorkerConfig) {
			   defer wg.Done()
			   runPoller(ctx, svc, s, cfg.FeeServiceAddr, log)
		   }(src)
	   }
	   expireInterval := time.Duration(cfg.ExpireIntervalSec) * time.Second
	   if expireInterval <= 0 {
		  expireInterval = 5 * time.Minute
	  }
	  wg.Add(1)
	  go func() {
		  defer wg.Done()
		   runExpireCron(ctx, svc, expireInterval, log)
	   }()
	   router := gin.New()
	  router.Use(gin.Recovery())
	  router.GET("/health", func(c *gin.Context) {
		   h := o.HealthReport()
		  code := http.StatusOK
		  if !h.Healthy {
			   code = http.StatusServiceUnavailable
		   }
		   c.JSON(code, h)
	  })
	   router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	  addr := cfg.HTTPAddr
	  if addr == "" {
		  addr = ":18091"
	   }
	   srv := &http.Server{Addr: addr, Handler: router}
	  wg.Add(1)
	   go func() {
		   defer wg.Done()
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Error().Err(err).Msg("http server error")
		  }
	  }()
	   <-ctx.Done()
	  log.Info().Msg("fee-oracle-worker shutting down")
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
	   wg.Wait()
	   log.Info().Msg("fee-oracle-worker stopped")
   }

// runPoller 单源采集循环。
  func runPoller(
	   ctx context.Context,
	  svc *feeoracle.Service,
	   src sourceWorkerConfig,
	  feeServiceAddr string,
	   log zerolog.Logger,
  ) {
	  pollInterval := time.Duration(src.PollSec) * time.Second
	  if pollInterval <= 0 {
		  pollInterval = 30 * time.Second
	   }
	   httpTimeout := time.Duration(src.TimeoutSec) * time.Second
	   if httpTimeout <= 0 {
		   httpTimeout = 10 * time.Second
	  }
	   client := &http.Client{Timeout: httpTimeout}
	   ticker := time.NewTicker(pollInterval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			   tick, err := fetchTick(ctx, client, src)
			  if err != nil {
				   metricFetchErrors.WithLabelValues(src.ID, src.Chain).Inc()
				  log.Debug().Err(err).Str("source", src.ID).Msg("fetch tick failed")
				  continue
			  }
			  if err := svc.PushTick(src.ID, tick); err != nil {
				  metricTickPush.WithLabelValues(src.ID, src.Chain, "push_error").Inc()
				   log.Debug().Err(err).Str("source", src.ID).Msg("push tick failed")
				  continue
			   }
			   metricTickPush.WithLabelValues(src.ID, src.Chain, "ok").Inc()
			   metricTickAgeSec.WithLabelValues(src.ID, src.Chain).Set(0)
			   if strings.TrimSpace(feeServiceAddr) != "" {
				   go forwardTick(ctx,
					   client, feeServiceAddr, src.ID, tick, log)
			  }
		  }
	   }
  }

// tickHTTPResponse 约定的 fee 源 HTTP 端点响应体。
type tickHTTPResponse struct {
	Chain  string `json:"chain"`
	Family string `json:"family"`
	// UTXO
	FeeRateSatPerVByte string `json:"fee_rate_sat_per_vbyte,omitempty"`
	EstimatedVBytes    int64  `json:"estimated_vbytes,omitempty"`
	// EVM
	BaseFeeGwei string `json:"base_fee_gwei,omitempty"`
	PriorityFeeGwei string `json:"priority_fee_gwei,omitempty"`
	GasLimit int64 `json:"gas_limit,omitempty"`
	// TRON
	EnergyUnits int64 `json:"energy_units,omitempty"`
	EnergyPriceSun    string `json:"energy_price_sun,omitempty"`
	BandwidthBytes    int64  `json:"bandwidth_bytes,omitempty"`
	BandwidthPriceSun string `json:"bandwidth_price_sun,omitempty"`
	// SOL
	BaseLamports       int64 `json:"base_lamports,omitempty"`
	ComputeUnits       int64 `json:"compute_units,omitempty"`
	MicroLamportsPerCU int64 `json:"micro_lamports_per_cu,omitempty"`
	// Generic
	NativeFee string `json:"native_fee,omitempty"`
	NativeAsset string `json:"native_asset,omitempty"`
}

// fetchTick 从 HTTP 端点获取并规范化一个 tick。

  func fetchTick(ctx context.Context, client *http.Client, src sourceWorkerConfig) (feeoracle.ChainFeeTick, error) {
	   if strings.TrimSpace(src.Endpoint) == "" {
		  return feeoracle.ChainFeeTick{}, fmt.Errorf("source %s: endpoint empty", src.ID)
	   }
	  req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.Endpoint, nil)
	  if err != nil {
		   return feeoracle.ChainFeeTick{}, err
	   }
	  resp, err := client.Do(req)
	   if err != nil {
		   return feeoracle.ChainFeeTick{}, err
	  }
	   defer resp.Body.Close()
	  if resp.StatusCode != http.StatusOK {
		   return feeoracle.ChainFeeTick{}, fmt.Errorf("source %s: HTTP %d", src.ID, resp.StatusCode)
	   }
	  body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	   if err != nil {
		  return feeoracle.ChainFeeTick{}, err
	  }
	   var raw tickHTTPResponse
	  if err := json.Unmarshal(body, &raw); err != nil {
		   return feeoracle.ChainFeeTick{}, fmt.Errorf("source %s: unmarshal: %w", src.ID, err)
	   }
	  family, err := feeoracle.ParseFamily(raw.Family)
	  if err != nil {
		  family = src.Family
	   }
	  now := time.Now().UTC()
	   input := buildInput(raw, src.Chain, family)
	   norm, err := feeoracle.Normalize(input)
	   if err != nil {
		  return feeoracle.ChainFeeTick{}, fmt.Errorf("source %s: normalize: %w", src.ID, err)
	  }
	  return feeoracle.ChainFeeTick{
		   SourceID: src.ID,
		   Chain: src.Chain,
		  Family:     family,
		  Input:      input,
		   Normalized: norm,
		  TickedAt:   now,
		   RecordedAt: now,
	  }, nil
   }

// buildInput 从 HTTP 响应体构造 ChainFeeInput（字符串→decimal 精度安全）。
  func buildInput(raw tickHTTPResponse, chain string, family feeoracle.Family) feeoracle.ChainFeeInput {
	   inp := feeoracle.ChainFeeInput{
		   Family: family,
		  Chain:  chain,
		   NativeAsset: strings.ToUpper(strings.TrimSpace(raw.NativeAsset)),
	   }
	   parse := func(s string) decimal.Decimal {
		   d, err := decimal.NewFromString(strings.TrimSpace(s))
		   if err != nil {
			   return decimal.Zero
		  }
		  return d
	   }
	   switch family {
	   case feeoracle.FamilyUTXO:
		   inp.FeeRateSatPerVByte = parse(raw.FeeRateSatPerVByte)
		  inp.EstimatedVBytes = raw.EstimatedVBytes
		  if inp.EstimatedVBytes == 0 {
			  inp.EstimatedVBytes = 141
		   }
	  case feeoracle.FamilyEVM:
		  inp.BaseFeeGwei = parse(raw.BaseFeeGwei)
		  inp.PriorityFeeGwei = parse(raw.PriorityFeeGwei)
		  inp.GasLimit = raw.GasLimit
		   if inp.GasLimit == 0 {
			   inp.GasLimit = 65000
		   }
	   case feeoracle.FamilyTRON:
		   inp.EnergyUnits = raw.EnergyUnits
		   inp.EnergyPriceSun = parse(raw.EnergyPriceSun)
		   inp.BandwidthBytes = raw.BandwidthBytes
		  inp.BandwidthPriceSun = parse(raw.BandwidthPriceSun)
	   case feeoracle.FamilySOL:
		   inp.BaseLamports = raw.BaseLamports
		  if inp.BaseLamports == 0 {
			   inp.BaseLamports = 5000
		   }
		   inp.ComputeUnits = raw.ComputeUnits
		   inp.MicroLamportsPerCU = raw.MicroLamportsPerCU
	   case feeoracle.FamilyGeneric:
		   inp.NativeFee = parse(raw.NativeFee)
	   }
	  return inp
   }

// forwardTick POST tick 到 fee-service；失败仅记 metric，不阻塞采集循环。
  func forwardTick(ctx context.Context, client *http.Client, addr, sourceID string, tick feeoracle.ChainFeeTick, log zerolog.Logger) {
	   payload, err := json.Marshal(map[string]any{
		   "source_id": sourceID,
		   "tick": tick,
	  })
	  if err != nil {
		   return
	  }
	  url := strings.TrimRight(addr, "/") + "/v1/feeoracle/ticks"
	   req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	   if err != nil {
		   return
	   }
	  req.Header.Set("Content-Type", "application/json")
	   resp, err := client.Do(req)
	   if err != nil {
		   metricTickPush.WithLabelValues(sourceID, tick.Chain, "forward_error").Inc()
		  log.Debug().Err(err).Str("source", sourceID).Msg("forward tick error")
		   return
	  }
	  resp.Body.Close()
	  if resp.StatusCode != http.StatusOK {
		  metricTickPush.WithLabelValues(sourceID, tick.Chain, "forward_http_error").Inc()
	  }
  }

// runExpireCron 定期清理过期的 QuoteVersion。

   func runExpireCron(ctx context.Context, svc *feeoracle.Service, interval time.Duration, log zerolog.Logger) {
	  t := time.NewTicker(interval)
	  defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-t.C:
			   n, err := svc.ExpireOld(ctx)
			  if err != nil {
				   log.Debug().Err(err).Msg("expire old versions")
				   continue
			  }
			   if n > 0 {
				   metricVersionsExpired.Add(float64(n))
				  log.Info().Int("count", n).Msg("expired old quote versions")
			   }
		  }
	   }
  }

// loadConfig 从 JSON 文件或默认值加载配置。
  func loadConfig(path string) workerConfig {
	  defaults := workerConfig{
		  HTTPAddr:          ":18091",
		   ExpireIntervalSec: 300,
		   LogLevel:          "info",
	   }
	   if path == "" {
		  return defaults
	   }
	   data, err := os.ReadFile(path)
	  if err != nil {
		  return defaults
	  }
	  var cfg workerConfig
	   if err := json.Unmarshal(data, &cfg); err != nil {
		   return defaults
	   }
	   if cfg.HTTPAddr == "" {
		   cfg.HTTPAddr = defaults.HTTPAddr
	   }
	  if cfg.ExpireIntervalSec == 0 {
		   cfg.ExpireIntervalSec = defaults.ExpireIntervalSec
	   }
	  if cfg.LogLevel == "" {
		  cfg.LogLevel = defaults.LogLevel
	  }
	  return cfg
   }
