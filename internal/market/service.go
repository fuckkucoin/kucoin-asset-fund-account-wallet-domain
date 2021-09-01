// 行情 tick 和 K 线。
// 作者: Likeo
// 日期: 2021.08.31
package market

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
	"github.com/gorilla/websocket"
	"account-assets.kucoin.net/funding/internal/bus"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/slo"
)

// Tick 现货成交价。钱包行情和兑换报价共用这份缓存，进程内不再另开一本价。

type Tick struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
	ChangeBps int `json:"change_bps"`
	Ts int64 `json:"ts"`
	Seq uint64 `json:"seq"`
}

type Candle struct {
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	Open     string `json:"open"`
	High     string `json:"high"`
	Low      string `json:"low"`
	Close    string `json:"close"`
	StartTs int64  `json:"start_ts"`
	Seq     uint64 `json:"seq"`
}

type Envelope struct {
	Type string `json:"type"`
	Ticker *Tick `json:"ticker,omitempty"`
	Kline *Candle `json:"kline,omitempty"`
}

type Hub struct {
	mu sync.RWMutex
	clients map[*websocket.Conn]struct{}
	log     zerolog.Logger
}

// NewHub WebSocket 订阅集合，ticker/K 线从这里推给钱包页。

  func NewHub(log zerolog.Logger) *Hub {
	   return &Hub{clients: map[*websocket.Conn]struct{}{}, log: log}
  }

   func (h *Hub) Add(c *websocket.Conn) {
	   h.mu.Lock()
	   h.clients[c] = struct{}{}
	   h.mu.Unlock()
  }

  func (h *Hub) Remove(c *websocket.Conn) {
	  h.mu.Lock()
	  delete(h.clients, c)
	  h.mu.Unlock()
	   _ = c.Close()
  }

// BroadcastJSON 推给所有已连上的钱包页；写失败的连接丢掉。
  func (h *Hub) BroadcastJSON(v any) {
	   b, _ := json.Marshal(v)
	  h.mu.RLock()
	   defer h.mu.RUnlock()
	   for c := range h.clients {
		  _ = c.SetWriteDeadline(time.Now().Add(800 * time.Millisecond))
		  if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
			  go h.Remove(c)
		   }
	  }
  }

type Service struct {
	bus bus.Bus
	rdb        *redis.Client
	hub        *Hub
	log        zerolog.Logger
	topic      string
	klineTopic string
	ttl        time.Duration
	mu      sync.RWMutex
	last    map[string]Tick
	candles map[string][]Candle // symbol -> recent 1m
	openBar map[string]Candle
	seq     uint64
	up      websocket.Upgrader
	lagWarn time.Duration
	applied   atomic.Uint64
	stale     atomic.Uint64
	lastLagNs atomic.Int64
	klineKeep int
}

// New 行情服务： → 总线 → 内存/Redis → WS。热路径不走 SQL。
   func New(b bus.Bus, rdb *redis.Client, cfg *config.Config, log zerolog.Logger) *Service {
	  keep := cfg.Market.KlineKeep
	  if keep <= 0 {
		  keep = 120
	   }
	   return &Service{
		  bus: b, rdb: rdb, hub: NewHub(log), log: log,
		   topic: cfg.Market.TickTopic, klineTopic: cfg.Market.KlineTopic, ttl: cfg.Cache.TickerTTL,
		  last: map[string]Tick{}, candles: map[string][]Candle{}, openBar: map[string]Candle{},
		   up: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		  lagWarn:   cfg.Market.LagWarn,
		  klineKeep: keep,
	   }
   }

   func (s *Service) Hub() *Hub { return s.hub }

// Cache 当前 ticker 快照，给钱包页和 HTTP /market/ticker。
  func (s *Service) Cache() map[string]Tick {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make(map[string]Tick, len(s.last))
	  for k, v := range s.last {
		   out[k] = v
	   }
	  return out
  }

// Klines 1 分钟 K 线历史 + 未收盘 bar。
  func (s *Service) Klines(symbol string) []Candle {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   src := s.candles[symbol]
	  out := make([]Candle, len(src))
	   copy(out, src)
	   if c, ok := s.openBar[symbol]; ok {
		   out = append(out, c)
	   }
	  return out
   }

