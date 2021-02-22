package app

import (
	"os"
	"strings"
)

// HTTPListenAddr 解析进程 HTTP 监听地址。
// 先 LISTEN_ADDR，其次 PORT；defaultPort 为未配置时的站内默认（非 8080/8081）。
func HTTPListenAddr(defaultPort string) string {
	if v := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); v != "" {
		return v
	}
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		if strings.HasPrefix(p, ":") {
			return p
		}
		return ":" + p
	}
	dp := strings.TrimSpace(defaultPort)
	if dp == "" {
		dp = "8100"
	}
	if !strings.HasPrefix(dp, ":") {
		dp = ":" + dp
	}
	return dp
}
