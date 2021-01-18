// 钱包创建、导入，以及多链余额和流水查询。
// 覆盖 ETH/BTC/SOL/TRON/TON 和几条 EVM L2。
// 作者: Likeo
// 日期: 2021.01.17
package wallet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/cachettl"
	"account-assets.kucoin.net/funding/internal/conc"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/kms"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/trace"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

type BookBalance struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Book string `json:"book"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	ChainNative string `json:"chain_native"`
	Degraded    bool   `json:"degraded,omitempty"`
}

type BookReader interface {
	ListUserBooks(ctx context.Context,
		userID string) ([]BookRow, error)
}

type BookRow struct {
	AccountType string
	Asset     string
	Available decimal.Decimal
	Frozen    decimal.Decimal
}

// Service 多链余额/历史：账本 + 链 RPC 扇出。

type Service struct {
	reg     *Registry
	rdb     *redis.Client
	history HistoryStore
	books BookReader
	ttl   time.Duration
	queryTimeout time.Duration
	chainTimeout time.Duration
	repo         Repo
	enc kms.Keeper // 路径型 HSM：按次 unwrap，DEK 不常驻结构体
	cfg *config.Config
	addrCache sync.Map
	sf        conc.Group
	l1 *cachettl.Cache[[]BookBalance]
	registered func(context.Context, string) ([]RegisteredToken, error)
}

// RegisteredToken 是 Java 上架后同步进 aa_token_book 的一枚币。
type RegisteredToken struct {
	Asset    string
	Contract string
	Decimals int
}

// BindRegistered 入账和代币余额读取这份登记，不在进程里另维护币单。
func (s *Service) BindRegistered(fn func(context.Context, string) ([]RegisteredToken, error)) {
	if s == nil {
		return
	}
	s.registered = fn
}

func (s *Service) syncRegistered(ctx context.Context, chain string) {
	if s == nil || s.registered == nil || s.reg == nil {
		return
	}
	rows, err := s.registered(ctx, chain)
	if err != nil || len(rows) == 0 {
		return
	}
	s.reg.MergeRegistered(chain, rows)
}

// NewService 组装多链钱包查询面：账本余额 + 链上原生 + 流水，查询超时按 500ms 预算切。
   func NewService(cfg *config.Config, rdb *redis.Client, books BookReader, repo Repo, enc kms.Keeper, history HistoryStore) *Service {
	  if repo == nil {
		   // 未注入 SQLRepo 时使用内存仓储。
		   repo = NewMemRepo()
	  }
	  if history == nil {
		  history = NewMemHistory()
	  }
	   qt := cfg.NodeHealth.QueryTimeout
	  if qt <= 0 {
		  qt = 400 * time.Millisecond
	   }
	  ct := cfg.NodeHealth.ChainRPCTimeout
	  if ct <= 0 {
		   ct = 180 * time.Millisecond
	   }
	   return &Service{
		   reg:     openRegistry(cfg),
		  rdb:     rdb,
		  history: history,
		  books:   books,
		  ttl: cfg.Cache.BalanceTTL,
		   queryTimeout: qt,
		  chainTimeout: ct,
		   repo:         repo,
		   enc:          enc,
		  cfg: cfg,
		  l1: cachettl.New[[]BookBalance](cfg.Cache.BalanceTTL),
	   }
  }

   func openRegistry(cfg *config.Config) *Registry {
	   if cfg != nil && cfg.ChainDriver == "mock" {
		  return NewMockRegistry(cfg.NodeHealth)
	  }
	   return NewRegistry(cfg)
   }

  func (s *Service) Registry() *Registry { return s.reg }

  func canonChain(chain string) string {
	  return strings.ToUpper(strings.TrimSpace(chain))
  }

// Balances 直接读账本。链上查询超时的行标 Degraded，不把缓存行当作余额。
  func (s *Service) Balances(ctx context.Context, userID, chain string) ([]BookBalance, error) {
	   chain = canonChain(chain)
	   start := time.Now()
	   defer func() {
		   metrics.WalletQuerySeconds.WithLabelValues("balances").Observe(time.Since(start).Seconds())
	   }()
	   ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	  defer cancel()
	  return s.loadBalances(ctx, userID, chain)
   }

  func (s *Service) l1Get(key string) ([]BookBalance, bool) {
	   if s == nil || s.l1 == nil {
		  return nil, false
	  }
	  return s.l1.Get(key)
   }

  func (s *Service) l1Set(key string, rows []BookBalance) {
	   if s == nil || s.l1 == nil {
		   return
	  }
	  cp := append([]BookBalance(nil), rows...)
	  s.l1.Set(key, cp)
  }

// loadBalances 账本余额 + 各链原生；某链 RPC 超时记 Degraded，不拖垮整页。
   func (s *Service) loadBalances(ctx context.Context, userID, chain string) ([]BookBalance, error) {
	   metrics.BalanceCoalesce.Inc()
	   rows, err := s.books.ListUserBooks(ctx, userID)
	   if err != nil {
		   return nil, err
	  }
	   chains := s.reg.Chains()
	  if chain != "" {
		  chains = []string{chain}
	   }
	  native := s.fetchNatives(ctx, userID, chains)
	  out := make([]BookBalance, 0, len(rows)+len(chains))
	   for _, r := range rows {
		  if chain != "" && !AssetOnChain(r.Asset, chain) {
			   continue
		  }
		  item := BookBalance{
			  Asset: r.Asset,
			  Book:  r.AccountType,
			  Available: r.Available.String(),
			   Frozen:    r.Frozen.String(),
		   }
		   bindChainNative(&item, r.Asset, chain, native)
		   out = append(out, item)
	   }
	  seen := map[string]bool{}
	  for _, item := range out {
		  if item.Chain != "" && item.Asset != "" {
			   seen[item.Chain+"|"+item.Asset] = true
		  }
	   }
	   for _, c := range chains {
		   ad, err := s.reg.Get(c)
		   if err != nil {
			   continue
		  }
		   nkey := c + "|" + ad.NativeAsset()
		  if !seen[nkey] {
			   nat := native[nkey]
			  out = append(out, BookBalance{
				  Chain: c, Asset: ad.NativeAsset(), Book: "chain",
				  Available: "0", Frozen: "0", ChainNative: nat, Degraded: nat == "",
			  })
			   seen[nkey] = true
		  }
		  if tr, ok := ad.(TokenReader); ok {
			  for _, asset := range tr.TokenAssets() {
				   key := c + "|" + asset
				  if seen[key] {
					   continue
				   }
				  v, ok := native[key]
				  if !ok {
					   continue
				   }
				   out = append(out, BookBalance{
					   Chain: c, Asset: asset, Book: "chain",
					   Available: "0", Frozen: "0", ChainNative: v,
				  })
				   seen[key] = true
			  }
		  }
	   }
	  return out, nil
  }

// bindChainNative 把链上原生/代币余额挂到账本行。RPC 失败记 Degraded，不得用 0 冒充已对账。
  func bindChainNative(item *BookBalance, asset, filter string, native map[string]string) {
	  for _, c := range SupportedChains() {
		   if filter != "" && c != filter {
			  continue
		   }
		  if !AssetOnChain(asset, c) {
			  continue
		   }
		   if v, ok := native[c+"|"+asset]; ok {
			  item.Chain = c
			   item.ChainNative = v
			   item.Degraded = v == ""
			   return
		  }
	  }
	   for _, c := range SupportedChains() {
		  if filter != "" && c != filter {
			   continue
		  }
		  if AssetOnChain(asset, c) {
			   item.Chain = c
			  item.Degraded = true
			  return
		  }
	   }
  }

// fetchNatives 并行拉各链原生和配置里的代币余额，受 chainTimeout 约束；单链超时不得拖垮整页。

  func (s *Service) fetchNatives(ctx context.Context, userID string, chains []string) map[string]string {
	  for _, c := range chains {
		  s.syncRegistered(ctx, c)
	  }
	   type batch struct{ items map[string]string }
	  ch := make(chan batch, len(chains))
	   for _, c := range chains {
		   c := c
		  go func() {
			  cctx, cancel := context.WithTimeout(ctx, s.chainTimeout)
			  defer cancel()
			   ad, err := s.reg.Get(c)
			   if err != nil {
				  ch <- batch{}
				  return
			   }
			   addr := s.AddressFor(cctx, userID, c)
			  if addr == "" {
				   ch <- batch{}
				   return
			   }
			  items := map[string]string{}
			   var n decimal.Decimal
			  err = s.reg.WithRead(cctx, c, "balance", func(ep string) error {
				  var e error
				   n,
					   e = ad.GetBalance(WithRPCURL(cctx, ep), addr)
				  return e
			   })
			   if err == nil {
				   items[c+"|"+ad.NativeAsset()] = n.String()
			   }
			  if tr, ok := ad.(TokenReader); ok {
				   for _, asset := range tr.TokenAssets() {
					   var tb decimal.Decimal
					  e := s.reg.WithRead(cctx, c, "token_balance", func(ep string) error {
						  var te error
						  tb, te = tr.TokenBalance(WithRPCURL(cctx, ep), addr, asset)
						  return te
					  })
					  if e == nil {
						  items[c+"|"+asset] = tb.String()
					  }
				   }
			  }
			  ch <- batch{items: items}
		   }()
	  }
	   native := map[string]string{}
	   for i := 0; i < len(chains); i++ {
		  for k, v := range (<-ch).items {
			   native[k] = v
		   }
	   }
	  return native
  }

// TxHistory 交易记录：先读已落库 hash，再按链并行补 RPC 历史，同一 500ms 预算。

   func (s *Service) TxHistory(ctx context.Context, userID, chain string, limit int) ([]Tx, error) {
	  start := time.Now()
	   defer func() {
		  metrics.WalletQuerySeconds.WithLabelValues("tx_history").Observe(time.Since(start).Seconds())
	   }()
	  ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	   defer cancel()
	  chain = canonChain(chain)
	   if limit <= 0 || limit > 50 {
		   limit = 20
	  }
	  stored, err := s.history.List(ctx, userID, chain, "", limit)
	  if err != nil {
		   return nil, err
	   }
	  chains := []string{chain}
	   if chain == "" {
		  chains = s.reg.Chains()
	   }
	   type hist struct{ items []Tx }
	   ch := make(chan hist, len(chains))
	   for _, c := range chains {
		  c := c
		   go func() {
			  cctx, cancel := context.WithTimeout(ctx, s.chainTimeout)
			   defer cancel()
			   ad, err := s.reg.Get(c)
			   if err != nil {
				   ch <- hist{}
				  return
			   }
			   addr := s.AddressFor(cctx, userID, c)
			  var items []Tx
			   _ = s.reg.WithRead(cctx, c, "history", func(ep string) error {
				   var e error
				  items, e = ad.GetTxHistory(WithRPCURL(cctx, ep), addr, limit)
				   return e
			  })
			   ch <- hist{items: items}
		  }()
	  }
	   // 已落库的广播哈希先，避免被适配器种子历史盖掉。
	   byHash := map[string]Tx{}
	   order := make([]string, 0, limit)
	  push := func(t Tx) {
		  k := t.Chain + "|" + t.Hash
		   if _, ok := byHash[k]; ok {
			   return
		  }
		   byHash[k] = t
		   order = append(order, k)
	   }
	  for _, t := range stored {
		  push(t)
	   }
	   for i := 0; i < len(chains); i++ {
		  for _, t := range (<-ch).items {
			   push(t)
		   }
	  }
	   out := make([]Tx, 0, len(order))
	   for _, k := range order {
		  out = append(out, byHash[k])
		   if len(out) >= limit {
			   break
		  }
	   }
	  return out, nil
   }

// writeRPC Submit 超时，避免冻金等死 RPC。

   func (s *Service) writeRPC(ctx context.Context) (context.Context, context.CancelFunc) {
	  d := s.chainTimeout
	   if d <= 0 {
		   d = 180 * time.Millisecond
	   }
	   return context.WithTimeout(ctx, d)
   }

// Submit 广播转账：按节点健康选端点并故障转移。
   func (s *Service) Submit(ctx context.Context, chain, to string, amount decimal.Decimal) (hash string, err error) {
	  ctx, sp := trace.WithSpan(ctx, "wallet.Submit")
	   defer func() { sp.End(zerolog.Nop(), err) }()
	   start := time.Now()
	   defer metrics.ChainWriteSeconds.WithLabelValues("submit").Observe(time.Since(start).Seconds())
	   ctx, cancel := s.writeRPC(ctx)
	  defer cancel()
	   chain = canonChain(chain)
	   ad, err := s.reg.Get(chain)
	   if err != nil {
		  return "", err
	   }
	   userID := UserIDFrom(ctx)
	   from := s.AddressFor(ctx, userID, chain)
	   if from == "" || ValidChainAddress(chain, from) != nil {
		   return "", fmt.Errorf("custody from address unavailable")
	  }
	  if to == "" || ValidChainAddress(chain, to) != nil {
		   return "", fmt.Errorf("withdraw to address invalid")
	   }
	  if nonce := SubmitNonceFrom(ctx); nonce != "" && s.history != nil {
		   if h, ok := s.history.LookupSubmit(ctx, nonce); ok {
			  metrics.SubmitDeduped.Inc()
			  return h, nil
		   }
	  }
	   if s.enc != nil {
		  if nonce := SubmitNonceFrom(ctx); nonce != "" {
			   wm := WithdrawMemoFrom(ctx)
			   if err := signpipe.AllowCold(signpipe.ColdRef{
				   MovementID: nonce, Chain: chain, To: to, Amount: amount.String(),
			  }); err != nil {
				  return "", err
			   }
			  sig, e2 := s.enc.SignWithdraw(nonce, chain, to, amount.String(), wm.DestinationTagValue())
			  if e2 != nil {
				  return "", e2
			   }
			  ctx = WithWithdrawSig(ctx, sig)
			  metrics.KMSOps.WithLabelValues("sign_withdraw").Inc()
		  }
	   }
	   err = s.reg.WithFailover(ctx, chain, "submit", func(ep string) error {
		  var e error
		   hash, e = ad.SubmitTransfer(WithRPCURL(ctx, ep), from, to, amount)
		   return e
	   })
	   if err != nil || hash == "" {
		  return hash, err
	  }
	  if nonce := SubmitNonceFrom(ctx); nonce != "" && s.history != nil {
		   _ = s.history.RememberSubmit(ctx,
			   nonce, hash)
	   }
	  _ = s.history.Record(ctx, userID, Tx{
		   Chain: chain, Hash: hash, Asset: ad.NativeAsset(), Amount: amount,
		  Direction: "out", Status: TxPending, Address: to, At: time.Now().UTC(),
	   })
	  return hash, nil
  }

// Observe 认领入金哈希。只观察已有 inbound，不冻用户账。空哈希生成 observe id，重试复用。
  func (s *Service) Observe(ctx context.Context, chain, hash string, amount decimal.Decimal) (out string, err error) {
	  start := time.Now()
	   defer metrics.ChainWriteSeconds.WithLabelValues("observe").Observe(time.Since(start).Seconds())
	   ctx, cancel := s.writeRPC(ctx)
	  defer cancel()
	  chain = canonChain(chain)
	   ad, err := s.reg.Get(chain)
	   if err != nil {
		   return "", err
	   }
	  userID := UserIDFrom(ctx)
	   to := s.AddressFor(ctx, userID, chain)
	  if to == "" || ValidChainAddress(chain, to) != nil {
		  return "", fmt.Errorf("deposit to address unavailable")
	   }
	  err = s.reg.WithFailover(ctx, chain, "observe", func(ep string) error {
		  var e error
		  out, e = ad.ObserveInbound(WithRPCURL(ctx, ep), hash, to, amount)
		   return e
	  })
	   if err != nil || out == "" {
		   return out, err
	  }
	  _ = s.history.Record(ctx, userID, Tx{
		   Chain: chain, Hash: out, Asset: ad.NativeAsset(), Amount: amount,
		   Direction: DirIn, Status: TxPending, Address: to, At: time.Now().UTC(),
	  })
	   return out, nil
   }

// TxStatus 查链上确认状态，供 worker 决定入账还是回滚。

   func (s *Service) TxStatus(ctx context.Context, chain, hash string) (st TxStatus, err error) {
	  start := time.Now()
	   defer metrics.ChainWriteSeconds.WithLabelValues("status").Observe(time.Since(start).Seconds())
	  ctx, cancel := s.writeRPC(ctx)
	   defer cancel()
	  ad, err := s.reg.Get(chain)
	  if err != nil {
		   return "", err
	  }
	  err = s.reg.WithRead(ctx, chain, "status", func(ep string) error {
		   var e error
		   st, e = ad.GetTxStatus(WithRPCURL(ctx, ep), hash)
		  return e
	   })
	  return st, err
  }

// Inspect 拉链上金额/资产/确认数；充值入账必须用观测值，不能用申请额。
   func (s *Service) Inspect(ctx context.Context, chain, hash string) (obs Observation, err error) {
	   start := time.Now()
	   defer metrics.ChainWriteSeconds.WithLabelValues("inspect").Observe(time.Since(start).Seconds())
	   ctx, cancel := s.writeRPC(ctx)
	  defer cancel()
	   ad, err := s.reg.Get(chain)
	   if err != nil {
		   return Observation{}, err
	   }
	   s.syncRegistered(ctx, chain)
	  err = s.reg.WithRead(ctx, chain, "inspect", func(ep string) error {
		  rctx := WithRPCURL(ctx, ep)
		  if ins, ok := ad.(TxInspector); ok {
			  obs, err = ins.InspectTx(rctx, hash)
			   return err
		   }
		  st,
			   e := ad.GetTxStatus(rctx, hash)
		   obs = Observation{Hash: hash, Status: st}
		   if st == TxConfirmed {
			   obs.Confirmations = 8
		   }
		   return e
	   })
	   return obs, err
  }

// InspectBatch 同链批量观测，worker 一次扫多笔，少打 RPC。
  func (s *Service) InspectBatch(ctx context.Context, chain string, hashes []string) (map[string]Observation, error) {
	  start := time.Now()
	  defer metrics.ChainWriteSeconds.WithLabelValues("inspect_batch").Observe(time.Since(start).Seconds())
	  out := map[string]Observation{}
	  seen := map[string]struct{}{}
	  uniq := make([]string, 0, len(hashes))
	   for _, h := range hashes {
		  if h == "" {
			   continue
		  }
		   if _, ok := seen[h]; ok {
			   continue
		   }
		   seen[h] = struct{}{}
		   uniq = append(uniq, h)
	   }
	  if len(uniq) == 0 {
		   return out, nil
	   }
	  ctx, cancel := s.writeRPC(ctx)
	   defer cancel()
	  ad, err := s.reg.Get(chain)
	   if err != nil {
		  return nil, err
	  }
	  err = s.reg.WithRead(ctx, chain, "inspect_batch", func(ep string) error {
		   rctx := WithRPCURL(ctx, ep)
		   if b, ok := ad.(TxBatchInspector); ok {
			  got,
				   e := b.InspectTxBatch(rctx, uniq)
			   if e != nil {
				   return e
			   }
			   out = got
			  return nil
		  }
		   got := make(map[string]Observation, len(uniq))
		   for _, h := range uniq {
			   if ins, ok := ad.(TxInspector); ok {
				  o, e := ins.InspectTx(rctx, h)
				   if e != nil {
					   got[h] = Observation{Hash: h, Status: TxFailed}
					  continue
				  }
				  got[h] = o
				   continue
			  }
			   st, e := ad.GetTxStatus(rctx, h)
			  o := Observation{Hash: h, Status: st}
			  if e != nil {
				  o.Status = TxFailed
			   }
			   got[h] = o
		   }
		   out = got
		  return nil
	  })
	  metrics.InspectBatchSize.Observe(float64(len(uniq)))
	   if out == nil {
		  out = map[string]Observation{}
	   }
	  return out, err
  }

// Fork 实验分叉：确认数回退，已入账充值要 unwind。
   func (s *Service) Fork(chain string) (Fork, error) {
	   ad, err := s.reg.Get(chain)
	  if err != nil {
		  return nil, err
	   }
	   f, ok := ad.(Fork)
	   if !ok {
		   return nil, fmt.Errorf("chain %s has no fork control", chain)
	  }
	  return f, nil
  }

// NodeHealth 各链端点可用/延迟/熔断快照，给 /readyz 和告警。

  func (s *Service) NodeHealth(chain string) []EndpointHealth {
	   return s.reg.Health().Snapshot(chain)
  }

// AddressFor 取用户在该链的充值地址。先查 custody_addresses / aa_addr_binds。
// 档不得用用户号哈希静默派生后跳过库。

   func (s *Service) AddressFor(ctx context.Context, userID, chain string) string {
	   userID = strings.TrimSpace(userID)
	  chain = canonChain(chain)
	  if userID == "" || chain == "" {
		  return ""
	  }
	  ck := userID + "|" + chain
	   if a, ok := s.addrCache.Load(ck); ok {
		  if addr, ok := a.(string); ok && addr != "" {
			   return addr
		   }
	  }
	   if s.repo != nil {
		   if a, ok := s.repo.Address(ctx, userID, chain); ok && a != "" && ValidChainAddress(chain, a) == nil {
			  s.addrCache.Store(ck, a)
			  return a
		  }
	   }
	   if s.refuseSilentDerive() {
		  return ""
	  }
	   addr := DeriveAddress(userID, chain)
	   if addr == "" || ValidChainAddress(chain, addr) != nil {
		  return ""
	  }
	   s.addrCache.Store(ck, addr)
	   return addr
   }

  func (s *Service) refuseSilentDerive() bool {
	  if s == nil || s.cfg == nil {
		   return false
	   }
	  return authn.ProductionProfile(s.cfg.Auth.Kind, s.cfg.Profile)
  }

   func (s *Service) allowRevealMnemonic() bool {
	  if s == nil || s.cfg == nil {
		  return false
	   }
	  if authn.ProductionProfile(s.cfg.Auth.Kind, s.cfg.Profile) {
		   return false
	  }
	   return s.cfg.WalletRevealMnemonic || s.cfg.Auth.AllowInsecure
   }

// CreateWallet 生成助记词并落库（只存密文），派生多链地址。
  func (s *Service) CreateWallet(ctx context.Context, userID, label string) (*Wallet, error) {
	   mn, err := GenerateMnemonic()
	   if err != nil {
		   return nil, err
	   }
	  return s.persistWallet(ctx, userID, label, "created", mn)
   }

// ImportWallet 导入已有助记词，校验后按同一套规则派生地址。
   func (s *Service) ImportWallet(ctx context.Context, userID, label, secret string) (*Wallet, error) {
	  norm, err := NormalizeMnemonic(secret)
	   if err != nil {
		  return nil, err
	   }
	   return s.persistWallet(ctx, userID, label, "imported", norm)
   }

// persistWallet 助记词加密落库并派生多链地址；明文不进 SQL。
  func (s *Service) persistWallet(ctx context.Context, userID, label, source, secret string) (*Wallet, error) {
	   if label == "" {
		  label = source
	   }
	  w := NewWalletRecord(userID, label, source, secret)
	  if err := assertDerivedAddresses(w.Addresses); err != nil {
		  return nil, err
	   }
	   if s.enc == nil {
		   return nil, fmt.Errorf("kms envelope required")
	  }
	   plain := []byte(secret)
	   enc, err := s.enc.Encrypt(plain)
	  zeroSecret(plain)
	   if err != nil {
		  return nil, err
	   }
	   if err := s.repo.Insert(ctx, w, enc); err != nil {
		  return nil, err
	   }
	  w.Mnemonic = ""
	  if s.allowRevealMnemonic() {
		  w.Mnemonic = secret
	  }
	   s.forgetAddresses(userID)
	  s.Invalidate(ctx, userID)
	  return w, nil
  }

   func zeroSecret(b []byte) {
	  for i := range b {
		  b[i] = 0
	  }
  }

  func (s *Service) forgetAddresses(userID string) {
	   if s == nil || userID == "" {
		   return
	  }
	  for _, c := range SupportedChains() {
		   s.addrCache.Delete(userID + "|" + c)
	   }
  }

// assertDerivedAddresses 创建/导入时核对各链地址是否齐全且形态正确。

  func assertDerivedAddresses(addrs map[string]string) error {
	  for _, c := range SupportedChains() {
		  a := ""
		  if addrs != nil {
			   a = addrs[c]
		   }
		   if a == "" {
			   return fmt.Errorf("derive %s: empty", c)
		  }
		  if err := ValidChainAddress(c, a); err != nil {
			  return fmt.Errorf("derive %s: %w", c, err)
		   }
	  }
	   return nil
   }

// ListWallets 该用户已创建/导入的钱包列表。

   func (s *Service) ListWallets(ctx context.Context, userID string) ([]Wallet, error) {
	  list, err := s.repo.List(ctx, userID)
	   if err != nil {
		   return nil, err
	  }
	  for i := range list {
		  list[i].Mnemonic = ""
	  }
	  return list, nil
  }

  func (s *Service) Chains() []string { return s.reg.Chains() }

// Invalidate 入账/冻结后立刻丢掉余额缓存，避免页面看到冻前可用。
   func (s *Service) Invalidate(ctx context.Context, userID string) {
	  if userID != "" && s.l1 != nil {
		   s.l1.DelPrefix(BalanceCachePrefix(userID))
	   }
	   if s.rdb == nil || userID == "" {
		   return
	  }
	  var cursor uint64
	   pat := BalanceCachePrefix(userID) + "*"
	  for {
		  keys,
			   next, err := s.rdb.Scan(ctx, cursor, pat, 50).Result()
		   if err != nil {
			  return
		   }
		  if len(keys) > 0 {
			   _ = s.rdb.Del(ctx, keys...).Err()
		  }
		  cursor = next
		  if cursor == 0 {
			  break
		   }
	  }
  }

// ProbeLoop 后台探活各链端点，给调度提供可用/延迟/失败数据。
  func (s *Service) ProbeLoop(ctx context.Context, every time.Duration) {
	   if every <= 0 {
		   every = 2 * time.Second
	   }
	  t := time.NewTicker(every)
	   go func() {
		  defer t.Stop()
		   for {
			   select {
			   case <-ctx.Done():
				  return
			  case <-t.C:
				   for _, c := range s.reg.Chains() {
					   ad, _ := s.reg.Get(c)
					   _ = s.reg.WithRead(ctx, c, "ping", func(ep string) error {
						   _, err := ad.Ping(WithRPCURL(ctx, ep))
						   return err
					  })
				   }
			  }
		   }
	   }()
   }

// InjectInbound 适配器入金：往托管地址打一笔，给扫充值用。走链上观察。

   func (s *Service) InjectInbound(chain, to, asset string, amount decimal.Decimal) (string, error) {
	  ad, err := s.reg.Get(chain)
	  if err != nil {
		   return "", err
	  }
	   m, ok := ad.(*mockAdapter)
	   if !ok {
		  return "", fmt.Errorf("inbound inject requires mock adapter")
	   }
	  if to == "" {
		  return "", fmt.Errorf("to address required")
	  }
	  if asset == "" {
		  asset = m.NativeAsset()
	   }
	   return m.InjectInbound(to, asset, amount), nil
   }

// InboundConfirmed 该地址已确认入账哈希，worker ScanDeposits 补记 wallet_deposit。
  func (s *Service) InboundConfirmed(ctx context.Context, chain, address string) ([]Tx, error) {
	  ad, err := s.reg.Get(chain)
	   if err != nil {
		   return nil, err
	   }
	   list, err := ad.GetTxHistory(ctx, address, 50)
	   if err != nil {
		   return nil, err
	  }
	  out := make([]Tx, 0, len(list))
	  for _, t := range list {
		  if t.Direction != "in" || t.Status != TxConfirmed {
			   continue
		  }
		   if skipEmptyHistoryHash(t.Hash) {
			   continue
		  }
		  out = append(out, t)
	   }
	  return out, nil
  }

  func (s *Service) CustodyUsers(ctx context.Context) []string {
	  if s.repo == nil {
		  return nil
	  }
	   ids, err := s.repo.ListUserIDs(ctx)
	   if err != nil {
		  return nil
	   }
	   return ids
  }
