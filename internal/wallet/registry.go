// 多链节点机房：按可用、延迟和连续失败在地址池里切换。
// 作者: Likeo
// 日期: 2021.01.17
package wallet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/slo"
)

type TxStatus string

const (
	TxPending TxStatus = "pending"
	TxConfirmed TxStatus = "confirmed"
	TxFailed    TxStatus = "failed"
	DirIn                = "in"
	DirOut               = "out"
)

type Tx struct {
	Chain string
	Hash  string
	Asset string
	Amount    decimal.Decimal
	Direction string
	Status    TxStatus
	Address string
	FromAddress string // 付款方；观测可得时填充
	At          time.Time
	Height int64
	BlockHash string
	Reorged bool
}

// Adapter 是每条链的 RPC 面。账本只依赖该接口，不直接绑链 SDK。

type Adapter interface {
	ChainID() string
	NativeAsset() string
	GetBalance(ctx context.Context, address string) (decimal.Decimal, error)
	GetTxHistory(ctx context.Context, address string, limit int) ([]Tx, error)
	SubmitTransfer(ctx context.Context, from, to string, amount decimal.Decimal) (string, error)
	GetTxStatus(ctx context.Context, txHash string) (TxStatus, error)
	ObserveInbound(ctx context.Context, hash, to string, amount decimal.Decimal) (string, error)
	Ping(ctx context.Context) (time.Duration, error)
}

type TxInspector interface {
	InspectTx(ctx context.Context, txHash string) (Observation, error)
}

