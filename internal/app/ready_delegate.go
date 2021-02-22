package app

import (
	"context"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/platformjava"
)

// ShouldProbeJavaDelegate 在 query-gateway / 单体 api 或显式开关时探测 Java 契约面。

   func ShouldProbeJavaDelegate(serviceName string) bool {
	   if os.Getenv("KCFUND_READY_DELEGATE_PING") == "1" {
		   return true
	  }
	   switch strings.TrimSpace(serviceName) {
	   case "query-gateway", "api":

// MergeJavaDelegateReady 将 funding-bff、account-product 探测结果并入 /readyz。
  func MergeJavaDelegateReady(ctx context.Context, rt *Runtime, body map[string]any, code *int) {
	  if rt == nil || body == nil || code == nil || rt.Mem || rt.Dial == nil {
		   return
	  }
	  client := platformjava.New(rt.Dial)
	  targets := []string{platformjava.ServiceFundingBFF, platformjava.ServiceAccountProduct}
	   rows := make([]map[string]any, 0, len(targets))
	   allOK := true
	  for _, svc := range targets {
		   pr := platformjava.PingHealth(ctx, client, svc)
		   row := map[string]any{"service": svc, "ok": pr.OK()}
		   if pr.StatusCode > 0 {
			   row["status"] = pr.StatusCode
		  }
		  if pr.Err != nil {
			   row["error"] = pr.Err.Error()
		  }
		   rows = append(rows, row)
		   if !pr.OK() {
			  allOK = false
		  }
	  }
	  body["java_delegate"] = map[string]any{"ok": allOK, "targets": rows}
	  if !allOK && *code == 200 {
		   *code = 503
		  body["ok"] = false
	  }
   }
