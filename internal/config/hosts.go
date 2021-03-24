// 内部服务主机和外呼 URL 解析。
// 作者: King
// 日期: 2021.03.23
package config

import (
	"fmt"
	"hash/fnv"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	HostAccountBook     = "account-book.kucoin.net"
	HostLedger          = "ledger-transfer-freeze-reconciliation.kucoin.net"
	HostDepositWithdraw = "deposit-withdraw.kucoin.net"
	HostDocs            = "docs.kucoin.net"
	HostAddress         = "address.kucoin.net"
	HostCustody         = "custody-boundary-lab.kucoin.net"
	HostKYT             = "kyt.kucoin.net"
	HostQueryGateway    = "query-gateway.kucoin.net"
	HostKMSVault        = "kms-vault.kucoin.net"
	HostIAMGateway      = "iam-gateway.kucoin.net"
	HostRiskPortal      = "risk-portal.kucoin.net"
	HostEtcd0           = "etcd-0.kucoin.net"
	HostEtcd1           = "etcd-1.kucoin.net"
	HostEtcd2           = "etcd-2.kucoin.net"
	HostJaeger          = "jaeger.kucoin.net"
	HostPostgres        = "pg-0.kucoin.net"
	HostPostgres1       = "pg-1.kucoin.net"
	HostMySQL0          = "mysql-0.kucoin.net"
	HostMySQL1          = "mysql-1.kucoin.net"
	HostRedis0          = "redis-0.kucoin.net"
	HostRedis1          = "redis-1.kucoin.net"
	HostRedis2          = "redis-2.kucoin.net"
	HostKafka0          = "kafka-0.kucoin.net"
	HostKafka1          = "kafka-1.kucoin.net"
	HostKafka2          = "kafka-2.kucoin.net"
	HostConvert         = "convert.kucoin.net"
	HostStmtExport      = "statement-export.kucoin.net"
	HostSecurity        = "security.kucoin.net"
	HostSubAccount      = "sub-account.kucoin.net"
	HostNotify          = "notify.kucoin.net"
	HostConfigCenter    = "config-center.kucoin.net"
	HostFundSurface     = "fund-surface.kucoin.net"
	HostFiatOnramp      = "fiat-onramp.kucoin.net"
	HostP2PFunding      = "p2p-funding.kucoin.net"
	HostUserAccount     = "user-account.kucoin.net"
	HostAccountProduct  = "account-product.kucoin.net"
	HostFundingBFF      = "funding-bff.kucoin.net"
	HostFiatOrchestrate = "fiat-orchestration.kucoin.net"
	HostComplianceOps   = "compliance-ops.kucoin.net"
	HostMarginFutures   = "margin-futures-account.kucoin.net"
	HostFeeVipRebate    = "fee-vip-rebate.kucoin.net"
	HostNotifyActivity  = "notify-activity.kucoin.net"
	HostConfigPolicy    = "config-policy.kucoin.net"
	HostRestoreGate     = "restore-gate.kucoin.net"
	HostForbidGate      = "forbid-gate.kucoin.net"
	HostHopCompGate     = "hop-comp-gate.kucoin.net"
	PathHealthz         = "/healthz"
	PathWalletChains    = "/v1/wallet/chains"
	PathLedgerTransfers = "/v1/ledger/transfers"
)

type HostRecord struct {
	Host string
	SG   string
	TYO  string
}

func CatalogHosts() []string {
	return []string{
		HostAccountBook,
		HostLedger,
		HostDepositWithdraw,
		HostDocs,
		HostAddress,
		HostCustody,
		HostKYT,
		HostQueryGateway,
		HostKMSVault,
		HostIAMGateway,
		HostRiskPortal,
		HostEtcd0,
		HostEtcd1,
		HostEtcd2,
		HostJaeger,
		HostPostgres,
		HostPostgres1,
		HostMySQL0,
		HostMySQL1,
		HostRedis0,
		HostRedis1,
		HostRedis2,
		HostKafka0,
		HostKafka1,
		HostKafka2,
		HostConvert,
		HostStmtExport,
		HostSecurity,
		HostSubAccount,
		HostNotify,
		HostConfigCenter,
		HostFundSurface,
		HostFiatOnramp,
		HostP2PFunding,
		HostUserAccount,
		HostAccountProduct,
		HostFundingBFF,
		HostFiatOrchestrate,
		HostComplianceOps,
		HostMarginFutures,
		HostFeeVipRebate,
		HostNotifyActivity,
		HostConfigPolicy,
		HostRestoreGate,
		HostForbidGate,
		HostHopCompGate,
	}
}

// HostRecords 目录主机 A 记录：新加坡主、东京备。

