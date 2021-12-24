package svcdial

// javaHTTP 这些注册名的对端是 Java 进程。调用仍用 HTTP，服务名、路径和参数保持原样。
func javaHTTP(service string) bool {
	switch service {
	case "user-account-service",
		"account-product-service",
		"funding-bff-service",
		"fiat-orchestration-service",
		"compliance-ops-service",
		"margin-futures-account-service",
		"fee-vip-rebate-service",
		"notify-activity-service",
		"config-policy-service":
		return true
	default:
		return false
	}
}
