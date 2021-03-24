// 进程配置：限额、超时、手续费和节点阈值。
// 作者: Span
// 日期: 2021.03.23
package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultAuthMaxSkew = 5 * time.Minute
	DefaultSeedUserID  = "user-10001"
	PeerSeedUserID     = "user-10002"
)

type Config struct {
	HTTPAddr             string        `yaml:"http_addr"`
	GRPCAddr             string        `yaml:"grpc_addr"`
	SeedUserID           string        `yaml:"seed_user_id"`
	HTTPWriteTimeout     time.Duration `yaml:"http_write_timeout"`
	MySQLDSN             string
	RedisAddr            string
	KafkaBrokers         []string
	AESKeyB64            string `yaml:"aes_key_b64"`
	RSAKeyPath           string
	LogLevel             string
	KafkaOptional        bool
	Cache                CacheConfig            `yaml:"cache"`
	NodeHealth           NodeHealthConfig       `yaml:"node_health"`
	Risk                 RiskConfig             `yaml:"risk"`
	Swap                 SwapConfig             `yaml:"swap"`
	Transfer             TransferConfig         `yaml:"transfer"`
	Spot                 SpotConfig             `yaml:"spot"`
	Market               MarketConfig           `yaml:"market"`
	MySQL                PoolConfig             `yaml:"mysql"`
	Postgres             PostgresConfig         `yaml:"postgres"`
	Policy               PolicyRuntime          `yaml:"policy"`
	RedisPool            RedisPoolConfig        `yaml:"redis"`
	Worker               WorkerConfig           `yaml:"worker"`
	ProcessRole          string                 `yaml:"process_role"`
	Profile              string                 `yaml:"profile"`
	Storage              string                 `yaml:"storage"`
	Auth                 AuthConfig             `yaml:"auth"`
	Travel               TravelConfig           `yaml:"travel"`
	KYT                  KYTConfig              `yaml:"kyt"`
	KMS                  KMSConfig              `yaml:"kms"`
	BalproofHMAC         string                 `yaml:"balproof_hmac"`
	Chains               map[string]ChainConfig `yaml:"chains"`
	ChainDriver          string                 `yaml:"chain_driver"`
	Indexer              IndexerConfig          `yaml:"indexer"`
	Pprof                bool                   `yaml:"pprof"`
	WalletRevealMnemonic bool                   `yaml:"wallet_reveal_mnemonic"`
	KeySource            string                 `yaml:"key_source"`
	ConvertFill          ConvertFillConfig      `yaml:"convert_fill"`
	Etcd                 EtcdConfig             `yaml:"etcd"`
	Funds                FundsPageConfig        `yaml:"funds"`
	Mesh                 MeshServices           `yaml:"mesh"`
	Hosts                map[string]string      `yaml:"hosts"`
}

type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints"`
	Namespace string   `yaml:"namespace"`
	Env       string   `yaml:"env"`
}

type TokenSpec struct {
	Contract string `yaml:"contract"`
	Decimals int    `yaml:"decimals"`
}

type ChainConfig struct {
	Endpoints []string             `yaml:"endpoints"`
	Explorer  string               `yaml:"explorer"`
	Tokens    map[string]TokenSpec `yaml:"tokens"`
}

type IndexerConfig struct {
	URL     string        `yaml:"url"`
	Timeout time.Duration `yaml:"timeout"`
}

type AuthConfig struct {
	Enabled       bool              `yaml:"enabled"`
	AllowInsecure bool              `yaml:"allow_insecure"`
	Kind          string            `yaml:"kind"`
	Issuer        string            `yaml:"issuer"`
	JWKSURL       string            `yaml:"jwks_url"`
	Audience      string            `yaml:"audience"`
	BearerTokens  []string          `yaml:"bearer_tokens"`
	HMACKeys      map[string]string `yaml:"hmac_keys"`
	MaxSkew       time.Duration     `yaml:"max_skew"`
}