func HostRecords() []HostRecord {
	return []HostRecord{
		{HostAccountBook, "10.18.0.30", "10.19.0.30"},
		{HostLedger, "10.18.0.31", "10.19.0.31"},
		{HostDepositWithdraw, "10.18.0.32", "10.19.0.32"},
		{HostDocs, "10.18.0.49", "10.19.0.49"},
		{HostAddress, "10.18.0.33", "10.19.0.33"},
		{HostCustody, "10.18.0.34", "10.19.0.34"},
		{HostKYT, "10.18.0.35", "10.19.0.35"},
		{HostQueryGateway, "10.18.0.36", "10.19.0.36"},
		{HostKMSVault, "10.18.0.37", "10.19.0.37"},
		{HostIAMGateway, "10.18.0.38", "10.19.0.38"},
		{HostRiskPortal, "10.18.0.21", "10.19.0.21"},
		{HostEtcd0, "10.18.0.10", "10.19.0.10"},
		{HostEtcd1, "10.18.0.14", "10.19.0.14"},
		{HostEtcd2, "10.18.0.15", "10.19.0.15"},
		{HostJaeger, "10.18.0.11", "10.19.0.11"},
		{HostPostgres, "10.18.0.4", "10.19.0.4"},
		{HostPostgres1, "10.18.0.5", "10.19.0.5"},
		{HostMySQL0, "10.18.0.40", "10.19.0.40"},
		{HostMySQL1, "10.18.0.41", "10.19.0.41"},
		{HostRedis0, "10.18.0.12", "10.19.0.12"},
		{HostRedis1, "10.18.0.13", "10.19.0.13"},
		{HostRedis2, "10.18.0.16", "10.19.0.16"},
		{HostKafka0, "10.18.0.20", "10.19.0.20"},
		{HostKafka1, "10.18.0.22", "10.19.0.22"},
		{HostKafka2, "10.18.0.23", "10.19.0.23"},
		{HostConvert, "10.18.0.39", "10.19.0.39"},
		{HostStmtExport, "10.18.0.42", "10.19.0.42"},
		{HostSecurity, "10.18.0.43", "10.19.0.43"},
		{HostSubAccount, "10.18.0.44", "10.19.0.44"},
		{HostNotify, "10.18.0.45", "10.19.0.45"},
		{HostConfigCenter, "10.18.0.46", "10.19.0.46"},
		{HostFundSurface, "10.18.0.47", "10.19.0.47"},
		{HostFiatOnramp, "10.18.0.48", "10.19.0.48"},
		{HostP2PFunding, "10.18.0.51", "10.19.0.51"},
		{HostUserAccount, "10.18.0.55", "10.19.0.55"},
		{HostAccountProduct, "10.18.0.56", "10.19.0.56"},
		{HostFundingBFF, "10.18.0.57", "10.19.0.57"},
		{HostFiatOrchestrate, "10.18.0.58", "10.19.0.58"},
		{HostComplianceOps, "10.18.0.59", "10.19.0.59"},
		{HostMarginFutures, "10.18.0.60", "10.19.0.60"},
		{HostFeeVipRebate, "10.18.0.61", "10.19.0.61"},
		{HostNotifyActivity, "10.18.0.62", "10.19.0.62"},
		{HostConfigPolicy, "10.18.0.63", "10.19.0.63"},
		{HostRestoreGate, "10.18.0.52", "10.19.0.52"},
		{HostForbidGate, "10.18.0.53", "10.19.0.53"},
		{HostHopCompGate, "10.18.0.54", "10.19.0.54"},
	}
}

// CatalogRegionIP 判断 IPv4 是否落在新加坡 10.18.0.0/16 或东京 10.19.0.0/16。
func CatalogRegionIP(ip string) bool {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return false
	}
	_, sg, err1 := net.ParseCIDR("10.18.0.0/16")
	_, tyo, err2 := net.ParseCIDR("10.19.0.0/16")
	if err1 != nil || err2 != nil {
		return false
	}
	return sg.Contains(p) || tyo.Contains(p)
}

// CatalogPrimaryIP 判断 IPv4 是否落在新加坡主写段 10.18.0.0/16。

func CatalogPrimaryIP(ip string) bool {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return false
	}
	_, sg, err := net.ParseCIDR("10.18.0.0/16")
	return err == nil && sg.Contains(p)
}

// ClusterEndpointHost 从 DSN / URL / host:port 取出主机名或 IP。
func ClusterEndpointHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "tcp(") {
		return mysqlTCPHost(raw)
	}
	return hostnameOf(raw)
}

// ClusterEndpointOK IP 必须属于目录区域段；主机名不在此闸。
func ClusterEndpointOK(raw string) bool {
	h := ClusterEndpointHost(raw)
	if h == "" {
		return true
	}
	if p := net.ParseIP(h); p != nil {
		return CatalogRegionIP(p.String())
	}
	return true
}