// SeedTicks 给内存装配写入初始行情，钱包页和兑换共用同一套价。
   func (s *Service) SeedTicks() {
	  now := time.Now().UnixMilli()
	   for i, t := range []Tick{
		  {Symbol: "BTC-USDT", Price: "64000"},
		   {Symbol: "ETH-USDT", Price: "3200"},
		  {Symbol: "SOL-USDT", Price: "140"},
		  {Symbol: "TRX-USDT", Price: "0.12"},
		  {Symbol: "TON-USDT", Price: "5.5"},
	   } {
		  t.Ts = now
		  t.Seq = uint64(i + 1)
		  s.ApplyTick(t)
	   }
  }

// HasPair 现货是否有这对 ticker，兑换询价前先问这里。
  func (s *Service) HasPair(from, to string) bool {
	  s.mu.RLock()
	   _, ok := s.last[from+"-"+to]
	  _, inv := s.last[to+"-"+from]
	  s.mu.RUnlock()
	   if ok || inv {
		  return true
	  }
	  if _, ok := s.tickFromRedis(from + "-" + to); ok {
		   return true
	  }
	  if _, ok := s.tickFromRedis(to + "-" + from); ok {
		  return true
	   }
	   return false
  }

// Rate 读现货价，maxAge 用默认滞后阈值。
  func (s *Service) Rate(from, to string) (decimal.Decimal, bool) {
	  return s.QuoteRate(from, to, 0)
  }

// QuoteRate 兑换安全读价：缺 tick、解析失败或过期都返回 false，禁止把陈旧价锁进成交。
   func (s *Service) QuoteRate(from, to string, maxAge time.Duration) (decimal.Decimal, bool) {
	   if maxAge <= 0 {
		  maxAge = s.lagWarn
		   if maxAge <= 0 {
			   maxAge = slo.MarketPushTarget
		  }
		   maxAge *= 2
	  }
	  s.mu.RLock()
	   t, ok := s.last[from+"-"+to]
	   inv, invOK := s.last[to+"-"+from]
	   s.mu.RUnlock()
	   fresh := func(tk Tick) bool {
		   if tk.Ts <= 0 {
			  return true
		   }
		  age := time.Since(time.UnixMilli(tk.Ts))
		  if age < 0 {
			   age = 0
		   }
		   return age <= maxAge
	  }
	   if ok {
		  if !fresh(t) {
			  return decimal.Zero, false
		  }
		   d, err := decimal.NewFromString(t.Price)
		   return d, err == nil && !d.IsZero()
	  }
	   if invOK {
		   if !fresh(inv) {
			   return decimal.Zero, false
		   }
		   d, err := decimal.NewFromString(inv.Price)
		  if err != nil || d.IsZero() {
			  return decimal.Zero, false
		   }
		   return decimal.NewFromInt(1).Div(d), true
	   }
	   if t, ok := s.tickFromRedis(from + "-" + to); ok && fresh(t) {
		  d, err := decimal.NewFromString(t.Price)
		   return d, err == nil && !d.IsZero()
	   }
	  if inv, ok := s.tickFromRedis(to + "-" + from); ok && fresh(inv) {
		   d, err := decimal.NewFromString(inv.Price)
		  if err != nil || d.IsZero() {
			  return decimal.Zero, false
		  }
		  return decimal.NewFromInt(1).Div(d), true
	   }
	   return decimal.Zero, false
   }

  func (s *Service) tickFromRedis(symbol string) (Tick, bool) {
	   if s == nil || s.rdb == nil {
		   return Tick{}, false
	   }
	  b, err := s.rdb.Get(context.Background(), "ticker:"+symbol).Bytes()
	  if err != nil || len(b) == 0 {
		   return Tick{}, false
	  }
	  var t Tick
	   if json.Unmarshal(b, &t) != nil || t.Price == "" {
		  return Tick{}, false
	  }
	   return t, true
   }