type TravelConfig struct {
	OriginVASP       string                    `yaml:"origin_vasp"`
	PeerVASP         string                    `yaml:"peer_vasp"`
	DefaultThreshold string                    `yaml:"default_threshold"` // 未命中走廊时的最低金额；挂牌价后按 USD 名义；空或 0 表示绑定后一律开报文
	Corridors        []TravelCorridorThreshold `yaml:"corridors"`
	UsdRates         map[string]string         `yaml:"usd_rates"` // 非稳定币→USD 配置牌价，供门槛名义比较
}

// TravelCorridorThreshold 按走廊配置旅行规则金额门槛。
type TravelCorridorThreshold struct {
	Corridor  string `yaml:"corridor"` // SG>US 或 VASP 对 KC01>PEER
	Asset     string `yaml:"asset"`
	MinAmount string `yaml:"min_amount"`
}

type KYTConfig struct {
	Mode          string        `yaml:"mode"`
	VendorURL     string        `yaml:"vendor_url"`
	Timeout       time.Duration `yaml:"timeout"`
	DenyAddresses []string      `yaml:"deny_addresses"`
	ReviewAmount  string        `yaml:"review_amount"`
	DenyAmount    string        `yaml:"deny_amount"`
	DenyChains    []string      `yaml:"deny_chains"`
}

type KMSConfig struct {
	Kind     string        `yaml:"kind"`
	Endpoint string        `yaml:"endpoint"`
	Cluster  string        `yaml:"cluster_id"`
	Timeout  time.Duration `yaml:"timeout"`
	KEKPath  string        `yaml:"kek_path"`
	WrapPath string        `yaml:"wrap_path"`
}

// ConvertFillConfig 闪兑成交回调身份。HMAC 密钥只从环境变量注入。
type ConvertFillConfig struct {
	RequireIdent bool     `yaml:"require_ident"`
	MTLSCNs      []string `yaml:"mtls_cns"`
	HMACSecret   string   `yaml:"-"`
}

type PoolConfig struct {
	MaxOpen         int           `yaml:"max_open"`
	MaxIdle         int           `yaml:"max_idle"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// PostgresConfig 内部库主机 pg-0.kucoin.net；区域地址 SG 10.18 / TYO 10.19。
type PostgresConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpen         int           `yaml:"max_open"`
	MaxIdle         int           `yaml:"max_idle"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdle     time.Duration `yaml:"conn_max_idle"`
}

type PolicyRuntime struct {
	PackDir    string `yaml:"pack_dir"`
	Mode       string `yaml:"mode"`
	FailClosed bool   `yaml:"fail_closed"`
}

type RedisPoolConfig struct {
	PoolSize     int `yaml:"pool_size"`
	MinIdleConns int `yaml:"min_idle_conns"`
}

type WorkerConfig struct {
	ConfirmConcurrency int           `yaml:"confirm_concurrency"`
	ScanLimit          int           `yaml:"scan_limit"`
	Tick               time.Duration `yaml:"tick"`
	OutboxLimit        int           `yaml:"outbox_limit"`
	InspectBatch       int           `yaml:"inspect_batch"`
}

type CacheConfig struct {
	BalanceTTL time.Duration `yaml:"balance_ttl"`
	TickerTTL  time.Duration `yaml:"ticker_ttl"`
}

type NodeHealthConfig struct {
	FailThreshold   int           `yaml:"fail_threshold"`
	RetryAttempts   int           `yaml:"retry_attempts"`
	RetryBackoff    time.Duration `yaml:"retry_backoff"`
	ProbeInterval   time.Duration `yaml:"probe_interval"`
	QueryTimeout    time.Duration `yaml:"query_timeout"`
	ChainRPCTimeout time.Duration `yaml:"chain_rpc_timeout"`
	CircuitReset    time.Duration `yaml:"circuit_reset"`
	HedgeReads      bool          `yaml:"hedge_reads"`
}

