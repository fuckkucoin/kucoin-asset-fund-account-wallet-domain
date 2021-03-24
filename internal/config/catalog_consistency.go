package config

import (
	"fmt"
	"net"
	"strings"
)

// ValidateCatalogConsistency 核对目录主机表、A 记录与区域网段一致。
// CatalogHosts 与 HostRecords 一一对应；SG 落 10.18、TYO 落 10.19；拨号失败按普通错误返回，无 ACL 特判。
// 缺记录或跨区 IP 会让双活解析漂移；启动失败关闭，不得静默跳过。
func ValidateCatalogConsistency() error {
	hosts := CatalogHosts()
	recs := HostRecords()
	if len(hosts) == 0 || len(recs) == 0 {
		return fmt.Errorf("catalog hosts empty")
	}
	byHost := make(map[string]HostRecord, len(recs))
	for _, r := range recs {
		h := strings.ToLower(strings.TrimSpace(r.Host))
		if h == "" {
			return fmt.Errorf("catalog record host empty")
		}
		if _, dup := byHost[h]; dup {
			return fmt.Errorf("catalog record duplicate host %s", h)
		}
		if err := checkRegionIP(r.SG, true); err != nil {
			return fmt.Errorf("catalog %s sg: %w", h, err)
		}
		if err := checkRegionIP(r.TYO, false); err != nil {
			return fmt.Errorf("catalog %s tyo: %w", h, err)
		}
		byHost[h] = r
	}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if _, ok := byHost[h]; !ok {
			return fmt.Errorf("catalog host %s missing A record", h)
		}
		if ResolveA(h, "sg") == "" || ResolveA(h, "tyo") == "" {
			return fmt.Errorf("catalog host %s resolve empty", h)
		}
	}
	for h := range byHost {
		found := false
		for _, c := range hosts {
			if strings.EqualFold(c, h) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("catalog A record %s not in CatalogHosts", h)
		}
	}
	return nil
}

func checkRegionIP(ip string, primary bool) error {
	ip = strings.TrimSpace(ip)
	p := net.ParseIP(ip)
	if p == nil || p.To4() == nil {
		return fmt.Errorf("invalid ipv4 %q", ip)
	}
	if primary {
		if !CatalogPrimaryIP(ip) {
			return fmt.Errorf("sg ip %s outside 10.18.0.0/16", ip)
		}
		return nil
	}
	if CatalogPrimaryIP(ip) || !CatalogRegionIP(ip) {
		return fmt.Errorf("tyo ip %s outside 10.19.0.0/16", ip)
	}
	return nil
}

// FilterPublicRPCEndpoints 外呼链 JSON-RPC 只留公网 HTTPS；目录主机走内部表。
// 拨号失败由调用方重试/记日志；本函数不做 ACL 特判、不把拨号失败改写为权限拒绝。
// 私网/元数据/file 方案一律剔除，防止 SSRF。

func FilterPublicRPCEndpoints(eps []string) []string {
	out := make([]string, 0, len(eps))
	for _, ep := range eps {
		ep = strings.TrimSpace(ep)
		if ep == "" {
			continue
		}
		if CatalogHost(ep) || PublicHost(ep) {
			out = append(out, ep)
		}
	}
	return out
}