// ingestTick 热路径：总线 → 内存/Redis。过期/乱序 tick 丢掉，避免兑换读到回退价。

   func (s *Service) ingestTick(t Tick, broadcastTicker bool) *Candle {
	  if t.Ts > 0 {
		  lag := time.Since(time.UnixMilli(t.Ts))
		   if lag < 0 {
			  lag = 0
		   }
		  s.lastLagNs.Store(lag.Nanoseconds())
		   metrics.MarketTickLag.Observe(lag.Seconds())
		  warn := s.lagWarn
		  if warn <= 0 {
			   warn = slo.MarketPushTarget
		   }
		   if lag > warn {
			   s.stale.Add(1)
			   metrics.MarketStale.WithLabelValues("lag").Inc()
			   s.log.Warn().Str("symbol", t.Symbol).Dur("lag", lag).Msg("market tick lag over design budget; not applying to quote cache")
			  return nil
		   }
	   }
	   s.mu.Lock()
	   prev, ok := s.last[t.Symbol]
	  if ok && t.Seq != 0 && t.Seq < prev.Seq {
		   s.mu.Unlock()
		   s.stale.Add(1)
		  metrics.MarketStale.WithLabelValues("seq").Inc()
		  return nil
	   }
	  s.seq++
	   if t.Seq == 0 {
		   t.Seq = s.seq
	  }
	  s.last[t.Symbol] = t
	  candle := s.rollCandleLocked(t)
	  s.mu.Unlock()
	   s.applied.Add(1)
	  metrics.MarketApplied.Inc()
	   if s.rdb != nil {
		  b, _ := json.Marshal(t)
		  _ = s.rdb.Set(context.Background(), "ticker:"+t.Symbol, b, s.ttl).Err()
		   if candle != nil {
			   cb, _ := json.Marshal(candle)
			  _ = s.rdb.Set(context.Background(), "kline:"+t.Symbol, cb, time.Minute).Err()
		  }
	   }
	  if broadcastTicker {
		   s.hub.BroadcastJSON(Envelope{Type: "ticker", Ticker: &t})
	   }
	  return candle
   }

// ApplyTick 写入内存/Redis 并推 WebSocket；过期/乱序 tick 丢掉，把滞后从轮询级压到秒内。

  func (s *Service) ApplyTick(t Tick) {
	  c := s.ingestTick(t, true)
	   if c == nil || s.klineTopic == "" || s.klineTopic == s.topic {
		   return
	  }
	   cb, _ := json.Marshal(c)
	  _ = s.bus.Publish(context.Background(), s.klineTopic, []byte(t.Symbol), cb)
   }

  func (s *Service) ingestKline(c Candle) {
	   s.mu.Lock()
	  s.openBar[c.Symbol] = c
	  s.mu.Unlock()
	   if s.rdb != nil {
		  cb, _ := json.Marshal(c)
		   _ = s.rdb.Set(context.Background(), "kline:"+c.Symbol, cb, time.Minute).Err()
	  }
   }

// rollCandleLocked 把 tick 滚进 1 分钟 K；持 s.mu。

   func (s *Service) rollCandleLocked(t Tick) *Candle {
	   px, err := decimal.NewFromString(t.Price)
	  if err != nil {
		  return nil
	   }
	   bucket := (t.Ts / 60000) * 60000
	  cur, ok := s.openBar[t.Symbol]
	  if !ok || cur.StartTs != bucket {
		   if ok {
			  hist := append([]Candle{}, s.candles[t.Symbol]...)
			   hist = append(hist, cur)
			   keep := s.klineKeep
			  if keep <= 0 {
				  keep = 120
			   }
			  if len(hist) > keep {
				   hist = hist[len(hist)-keep:]
			   }
			  s.candles[t.Symbol] = hist
		  }
		   cur = Candle{
			   Symbol: t.Symbol, Interval: "1m",
			   Open: t.Price, High: t.Price, Low: t.Price, Close: t.Price,
			   StartTs: bucket, Seq: t.Seq,
		  }
		   s.openBar[t.Symbol] = cur
		   cp := cur
		   return &cp
	   }
	  hi, _ := decimal.NewFromString(cur.High)
	  lo,
		   _ := decimal.NewFromString(cur.Low)
	  if px.GreaterThan(hi) {
		   cur.High = t.Price
	   }
	  if px.LessThan(lo) {
		   cur.Low = t.Price
	   }
	   cur.Close = t.Price
	  cur.Seq = t.Seq
	   s.openBar[t.Symbol] = cur
	   cp := cur
	  return &cp
  }

// Consume 订 ticker 和 K 线两个 topic，钱包页和交易页吃同一路。
   func (s *Service) Consume(ctx context.Context) {
	  s.bus.Subscribe(ctx, s.topic, "wallet-market-cache", func(ctx context.Context, _, value []byte) error {
		   var t Tick
		  if err := json.Unmarshal(value, &t); err != nil {
			   return err
		  }
		   s.ApplyTick(t)
		  return nil
	  })
	  if s.klineTopic != "" && s.klineTopic != s.topic {
		   s.bus.Subscribe(ctx, s.klineTopic, "wallet-kline-cache", func(ctx context.Context, _, value []byte) error {
			   var c Candle
			   if err := json.Unmarshal(value, &c); err != nil {
				  return err
			  }
			  s.ingestKline(c)
			  s.hub.BroadcastJSON(Envelope{Type: "kline", Kline: &c})
			  return nil
		  })
	  }
   }