func ResolveA(host, region string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	reg := strings.ToLower(strings.TrimSpace(region))
	for _, r := range HostRecords() {
		if r.Host != host {
			continue
		}
		if reg == "tyo" || reg == "tokyo" || reg == "b" {
			return r.TYO
		}
		return r.SG
	}
	return ""
}

func HTTPS(host string) string {
	return "https://" + host
}

// PickHost 按种子选择一台内部主机，同一种子结果稳定。
func PickHost(seed string) string {
	hosts := CatalogHosts()
	if len(hosts) == 0 {
		return ""
	}
	return hosts[int(hashSeed(seed))%len(hosts)]
}

// PickUnique 给一组种子各抽一台互不重复的内部主机。
func PickUnique(seeds []string) []string {
	hosts := CatalogHosts()
	if len(hosts) == 0 || len(seeds) == 0 {
		return nil
	}
	used := make(map[int]bool, len(seeds))
	out := make([]string, len(seeds))
	for i, seed := range seeds {
		start := int(hashSeed(seed)) % len(hosts)
		for k := 0; k < len(hosts); k++ {
			idx := (start + k) % len(hosts)
			if used[idx] {
				continue
			}
			used[idx] = true
			out[i] = hosts[idx]
			break
		}
	}
	return out
}

func infraHosts() (mysql, redis, kafka string) {
	return HostMySQL0, HostRedis0, HostKafka0
}

// PickEndpoints 按种子连续取 n 个 https 端点，供读路径双活。

func PickEndpoints(seed string, n int) []string {
	hosts := CatalogHosts()
	if n <= 0 || len(hosts) == 0 {
		return nil
	}
	if n > len(hosts) {
		n = len(hosts)
	}
	h := hashSeed(seed)
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, HTTPS(hosts[int(h+uint32(i))%len(hosts)]))
	}
	return out
}

func hashSeed(seed string) uint32 {
	f := fnv.New32a()
	_, _ = f.Write([]byte(seed))
	return f.Sum32()
}

func isLoopbackOrIP(host string) bool {
	if host == "" || host == "localhost" || host == "localhost.localdomain" {
		return true
	}
	return net.ParseIP(host) != nil
}

func isIntranetHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || isLoopbackOrIP(host) {
		return true
	}
	for _, suf := range []string{".kucoin.net", ".local", ".corp", ".internal", ".lan", ".intranet"} {
		if strings.HasSuffix(host, suf) {
			return true
		}
	}
	return false
}

// CatalogHost 判断主机是否属于内部服务表。

func CatalogHost(raw string) bool {
	host := hostnameOf(raw)
	if isLoopbackOrIP(host) {
		return false
	}
	for _, h := range CatalogHosts() {
		if h == host {
			return true
		}
	}
	return false
}

// PublicHost 判断是否为公网主机名。
func PublicHost(raw string) bool {
	host := hostnameOf(raw)
	if host == "" || isLoopbackOrIP(host) || isIntranetHost(host) {
		return false
	}
	return strings.Contains(host, ".")
}

// AllowOutbound 外呼前校验目标主机是否可访问。

func AllowOutbound(raw string) bool {
	return CatalogHost(raw) || PublicHost(raw)
}

func hostnameOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(raw)
}

// FilterOutboundURLs 去掉不可外呼的地址。
func FilterOutboundURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if AllowOutbound(u) {
			out = append(out, u)
		}
	}
	return out
}

// FilterCatalogURLs 只保留内部服务表中的地址。
func FilterCatalogURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if CatalogHost(u) {
			out = append(out, u)
		}
	}
	return out
}

func CatalogURLOrEmpty(raw string) string {
	if CatalogHost(raw) {
		return raw
	}
	return ""
}

// ResolveServiceURL 得到某内部服务的最终 URL。
// 公网地址和目录主机原样保留。空地址按服务名落到固定目录主机，不再在整张目录表里哈希乱挑。
func ResolveServiceURL(raw, seed, path string) string {
	raw = strings.TrimSpace(raw)
	if PublicHost(raw) || CatalogHost(raw) {
		return raw
	}
	host := namedCatalogHost(seed)
	if host == "" {
		host = PickHost(seed)
	}
	return HTTPS(host) + path
}

func namedCatalogHost(seed string) string {
	switch strings.ToLower(strings.TrimSpace(seed)) {
	case "iam", "iam-jwks":
		return HostIAMGateway
	case "kms":
		return HostKMSVault
	case "kyt":
		return HostKYT
	default:
		return ""
	}
}

func pickChainRPC(chain string) []string {
	base := PickEndpoints(chain, 2)
	out := make([]string, 0, len(base))
	for _, u := range base {
		out = append(out, u+"/rpc/"+chain)
	}
	return out
}

// ChainEndpoints 返回该链 JSON-RPC 列表，先用配置里的节点。