type EndpointHealth struct {
	Chain     string `json:"chain"`
	Endpoint  string `json:"endpoint"`
	Available bool   `json:"available"`
	LatencyMs int64 `json:"latency_ms"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	Circuit             string `json:"circuit"`
}

type node struct {
	url   string
	mu    sync.Mutex
	fails int
	latency  time.Duration
	up       bool
	openedAt time.Time
}

type HealthTracker struct {
	failThreshold int
	reset         time.Duration
	nodes map[string][]*node // chain -> endpoints
	mu    sync.RWMutex
}

  func NewHealthTracker(failThreshold int, reset time.Duration) *HealthTracker {
	   if failThreshold <= 0 {
		   failThreshold = 3
	  }
	  if reset <= 0 {
		   reset = slo.CircuitReset
	  }
	  return &HealthTracker{failThreshold: failThreshold, reset: reset, nodes: map[string][]*node{}}
   }

  func (h *HealthTracker) Register(chain string, urls []string) {
	  h.mu.Lock()
	  defer h.mu.Unlock()
	   list := make([]*node, 0, len(urls))
	  for _, u := range urls {
		  list = append(list, &node{url: u, up: true})
		  metrics.NodeAvailable.WithLabelValues(chain, u).Set(1)
	   }
	   h.nodes[chain] = list
  }

// Record 记录 RPC 耗时和成败；连续失败打开熔断，恢复后半开探测。

  func (h *HealthTracker) Record(chain, url string, latency time.Duration, err error) {
	   h.mu.RLock()
	   list := h.nodes[chain]
	   h.mu.RUnlock()
	  for _, n := range list {
		  if n.url != url {
			   continue
		  }
		   n.mu.Lock()
		  if err != nil {
			   n.fails++
			  if n.fails >= h.failThreshold {
				   // 连续失败熔断，后续选点走别的节点。
				  n.up = false
				   n.openedAt = time.Now()
				   metrics.NodeAvailable.WithLabelValues(chain,
					  url).Set(0)
			  }
		  } else {
			  n.fails = 0
			   n.up = true
			   n.openedAt = time.Time{}
			  if n.latency == 0 {
				  n.latency = latency
			   } else {
				   n.latency = (n.latency*7 + latency*3) / 10
			  }
			   metrics.NodeLatency.WithLabelValues(chain, url).Set(float64(n.latency.Milliseconds()))
			  metrics.NodeAvailable.WithLabelValues(chain, url).Set(1)
		  }
		  n.mu.Unlock()
		  return
	  }
  }

  func (n *node) probeOK(reset time.Duration) (up bool, lat time.Duration, halfOpen bool) {
	   n.mu.Lock()
	   defer n.mu.Unlock()
	  if n.up {
		  return true, n.latency, false
	   }
	   if !n.openedAt.IsZero() && time.Since(n.openedAt) >= reset {
		   // 半开探测，延迟打高一点，健康节点仍先。
		   return true,
			   n.latency + time.Hour, true
	  }
	  return false, n.latency, false
  }

  func (h *HealthTracker) Pick(chain string) (string, error) {
	   return h.PickExcept(chain, nil)
   }

// Pick 选当前最健康、延迟最低的端点；全挂时仍返回一个兜底，避免冻单没出口。
   func (h *HealthTracker) PickExcept(chain string, skip map[string]bool) (string, error) {
	  h.mu.RLock()
	  list := h.nodes[chain]
	   h.mu.RUnlock()
	  var best *node
	   var bestLat time.Duration
	   var fallback *node
	  for _, n := range list {
		  if skip[n.url] {
			  if fallback == nil {
				  fallback = n
			  }
			   continue
		  }
		   up, lat, half := n.probeOK(h.reset)
		   if !up {
			   if fallback == nil {
				   fallback = n
			  }
			  continue
		  }
		  if half && best != nil {
			   continue
		   }
		  if best == nil || lat < bestLat {
			   best = n
			   bestLat = lat
		   }
	   }
	   if best != nil {
		  return best.url, nil
	   }
	  // 全挂时仍给一个出口，冻单不能没节点可发。
	   if fallback != nil {
		   return fallback.url, nil
	  }
	   if len(list) > 0 {
		   return list[0].url, nil
	  }
	   return "", fmt.Errorf("no endpoints for chain %s", chain)
  }

   func (h *HealthTracker) PickN(chain string, n int) []string {
	  if n <= 0 {
		  n = 1
	  }
	  h.mu.RLock()
	  list := h.nodes[chain]
	   h.mu.RUnlock()
	  type scored struct {
		   url string
		   lat time.Duration
		   up  bool
	   }
	  rows := make([]scored, 0, len(list))
	   for _, nd := range list {
		  up, lat, _ := nd.probeOK(h.reset)
		  rows = append(rows, scored{url: nd.url, lat: lat, up: up})
	  }
	  out := make([]string, 0, n)
	  used := map[string]bool{}
	   for len(out) < n && len(out) < len(rows) {
		  var best *scored
		  for i := range rows {
			  if used[rows[i].url] {
				   continue
			  }
			  if !rows[i].up && best != nil {
				  continue
			   }
			   if best == nil || (rows[i].up && !best.up) || (rows[i].up == best.up && rows[i].lat < best.lat) {
				  best = &rows[i]
			   }
		   }
		  if best == nil {
			   break
		  }
		  used[best.url] = true
		   out = append(out, best.url)
	  }
	  return out
  }

   func (h *HealthTracker) Snapshot(chain string) []EndpointHealth {
	   h.mu.RLock()
	   defer h.mu.RUnlock()
	   out := []EndpointHealth{}
	  for c, list := range h.nodes {
		   if chain != "" && c != chain {
			   continue
		  }
		   for _, n := range list {
			  n.mu.Lock()
			   circ := "closed"
			  if !n.up {
				  circ = "open"
				   if !n.openedAt.IsZero() && time.Since(n.openedAt) >= h.reset {
					  circ = "half_open"
				  }
			  }
			  out = append(out, EndpointHealth{
				   Chain:     c,
				  Endpoint:  n.url,
				  Available: n.up,
				  LatencyMs:           n.latency.Milliseconds(),
				   ConsecutiveFailures: n.fails,
				   Circuit:             circ,
			   })
			   n.mu.Unlock()
		  }
	   }
	  return out
   }

  func (h *HealthTracker) DownCount() int {
	  h.mu.RLock()
	   defer h.mu.RUnlock()
	   n := 0
	   for _, list := range h.nodes {
		   for _, nd := range list {
			  nd.mu.Lock()
			  if !nd.up {
				  n++
			   }
			  nd.mu.Unlock()
		  }
	  }
	  return n
   }

type Registry struct {
	adapters map[string]Adapter
	health *HealthTracker
	retryN int
	backoff time.Duration
	hedge   bool // 查询可对赌两条；Submit 禁止 hedge，避免一笔冻结双播
}

  func NewEmptyRegistry(cfg config.NodeHealthConfig) *Registry {
	   h := NewHealthTracker(cfg.FailThreshold, cfg.CircuitReset)
	   r := &Registry{
		   adapters: map[string]Adapter{},
		   health:   h,
		  retryN:   cfg.RetryAttempts,
		  backoff:  cfg.RetryBackoff,
	   }
	  if r.retryN <= 0 {
		   r.retryN = 3
	   }
	   if r.backoff <= 0 {
		   r.backoff = 50 * time.Millisecond
	   }
	  r.hedge = cfg.HedgeReads
	   return r
   }

// NewRegistry 按配置注册各链 JSON-RPC。

  func NewRegistry(cfg *config.Config) *Registry {
	  nh := config.NodeHealthConfig{}
	   if cfg != nil {
		  nh = cfg.NodeHealth
	   }
	   r := NewEmptyRegistry(nh)
	  timeout := nh.ChainRPCTimeout
	  idx := newIndexerClient(config.IndexerURL(cfg),
		   0)
	   if cfg != nil {
		   idx = newIndexerClient(config.IndexerURL(cfg), cfg.Indexer.Timeout)
	  }
	   for _, c := range SupportedChains() {
		   urls := config.ChainEndpoints(cfg, c)
		   r.Put(newRPCAdapter(c, ListedAsset(c), urls, timeout, config.ChainTokens(cfg, c), idx), urls)
	  }
	   return r
   }

// NewMockRegistry 进程内注册表：不访问外部节点。

   func NewMockRegistry(cfg config.NodeHealthConfig) *Registry {
	   r := NewEmptyRegistry(cfg)
	  for _, c := range SupportedChains() {
		   urls := config.PickEndpoints(c, 2)
		   r.Put(newMockAdapter(c, ListedAsset(c), urls, mockChainBalance(c)), urls)
	  }
	  return r
   }

   func mockChainBalance(chain string) decimal.Decimal {
	   switch chain {
	  case "ETH":
		   return decimal.RequireFromString("12.5")
	   case "BTC":
		   return decimal.RequireFromString("0.42")
	  case "SOL":
		  return decimal.RequireFromString("88")
	  case "TRON":
		   return decimal.RequireFromString("1500")
	  case "TON":
		   return decimal.RequireFromString("42")
	  case "POLYGON":
		   return decimal.RequireFromString("80")
	   case "ARBITRUM":
		  return decimal.RequireFromString("4.2")
	   case "OPTIMISM":
		   return decimal.RequireFromString("2.7")
	   case "BASE":
		  return decimal.RequireFromString("1.9")
	   default:
		   return decimal.RequireFromString("1")
	  }
  }

   func (r *Registry) FailNext(chain string, n int) {
	  a, err := r.Get(chain)
	   if err != nil {
		   return
	   }
	   if m, ok := a.(*mockAdapter); ok {
		   m.FailNext(n)
	  }
   }

   func (r *Registry) FailEndpoint(chain, url string, n int) {
	   a, err := r.Get(chain)
	  if err != nil {
		  return
	  }
	   if m, ok := a.(*mockAdapter); ok {
		  m.FailEndpoint(url, n)
	   }
   }

   func (r *Registry) Endpoints(chain string) []string {
	   snap := r.health.Snapshot(chain)
	   out := make([]string, 0, len(snap))
	   for _, e := range snap {
		   out = append(out, e.Endpoint)
	  }
	   return out
   }

  func (r *Registry) Put(a Adapter, urls []string) {
	   r.adapters[a.ChainID()] = a
	  r.health.Register(a.ChainID(), urls)
  }

   func (r *Registry) Get(chain string) (Adapter, error) {
	  a, ok := r.adapters[chain]
	  if !ok {
		  return nil, fmt.Errorf("unknown chain %s", chain)
	   }
	   return a, nil
   }

func (r *Registry) MergeRegistered(chain string, rows []RegisteredToken) {
	if r == nil {
		return
	}
	ad, err := r.Get(strings.ToUpper(strings.TrimSpace(chain)))
	if err != nil {
		return
	}
	rpc, ok := ad.(*rpcAdapter)
	if !ok {
		return
	}
	for _, row := range rows {
		rpc.MergeToken(row.Asset, config.TokenSpec{Contract: row.Contract, Decimals: row.Decimals})
	}
}

  func (r *Registry) Chains() []string {
	  out := make([]string, 0, len(r.adapters))
	   for k := range r.adapters {
		   out = append(out, k)
	   }
	  return out
   }

  func (r *Registry) Health() *HealthTracker { return r.health }

// WithFailover 按健康度选点，失败切点重试。ctx 取消立刻停。

   func (r *Registry) WithFailover(ctx context.Context, chain, op string, fn func(ep string) error) error {
	  var last error
	   tried := map[string]bool{}
	  for i := 0; i < r.retryN; i++ {
		  ep, err := r.health.PickExcept(chain, tried)
		   if err != nil {
			  return err
		  }
		  start := time.Now()
		   last = fn(ep)
		   r.health.Record(chain, ep, time.Since(start), last)
		   result := "ok"
		   if last != nil {
			   result = "fail"
		   }
		  metrics.ChainRPC.WithLabelValues(chain,
			op, result).Inc()
		if last == nil {
			if op == "submit" {
				metrics.ChainSubmit.WithLabelValues(chain, "ok").Inc()
			}
			return nil
		}
		tried[ep] = true
		select {
		case <-ctx.Done():
			if op == "submit" {
				metrics.ChainSubmit.WithLabelValues(chain, "fail").Inc()
			}
			return ctx.Err()
		case <-time.After(r.backoff * time.Duration(i+1)):
		}
	}
	if op == "submit" {
		metrics.ChainSubmit.WithLabelValues(chain, "fail").Inc()
	}
	return last
}

  func (r *Registry) recordRPC(chain, op, ep string, d time.Duration, err error) {
	  r.health.Record(chain, ep, d, err)
	  result := "ok"
	   if err != nil {
		  result = "fail"
	  }
	  metrics.ChainRPC.WithLabelValues(chain, op, result).Inc()
  }

// WithRead 查询可对赌两条；Submit/Observe 仍走故障转移，禁止一笔冻结双播。
   func (r *Registry) WithRead(ctx context.Context, chain, op string, fn func(ep string) error) error {
	  if r != nil && r.hedge {
		   return r.WithHedged(ctx,
			chain, op, fn)
	}
	return r.WithFailover(ctx, chain, op, fn)
}

// WithHedged 两条查询竞速，先成功的算；写路径禁止用，避免双花。
   func (r *Registry) WithHedged(ctx context.Context, chain, op string, fn func(ep string) error) error {
	   eps := r.health.PickN(chain, 2)
	   if len(eps) < 2 {
		  return r.WithFailover(ctx, chain, op, fn)
	   }
	   ctx2, cancel := context.WithCancel(ctx)
	  defer cancel()
	   type res struct {
		  ep  string
		   err error
		   d   time.Duration
	  }
	   ch := make(chan res, 2)
	  for _, ep := range eps {
		   ep := ep
		   go func() {
			  start := time.Now()
			   err := fn(ep)
			   ch <- res{ep: ep, err: err, d: time.Since(start)}
		  }()
	}
	var last error
	for i := 0; i < len(eps); i++ {
		select {
		case <-ctx2.Done():
			if last != nil {
				return last
			}
			return ctx.Err()
		case r0 := <-ch:
			r.recordRPC(chain, op, r0.ep, r0.d, r0.err)
			if r0.err == nil {
				cancel()
				return nil
			}
			last = r0.err
		}
	}
	if last == nil {
		last = fmt.Errorf("hedged read failed for %s", chain)
	}
	return last
}