// ProduceLoop 现货侧产出 ticker + 1m K 线，间隔默认 800ms，一致 <1s 推送预算。
   func (s *Service) ProduceLoop(ctx context.Context, every time.Duration) {
	  symbols := []struct {
		   sym   string
		   price decimal.Decimal
	  }{
		  {"BTC-USDT", decimal.RequireFromString("64000")},
		  {"ETH-USDT", decimal.RequireFromString("3200")},
		  {"SOL-USDT", decimal.RequireFromString("140")},
		  {"TRX-USDT", decimal.RequireFromString("0.12")},
		  {"TON-USDT", decimal.RequireFromString("5.5")},
	  }
	  if every <= 0 {
		   every = 800 * time.Millisecond
	  }
	  t := time.NewTicker(every)
	   go func() {
		   defer t.Stop()
		   var seq uint64
		  for {
			  select {
			   case <-ctx.Done():
				  return
			  case <-t.C:
				   for i := range symbols {
					  // 整数 bps 抖动（±10），账本金额不得走 float64。
					   bps := decimal.NewFromInt(int64(rand.Intn(21) - 10))
					   factor := decimal.NewFromInt(1).Add(bps.Div(decimal.NewFromInt(10000)))
					  symbols[i].price = symbols[i].price.Mul(factor)
					  seq++
					   tick := Tick{
						  Symbol: symbols[i].sym,
						  Price:  symbols[i].price.StringFixed(6),
						   Ts:     time.Now().UnixMilli(),
						  Seq: seq,
					   }
					  b, _ := json.Marshal(tick)
					  if err := s.bus.Publish(ctx, s.topic, []byte(tick.Symbol), b); err != nil {
						   s.log.Warn().Err(err).Msg("tick publish")
					   }
				   }
			  }
		   }
	   }()
  }

type LagSnapshot struct {
	LastLagMs    int64   `json:"last_lag_ms"`
	Applied      uint64  `json:"applied"`
	StaleDropped uint64  `json:"stale_dropped"`
	StaleRate    float64 `json:"stale_rate"`
	LagTargetMs int64 `json:"lag_target_ms"`
	PollingBaselineMs int64   `json:"polling_baseline_ms"`
	StaleTarget       float64 `json:"stale_rate_target"`
	NoisyFeedBaseline float64 `json:"noisy_feed_baseline"`
	DesignNote        string  `json:"design_note"`
}

// Lag 当前推送滞后和过期占比，给 1s / 0.5% 设计目标和告警用。
   func (s *Service) Lag() LagSnapshot {
	   applied := s.applied.Load()
	  stale := s.stale.Load()
	  rate := 0.0
	  if applied+stale > 0 {
		   rate = float64(stale) / float64(applied+stale)
	   }
	  return LagSnapshot{
		   LastLagMs:    s.lastLagNs.Load() / int64(time.Millisecond),
		   Applied:      applied,
		  StaleDropped: stale,
		   StaleRate:         rate,
		  LagTargetMs:       slo.MarketPushTarget.Milliseconds(),
		   PollingBaselineMs: slo.MarketPollingBaseline.Milliseconds(),
		  StaleTarget:       slo.MarketStaleRateTarget,
		  NoisyFeedBaseline: slo.MarketNoisyFeedBaseline,
		  DesignNote:        "push lag target 1s vs 15s polling; stale/error 0.5% vs 8% baseline",
	  }
  }

// ServeWS 后端推送入口；热路径不走 SQL。

  func (s *Service) ServeWS(w http.ResponseWriter, r *http.Request) {
	  c, err := s.up.Upgrade(w, r, nil)
	   if err != nil {
		  return
	  }
	  s.hub.Add(c)
	  snap := map[string]any{"type": "snapshot", "tickers": s.Cache()}
	  kl := map[string][]Candle{}
	  for sym := range s.Cache() {
		  kl[sym] = s.Klines(sym)
	   }
	  snap["klines"] = kl
	  b, _ := json.Marshal(snap)
	  _ = c.WriteMessage(websocket.TextMessage, b)
	   go func() {
		  defer s.hub.Remove(c)
		  for {
			   if _, _, err := c.ReadMessage(); err != nil {
				   return
			   }
		   }
	   }()
   }