type RiskConfig struct {
	MaxSingle                  string        `yaml:"max_single"`
	DailyLimit                 string        `yaml:"daily_limit"`
	DailyWithdrawLimit         string        `yaml:"daily_withdraw_limit"`
	DailyDepositLimit          string        `yaml:"daily_deposit_limit"`
	SuspiciousAmount           string        `yaml:"suspicious_amount"`
	SuspiciousRequiresAdvanced bool          `yaml:"suspicious_requires_advanced"`
	MinAmount                  string        `yaml:"min_amount"`
	AllowlistEnabled           bool          `yaml:"allowlist_enabled"`
	MaxVelocity                int           `yaml:"max_velocity"`
	MaxWithdrawVelocity        int           `yaml:"max_withdraw_velocity"`
	MaxDepositVelocity         int           `yaml:"max_deposit_velocity"`
	VelocityWindow             time.Duration `yaml:"velocity_window"`
	FirstDestinationHalfCap    bool          `yaml:"first_destination_half_cap"`
}

type SwapConfig struct {
	FeeBPS      int           `yaml:"fee_bps"`
	QuoteTTL    time.Duration `yaml:"quote_ttl"`
	SlippageBPS int           `yaml:"slippage_bps"`
	QuoteMaxAge time.Duration `yaml:"quote_max_age"`
}

type TransferConfig struct {
	MockConfirmAfter  time.Duration     `yaml:"mock_confirm_after"`
	LockTTL           time.Duration     `yaml:"lock_ttl"`
	DisableDriftBlock bool              `yaml:"disable_drift_block"`
	StuckFrozen       time.Duration     `yaml:"stuck_frozen"`
	StuckSubmitted    time.Duration     `yaml:"stuck_submitted"`
	ConfirmDepth      int               `yaml:"confirm_depth"`
	WithdrawFeeBPS    int               `yaml:"withdraw_fee_bps"`
	EarnInterestBPS   int               `yaml:"earn_interest_bps"`
	HotMax            map[string]string `yaml:"hot_max"`
	WithdrawCooling   time.Duration     `yaml:"withdraw_cooling"`
	PassThroughHold   time.Duration     `yaml:"pass_through_hold"`
	PassThroughBPS    int               `yaml:"pass_through_bps"`
}

type SpotConfig struct {
	FeeBPS int `yaml:"fee_bps"`
}

type MarketConfig struct {
	TickTopic       string        `yaml:"tick_topic"`
	KlineTopic      string        `yaml:"kline_topic"`
	ProduceInterval time.Duration `yaml:"produce_interval"`
	LagWarn         time.Duration `yaml:"lag_warn"`
	KlineKeep       int           `yaml:"kline_keep"`
}

