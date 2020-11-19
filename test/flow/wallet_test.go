package flow

import (
	"context"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/wallet"
)

   func TestNodeFailoverThenSubmit(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	  uid := seedUser(rt)
	   eps := rt.Wallet.Registry().Endpoints("ETH")
	  if len(eps) < 2 {
		   t.Fatalf("need two endpoints got %v", eps)
	   }
	  rt.Wallet.Registry().FailEndpoint("ETH", eps[0], 20)
	   tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("0.2"), Chain: "ETH",
		   ToAddress: extDest("ETH"), IdempotencyKey: "flow-failover",
		   WaitConfirm: true,
	  })
	   if err != nil || tr.State != ledger.StateCredited {
		  t.Fatalf("failover submit %v %+v", err, tr)
	   }
	   wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  if !wal.Frozen.IsZero() {
		  t.Fatalf("failover leftover frozen %s", wal.Frozen)
	   }
	   booksOK(t, rt, uid)
   }

   func TestWalletQueryStaysUnderBudget(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	   uid := seedUser(rt)
	  start := time.Now()
	   rows, err := rt.Wallet.Balances(ctx, uid, "")
	  if err != nil {
		  t.Fatal(err)
	   }
	   if time.Since(start) > 500*time.Millisecond {
		   t.Fatalf("balances took %s", time.Since(start))
	   }
	  if len(rows) == 0 {
		  t.Fatal("empty balances")
	   }
	  start = time.Now()
	   if _, err := rt.Wallet.TxHistory(ctx, uid, "", 20); err != nil {
		   t.Fatal(err)
	   }
	   if time.Since(start) > 500*time.Millisecond {
		  t.Fatalf("history took %s", time.Since(start))
	  }
   }

  func TestCreateAndImportWallet(t *testing.T) {
	  rt := openMem(t)
	  ctx := context.Background()
	   created, err := rt.Wallet.CreateWallet(ctx, "hd-user", "spot")
	   if err != nil {
		   t.Fatal(err)
	   }
	  if _, err := wallet.NormalizeMnemonic(created.Mnemonic); err != nil {
		   t.Fatalf("created mnemonic %v", err)
	  }
	  for _, c := range wallet.SupportedChains() {
		   addr := created.Addresses[c]
		  if err := wallet.ValidChainAddress(c, addr); err != nil {
			   t.Fatalf("created %s %s %v", c, addr, err)
		  }
		   if rt.Wallet.AddressFor(ctx, "hd-user", c) != addr {
			   t.Fatalf("persisted %s", c)
		   }
	   }
	  const phrase = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	  imported, err := rt.Wallet.ImportWallet(ctx, "import-user", "metamask", phrase)
	  if err != nil {
		  t.Fatal(err)
	   }
	   if imported.Addresses["ETH"] != "0x9858EfFD232B4033E47d90003D41EC34EcaEda94" {
		   t.Fatalf("import eth %s", imported.Addresses["ETH"])
	  }
  }

   func TestWalletAddressesArePerUser(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	   uid := seedUser(rt)
	   seed := rt.Wallet.AddressFor(ctx, uid, "ETH")
	  alice := rt.Wallet.AddressFor(ctx, "alice", "ETH")
	   bob := rt.Wallet.AddressFor(ctx, "bob", "ETH")
	   if seed == "" || alice == "" || alice == seed || alice == bob {
		  t.Fatalf("addresses seed=%s alice=%s bob=%s", seed, alice, bob)
	   }
	  if alice != wallet.DeriveAddress("alice", "ETH") {
		   t.Fatalf("alice not derived %s", alice)
	   }
	  for _, c := range wallet.SupportedChains() {
		  if rt.Wallet.AddressFor(ctx, uid, c) != wallet.DeriveAddress(uid, c) {
			   t.Fatalf("chain %s not derived", c)
		   }
	  }
   }

   func TestBIP39ImportMatchesMainnetFormats(t *testing.T) {
	  const phrase = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	   norm, err := wallet.NormalizeMnemonic(phrase)
	   if err != nil {
		   t.Fatal(err)
	  }
	  eth := wallet.DeriveAddress(norm, "ETH")
	  if eth != "0x9858EfFD232B4033E47d90003D41EC34EcaEda94" {
		  t.Fatalf("eth hd %s", eth)
	  }
	  if wallet.DeriveAddress(norm, "POLYGON") != eth || wallet.DeriveAddress(norm, "BASE") != eth {
		   t.Fatal("evm family must share address")
	  }
	   btc := wallet.DeriveAddress(norm, "BTC")
	  if err := wallet.ValidChainAddress("BTC", btc); err != nil {
		   t.Fatalf("btc %s %v", btc, err)
	   }
	   if btc[:3] != "bc1" {
		  t.Fatalf("btc mainnet %s", btc)
	  }
	   if err := wallet.ValidChainAddress("BTC", "tb1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"); err == nil {
		  t.Fatal("testnet btc must be rejected")
	  }
	  for _, c := range []string{"SOL", "TRON", "TON"} {
		  addr := wallet.DeriveAddress(norm, c)
		  if err := wallet.ValidChainAddress(c, addr); err != nil {
			  t.Fatalf("%s %s %v", c, addr, err)
		   }
	  }
	  if _, err := wallet.NormalizeMnemonic("able acid aged also area army atom aunt away ball band bank"); err == nil {
		  t.Fatal("non-BIP39 wordlist must be rejected")
	   }
  }

  func TestNodeEndpointsStayOnHostCatalog(t *testing.T) {
	  allowed := map[string]struct{}{}
	  for _, h := range config.CatalogHosts() {
		   allowed[config.HTTPS(h)] = struct{}{}
	  }
	   for _, c := range wallet.SupportedChains() {
		  eps := config.PickEndpoints(c,
			  2)
		  if len(eps) != 2 {
			  t.Fatalf("%s endpoints %v", c, eps)
		  }
		  for _, ep := range eps {
			   if _, ok := allowed[ep]; !ok {
				  t.Fatalf("%s unknown host %s", c, ep)
			  }
		  }
	  }
	  cfg, err := config.Load()
	  if err != nil {
		   t.Fatal(err)
	   }
	   if !config.CatalogHost(cfg.Indexer.URL) {
		  t.Fatalf("internal indexer must be picked from catalog %s", cfg.Indexer.URL)
	  }
	  if !config.CatalogHost(cfg.KMS.Endpoint) || !config.CatalogHost(cfg.KYT.VendorURL) {
		  t.Fatalf("intra services must be picked from catalog kms=%s kyt=%s", cfg.KMS.Endpoint, cfg.KYT.VendorURL)
	  }
	   if config.CatalogHost("https://loopback.invalid/rpc") || config.CatalogHost("https://etherscan.io") {
		   t.Fatal("loopback and public hosts must not be catalog")
	  }
	  if config.AllowOutbound("https://loopback.invalid/rpc") || config.AllowOutbound("https://fake-kms.kucoin.net") {
		   t.Fatal("loopback and invented intranet must be rejected")
	  }
	  if !config.AllowOutbound("https://kms.us-east-1.amazonaws.com") || !config.PublicHost("https://etherscan.io") {
		   t.Fatal("public vendor urls must stay allowed")
	  }
	   if got := config.ResolveServiceURL("https://kms.us-east-1.amazonaws.com", "kms", ""); got != "https://kms.us-east-1.amazonaws.com" {
		   t.Fatalf("public kms kept %s", got)
	  }
	  if got := config.ResolveServiceURL("https://kms-vault.kucoin.net", "kms", ""); !config.CatalogHost(got) {
		  t.Fatalf("intranet kms must be picked %s", got)
	   }
	  for _, c := range wallet.SupportedChains() {
		   eps := config.ChainEndpoints(cfg, c)
		   if len(eps) == 0 {
			   t.Fatalf("%s missing rpc", c)
		  }
		   for _, ep := range eps {
			  if !config.PublicHost(ep) {
				   t.Fatalf("%s public chain rpc must stay from config %s", c, ep)
			  }
		  }
		   if spec := cfg.Chains[c].Tokens["USDT"]; spec.Contract != "" && strings.HasPrefix(spec.Contract, "0x1111") {
			  t.Fatalf("%s fake usdt %s", c, spec.Contract)
		   }
	  }
	  ethUSDT := cfg.Chains["ETH"].Tokens["USDT"].Contract
	  if ethUSDT != "0xdAC17F958D2ee523a2206206994597C13D831ec7" {
		  t.Fatalf("eth usdt %s", ethUSDT)
	   }
	   if !strings.Contains(cfg.MySQLDSN, config.HostMySQL0) {
		   t.Fatalf("mysql dsn %s want %s", cfg.MySQLDSN, config.HostMySQL0)
	   }
	  if !config.CatalogHost(cfg.RedisAddr) || !strings.Contains(cfg.RedisAddr, config.HostRedis0) {
		  t.Fatalf("redis %s", cfg.RedisAddr)
	   }
	  if len(cfg.KafkaBrokers) == 0 || !strings.Contains(cfg.KafkaBrokers[0], config.HostKafka0) {
		   t.Fatalf("kafka %v", cfg.KafkaBrokers)
	   }
	  if config.AllowOutbound("https://mysql") || config.AllowOutbound("https://redis") {
		  t.Fatal("compose service names must not be outbound targets")
	  }
   }