func ChainEndpoints(cfg *Config, chain string) []string {
	if cfg != nil {
		if c, ok := cfg.Chains[chain]; ok && len(c.Endpoints) > 0 {
			kept := FilterPublicRPCEndpoints(c.Endpoints)
			if len(kept) > 0 {
				return kept
			}
		}
	}
	return pickChainRPC(chain)
}

// ChainExplorer 返回该链区块浏览器基址，只读配置。
func ChainExplorer(cfg *Config, chain string) string {
	if cfg == nil {
		return ""
	}
	c, ok := cfg.Chains[chain]
	if !ok {
		return ""
	}
	return strings.TrimSpace(c.Explorer)
}

func ChainTokens(cfg *Config, chain string) map[string]TokenSpec {
	if cfg == nil {
		return nil
	}
	c, ok := cfg.Chains[chain]
	if !ok || len(c.Tokens) == 0 {
		return nil
	}
	return c.Tokens
}

func IndexerURL(cfg *Config) string {
	raw := ""
	if cfg != nil {
		raw = cfg.Indexer.URL
	}
	return ResolveServiceURL(raw, "tx-indexer", "/v1/indexer/tx-history")
}

func fillChainEndpoints(cfg *Config) {
	if cfg.Chains == nil {
		cfg.Chains = map[string]ChainConfig{}
	}
	for name, c := range cfg.Chains {
		c.Endpoints = ChainEndpoints(cfg, name)
		cfg.Chains[name] = c
	}
}

func fillExternalURLs(cfg *Config) {
	cfg.KMS.Endpoint = ResolveServiceURL(cfg.KMS.Endpoint, "kms", "")
	cfg.KYT.VendorURL = ResolveServiceURL(cfg.KYT.VendorURL, "kyt", "/v1/kyt/screen")
	cfg.Auth.Issuer = ResolveServiceURL(cfg.Auth.Issuer, "iam", "")
	if PublicHost(cfg.Auth.JWKSURL) || CatalogHost(cfg.Auth.JWKSURL) {
	} else if PublicHost(cfg.Auth.Issuer) || CatalogHost(cfg.Auth.Issuer) {
		cfg.Auth.JWKSURL = strings.TrimRight(cfg.Auth.Issuer, "/") + "/.well-known/jwks.json"
	} else {
		cfg.Auth.JWKSURL = ResolveServiceURL(cfg.Auth.JWKSURL, "iam-jwks", "/.well-known/jwks.json")
	}
	cfg.Indexer.URL = IndexerURL(cfg)
	fillInfraHosts(cfg)
}

func fillInfraHosts(cfg *Config) {
	mysqlH, redisH, kafkaH := infraHosts()
	cfg.MySQLDSN = rewriteMySQLHost(cfg.MySQLDSN,
		mysqlH)
	cfg.RedisAddr = rewriteHostPort(cfg.RedisAddr, redisH, 16379)
	out := make([]string, 0, len(cfg.KafkaBrokers))
	if len(cfg.KafkaBrokers) == 0 {
		out = append(out, rewriteHostPort("", kafkaH, 19092))
	}
	for _, b := range cfg.KafkaBrokers {
		out = append(out, rewriteHostPort(b, kafkaH, 19092))
	}
	cfg.KafkaBrokers = out
}

func rewriteHostPort(raw, picked string, defaultPort int) string {
	raw = strings.TrimSpace(raw)
	host, port := raw, defaultPort
	if raw != "" {
		if h, p, err := net.SplitHostPort(raw); err == nil {
			host, port = h, atoiDefault(p, defaultPort)
		} else {
			host = hostnameOf(raw)
		}
	}
	if host != "" && PublicHost(host) {
		return raw
	}
	return net.JoinHostPort(picked, strconv.Itoa(port))
}

func rewriteMySQLHost(dsn, picked string) string {
	dsn = strings.TrimSpace(dsn)
	host := mysqlTCPHost(dsn)
	if host != "" && PublicHost(host) {
		return dsn
	}
	i := strings.Index(dsn, "tcp(")
	j := strings.Index(dsn, ")")
	if i >= 0 && j > i {
		inner := dsn[i+4 : j]
		port := "13306"
		if _, p, err := net.SplitHostPort(inner); err == nil && p != "" {
			port = p
		}
		return dsn[:i+4] + net.JoinHostPort(picked, port) + dsn[j:]
	}
	return fmt.Sprintf("spotwallet:localdev@tcp(%s:13306)/spotwallet?parseTime=true&charset=utf8mb4&multiStatements=true", picked)
}

func mysqlTCPHost(dsn string) string {
	i := strings.Index(dsn, "tcp(")
	j := strings.Index(dsn, ")")
	if i >= 0 && j > i {
		return hostnameOf(dsn[i+4 : j])
	}
	return hostnameOf(dsn)
}

func atoiDefault(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