// Load 读 yaml，再用环境变量覆盖缺省值。
func Load() (*Config, error) {
	path := FindRepoFile(getenv("CONFIG_PATH", "configs/app.yaml"))
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.HTTPAddr = getenv("HTTP_ADDR", cfg.HTTPAddr)
	cfg.GRPCAddr = getenv("GRPC_ADDR", cfg.GRPCAddr)
	if v := getenv("MYSQL_DSN", ""); v != "" {
		cfg.MySQLDSN = v
	}
	if v := getenv("REDIS_ADDR", ""); v != "" {
		cfg.RedisAddr = v
	}
	cfg.AESKeyB64 = getenv("AES_KEY_B64", cfg.AESKeyB64)
	if v := getenv("KEY_SOURCE", ""); v != "" {
		cfg.KeySource = v
	}
	cfg.RSAKeyPath = FindRepoFile(getenv("RSA_PRIVATE_KEY_PATH", cfg.KMS.KEKPath))
	if cfg.RSAKeyPath == "" && !prodKeeperKind(cfg.KMS.Kind) {
		// 仓内样例仅 file/mem 装配可用。云 KMS / PKCS#11 不得再挂上这把演示私钥。
		cfg.RSAKeyPath = FindRepoFile("configs/kms/kek.pem")
	}
	cfg.LogLevel = getenv("LOG_LEVEL", "info")
	cfg.KafkaOptional = getenv("KAFKA_OPTIONAL", "true") == "true"
	if v := getenv("KAFKA_BROKERS", ""); v != "" {
		cfg.KafkaBrokers = []string{v}
	}
	if cfg.HTTPWriteTimeout == 0 {
		cfg.HTTPWriteTimeout = 8 * time.Second
	}
	if cfg.Cache.BalanceTTL == 0 {
		cfg.Cache.BalanceTTL = 3 * time.Second
	}
	if cfg.Transfer.LockTTL == 0 {
		cfg.Transfer.LockTTL = 12 * time.Second
	}
	if cfg.Market.TickTopic == "" {
		cfg.Market.TickTopic = "market.ticks"
	}
	if cfg.Market.KlineTopic == "" {
		cfg.Market.KlineTopic = "market.klines"
	}
	if cfg.Market.LagWarn == 0 {
		cfg.Market.LagWarn = time.Second
	}
	if cfg.NodeHealth.QueryTimeout == 0 {
		cfg.NodeHealth.QueryTimeout = 400 * time.Millisecond
	}
	if cfg.NodeHealth.ChainRPCTimeout == 0 {
		cfg.NodeHealth.ChainRPCTimeout = 180 * time.Millisecond
	}
	if cfg.NodeHealth.CircuitReset == 0 {
		cfg.NodeHealth.CircuitReset = 2 * time.Second
	}
	if cfg.Swap.QuoteMaxAge == 0 {
		cfg.Swap.QuoteMaxAge = 2 * time.Second
	}
	if cfg.Swap.SlippageBPS == 0 {
		cfg.Swap.SlippageBPS = 50
	}
	if cfg.Transfer.StuckFrozen == 0 {
		cfg.Transfer.StuckFrozen = 15 * time.Second
	}
	if cfg.Transfer.StuckSubmitted == 0 {
		cfg.Transfer.StuckSubmitted = 30 * time.Second
	}
	if cfg.Transfer.ConfirmDepth <= 0 {
		cfg.Transfer.ConfirmDepth = 1
	}
	if cfg.Transfer.WithdrawCooling <= 0 {
		cfg.Transfer.WithdrawCooling = 24 * time.Hour
	}
	if cfg.Transfer.PassThroughHold <= 0 {
		cfg.Transfer.PassThroughHold = 15 * time.Minute
	}
	if cfg.Transfer.PassThroughBPS <= 0 {
		cfg.Transfer.PassThroughBPS = 8000
	}
	if cfg.ChainDriver == "" {
		cfg.ChainDriver = getenv("CHAIN_DRIVER", "rpc")
	}
	if cfg.KMS.Kind == "" {
		cfg.KMS.Kind = getenv("KMS_KIND", "cloudhsm")
	}
	if strings.TrimSpace(cfg.KeySource) == "" && prodKeeperKind(cfg.KMS.Kind) {
		cfg.KeySource = "kms"
	}
	if prodKeeperKind(cfg.KMS.Kind) && strings.TrimSpace(os.Getenv("RSA_PRIVATE_KEY_PATH")) == "" && demoKEKPath(cfg.RSAKeyPath) {
		cfg.RSAKeyPath = ""
	}
	if v := getenv("KMS_ENDPOINT", ""); v != "" {
		cfg.KMS.Endpoint = v
	}
	if cfg.KMS.Timeout <= 0 {
		cfg.KMS.Timeout = 2 * time.Second
	}
	if cfg.KMS.KEKPath == "" {
		cfg.KMS.KEKPath = cfg.RSAKeyPath
	} else {
		cfg.KMS.KEKPath = FindRepoFile(cfg.KMS.KEKPath)
	}
	if cfg.KMS.WrapPath == "" {
		cfg.KMS.WrapPath = FindRepoFile("configs/kms/dek.wrap")
	} else {
		cfg.KMS.WrapPath = FindRepoFile(cfg.KMS.WrapPath)
	}
	if cfg.KYT.Mode == "" {
		cfg.KYT.Mode = getenv("KYT_MODE", "vendor")
	}
	if v := getenv("KYT_VENDOR_URL", ""); v != "" {
		cfg.KYT.VendorURL = v
	}
	if cfg.KYT.Timeout <= 0 {
		cfg.KYT.Timeout = 2 * time.Second
	}
	if v := os.Getenv("APP_PROFILE"); v != "" {
		cfg.Profile = v
	}
	if v := getenv("GATE_STORAGE", ""); v != "" {
		cfg.Storage = v
	}
	if cfg.Storage == "" {
		if v := getenv("STORAGE", ""); v != "" {
			cfg.Storage = v
		}
	}
	if cfg.Auth.Kind == "" {
		cfg.Auth.Kind = getenv("AUTH_KIND", "iam")
	}
	if strings.TrimSpace(cfg.Storage) == "" && prodAuthKind(cfg.Auth.Kind, cfg.Profile) {
		cfg.Storage = "sql"
	}
	fillConvertFill(cfg)
	if v := os.Getenv("AUTH_ALLOW_INSECURE"); v != "" {
		cfg.Auth.AllowInsecure = v == "true"
	}
	if v := os.Getenv("WALLET_REVEAL_MNEMONIC"); v != "" {
		cfg.WalletRevealMnemonic = v == "true"
	}
	if v := getenv("IAM_ISSUER", ""); v != "" {
		cfg.Auth.Issuer = v
	}
	if v := getenv("IAM_JWKS_URL", ""); v != "" {
		cfg.Auth.JWKSURL = v
	}
	if cfg.Auth.Audience == "" {
		cfg.Auth.Audience = getenv("IAM_AUDIENCE", "account-assets")
	}
	if v := getenv("AUTH_BEARER", ""); v != "" {
		cfg.Auth.BearerTokens = []string{v}
	}
	if v := getenv("BALPROOF_HMAC_SECRET", ""); v != "" {
		cfg.BalproofHMAC = v
	}
	if v := getenv("TRAVEL_ORIGIN_VASP", ""); v != "" {
		cfg.Travel.OriginVASP = v
	}
	if v := getenv("TRAVEL_PEER_VASP", ""); v != "" {
		cfg.Travel.PeerVASP = v
	}
	if v := getenv("TRAVEL_DEFAULT_THRESHOLD", ""); v != "" {
		cfg.Travel.DefaultThreshold = v
	}
	if cfg.Auth.MaxSkew <= 0 {
		cfg.Auth.MaxSkew = DefaultAuthMaxSkew
	}
	fillChainEndpoints(cfg)
	if err := ValidateCatalogConsistency(); err != nil {
		return nil, fmt.Errorf("catalog hosts: %w", err)
	}
	if cfg.Indexer.Timeout <= 0 {
		cfg.Indexer.Timeout = 2 * time.Second
	}
	fillExternalURLs(cfg)
	if cfg.Risk.VelocityWindow == 0 {
		cfg.Risk.VelocityWindow = time.Minute
	}
	if cfg.MySQL.MaxOpen <= 0 {
		cfg.MySQL.MaxOpen = 16
	}
	if cfg.MySQL.MaxIdle <= 0 {
		cfg.MySQL.MaxIdle = 8
	}
	if cfg.MySQL.ConnMaxLifetime <= 0 {
		cfg.MySQL.ConnMaxLifetime = 5 * time.Minute
	}
	if v := getenv("ACCOUNT_ASSETS_PG_DSN", ""); v != "" {
		cfg.Postgres.DSN = v
	}
	if cfg.Postgres.MaxOpen <= 0 {
		cfg.Postgres.MaxOpen = 16
	}
	if cfg.Postgres.MaxIdle <= 0 {
		cfg.Postgres.MaxIdle = 8
	}
	if cfg.Postgres.ConnMaxLifetime <= 0 {
		cfg.Postgres.ConnMaxLifetime = 5 * time.Minute
	}
	if cfg.Postgres.ConnMaxIdle <= 0 {
		cfg.Postgres.ConnMaxIdle = 2 * time.Minute
	}
	if cfg.Policy.PackDir == "" {
		cfg.Policy.PackDir = getenv("POLICY_PACK_DIR", "configs/policy")
	}
	if cfg.Policy.Mode == "" {
		cfg.Policy.Mode = getenv("POLICY_MODE", "enforce")
	}
	if v := os.Getenv("POLICY_FAIL_CLOSED"); v != "" {
		cfg.Policy.FailClosed = v == "true"
	} else {
		cfg.Policy.FailClosed = true
	}
	if cfg.RedisPool.PoolSize <= 0 {
		cfg.RedisPool.PoolSize = 8
	}
	if cfg.Worker.ConfirmConcurrency <= 0 {
		cfg.Worker.ConfirmConcurrency = 8
	}
	if cfg.Worker.ScanLimit <= 0 {
		cfg.Worker.ScanLimit = 200
	}
	if cfg.Worker.Tick <= 0 {
		cfg.Worker.Tick = 400 * time.Millisecond
	}
	if cfg.Worker.OutboxLimit <= 0 {
		cfg.Worker.OutboxLimit = 100
	}
	if cfg.Worker.InspectBatch <= 0 {
		cfg.Worker.InspectBatch = 32
	}
	if cfg.Market.KlineKeep <= 0 {
		cfg.Market.KlineKeep = 120
	}
	if v := os.Getenv("PROCESS_ROLE"); v != "" {
		cfg.ProcessRole = v
	} else if cfg.ProcessRole == "" {
		cfg.ProcessRole = "all"
	}
	if cfg.SeedUserID == "" {
		cfg.SeedUserID = getenv("SEED_USER_ID", DefaultSeedUserID)
	}
	if v := getenv("ETCD_ENDPOINTS", ""); v != "" {
		cfg.Etcd.Endpoints = splitCSV(v)
	}
	if len(cfg.Etcd.Endpoints) == 0 {
		cfg.Etcd.Endpoints = []string{
			"http://" + HostEtcd0 + ":2379",
			"http://" + HostEtcd1 + ":2379",
			"http://" + HostEtcd2 + ":2379",
		}
	}
	if cfg.Etcd.Namespace == "" {
		cfg.Etcd.Namespace = getenv("ETCD_NAMESPACE", "account-assets")
	}
	if cfg.Etcd.Env == "" {
		cfg.Etcd.Env = getenv("ETCD_ENV", "production")
	}
	fillFundsPage(cfg)
	mergeAccountAssetsCatalog(cfg)
	fillMesh(cfg)
	return cfg, nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string,
		0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// FindRepoFile 从当前目录往上找仓库文件（配置、迁移、密钥材料）。

func FindRepoFile(rel string) string {
	if rel == "" {
		return rel
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	wd, err := os.Getwd()
	if err != nil {
		return rel
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(wd, rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return rel
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func prodKeeperKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "cloud-kms", "cloudkms", "cloud-kms-adapter", "cloudhsm", "cloudhsm-adapter", "pkcs11", "pkcs11-adapter":
		return true
	default:
		return false
	}
}

func prodAuthKind(kind, profile string) bool {
	p := strings.ToLower(strings.TrimSpace(profile))
	if p == "production" || p == "prod" {
		return true
	}
	k := strings.ToLower(strings.TrimSpace(kind))
	return k == "iam" || k == "production" || k == "prod"
}

func demoKEKPath(path string) bool {
	p := filepath.ToSlash(strings.TrimSpace(path))
	if p == "" {
		return false
	}
	if p == "configs/kms/kek.pem" || strings.HasSuffix(p, "/configs/kms/kek.pem") {
		return true
	}
	return strings.HasSuffix(p, "/kms/kek.pem") && strings.Contains(p, "/configs/")
}

func fillConvertFill(cfg *Config) {
	if cfg == nil || !prodAuthKind(cfg.Auth.Kind, cfg.Profile) {
		return
	}
	if v := strings.TrimSpace(os.Getenv("CONVERT_FILL_HMAC_SECRET")); v != "" {
		cfg.ConvertFill.HMACSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("CONVERT_FILL_MTLS_CNS")); v != "" && len(cfg.ConvertFill.MTLSCNs) == 0 {
		cfg.ConvertFill.MTLSCNs = splitCSV(v)
	}
	if !cfg.ConvertFill.RequireIdent {
		req := strings.TrimSpace(os.Getenv("CONVERT_FILL_REQUIRE_IDENT"))
		cfg.ConvertFill.RequireIdent = req == "" || strings.EqualFold(req, "1") || strings.EqualFold(req, "true")
	}
	if len(cfg.ConvertFill.MTLSCNs) == 0 && strings.TrimSpace(cfg.ConvertFill.HMACSecret) == "" {
		cfg.ConvertFill.MTLSCNs = []string{HostConvert}
	}
}
