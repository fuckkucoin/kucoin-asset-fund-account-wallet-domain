package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ServiceDialURL 把目录主机或逻辑主机收成可直接 HTTP 调用的基址。
func ServiceDialURL(raw, httpAddr string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	scheme := ""
	rest := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		scheme = raw[:i]
		rest = raw[i+3:]
	}
	rest = strings.TrimRight(rest, "/")
	if rest == "" {
		return ""
	}
	if h, p, err := net.SplitHostPort(rest); err == nil && h != "" && p != "" {
		if scheme == "" {
			scheme = "http"
		}
		return scheme + "://" + net.JoinHostPort(h, p)
	}
	port := listenPort(httpAddr)
	if port == "" {
		port = "18480"
	}
	return "http://" + net.JoinHostPort(rest, port)
}

func listenPort(httpAddr string) string {
	httpAddr = strings.TrimSpace(httpAddr)
	if httpAddr == "" {
		return ""
	}
	if strings.HasPrefix(httpAddr, ":") {
		return strings.TrimPrefix(httpAddr, ":")
	}
	_, p, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return ""
	}
	return p
}

// GRPCListen 内部 gRPC 监听地址。端口是本进程 HTTP 端口加 1。
func GRPCListen(httpAddr string) string {
	port := listenPort(httpAddr)
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n >= 65535 {
		return ":18481"
	}
	return ":" + strconv.Itoa(n+1)
}

// GRPCTarget 从已注册的 HTTP 基址得到内部 gRPC 地址。主机不变，端口加 1。
func GRPCTarget(httpURL string) (string, error) {
	raw := strings.TrimSpace(httpURL)
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	}
	raw = strings.TrimRight(raw, "/")
	host, port, err := net.SplitHostPort(raw)
	if err != nil || host == "" || port == "" {
		if err == nil {
			err = fmt.Errorf("grpc target: %s", httpURL)
		}
		return "", err
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n >= 65535 {
		if err == nil {
			err = fmt.Errorf("grpc port: %s", port)
		}
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(n+1)), nil
}
