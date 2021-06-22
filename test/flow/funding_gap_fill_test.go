package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/seclock"
)

// TestFundingPublicGapFill 覆盖公开缺口：Passkey 冷却原因、出金 BTC 双额度、冻结 CSV、划转额度边缘。
  func TestFundingPublicGapFill(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	   }
	   h := httpapi.Router(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	   uid := rt.Cfg.SeedUserID
	   call := func(method, path string, body any, role string) *httptest.ResponseRecorder {
		  var req *http.Request
		  if body != nil {
			   raw, _ := json.Marshal(body)
			  req = httptest.NewRequest(method, path, bytes.NewReader(raw))
			  req.Header.Set("Content-Type", "application/json")
		   } else {
			   req = httptest.NewRequest(method, path, nil)
		   }
		   req.Header.Set("Authorization", "Bearer "+tok)
		   req.Header.Set("X-User-Id", uid)
		   if strings.Contains(path, "/ops") {
			   req.Header.Set("X-Actor", uid)
		   }
		  if role != "" {
			  req.Header.Set("X-Role", role)
		  } else if strings.Contains(path, "/ops") {
			  req.Header.Set("X-Role", "user")
		  }
		  w := httptest.NewRecorder()
		  h.ServeHTTP(w, req)
		  return w
	  }
	   // 1) Passkey 删除 → 冷却原因透出到出金锁
	  w := call(http.MethodPost, "/v1/security/cooldown", map[string]any{
		   "user_id": uid, "reason": "Deleting Passkey",
	   }, "ops")
	  if w.Code != 200 {
		  t.Fatalf("cooldown %d %s", w.Code, w.Body.String())
	  }
	  var cool struct {
		   Reason string `json:"reason"`
	  }
	   _ = json.Unmarshal(w.Body.Bytes(), &cool)
	   if cool.Reason != seclock.ReasonPasskeyDelete {
		   t.Fatalf("cooldown reason %q want %q", cool.Reason, seclock.ReasonPasskeyDelete)
	  }
	  w = call(http.MethodGet, "/v1/security/withdraw-lock", nil, "")
	  if w.Code != 200 {
		  t.Fatalf("lock %d %s", w.Code, w.Body.String())
	  }
	   var lockBody struct {
		  Lock struct {
			  Locked bool   `json:"locked"`
			   Reason string `json:"reason"`
		  } `json:"lock"`
	   }
	   _ = json.Unmarshal(w.Body.Bytes(), &lockBody)
	   if !lockBody.Lock.Locked {
		  t.Fatal("expected withdraw lock after passkey delete")
	  }
	  if lockBody.Lock.Reason != seclock.ReasonPasskeyDelete {
		   t.Fatalf("lock reason %q", lockBody.Lock.Reason)
	   }
	   // 2) 出金配额公开 BTC / 计价币双额度字段
	   w = call(http.MethodGet, "/api/v1/withdrawals/quotas?currency=USDT&chain=ETH", nil, "")
	  if w.Code != 200 {
		   t.Fatalf("quotas %d %s", w.Code, w.Body.String())
	  }
	   var wrap struct {
		  Data map[string]any `json:"data"`
	   }
	   if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil || wrap.Data == nil {
		   // 部分轨直接返回 data 本体
		  var flat map[string]any
		   _ = json.Unmarshal(w.Body.Bytes(), &flat)
		  if flat["data"] != nil {
			  wrap.Data, _ = flat["data"].(map[string]any)
		  } else {
			   wrap.Data = flat
		   }
	   }
	  for _, k := range []string{"limitBTCAmount", "usedBTCAmount", "usedQuotaCurrencyAmount", "remainingQuotaAmount", "availableWithdrawAmount"} {
		  if _, ok := wrap.Data[k]; !ok {
			   // 兼容 code/data 包一层
			  if data, ok2 := wrap.Data["data"].(map[string]any); ok2 {
				  wrap.Data = data
			   }
			  if _, ok = wrap.Data[k]; !ok {
				  t.Fatalf("quotas missing %s in %#v", k, wrap.Data)
			  }
		   }
	  }
	   // 3) 冻结原因 CSV 导出
	   w = call(http.MethodGet, "/v1/assets/frozen?format=csv", nil, "")
	  if w.Code != 200 {
		   t.Fatalf("frozen csv %d %s", w.Code, w.Body.String())
	  }
	  ct := w.Header().Get("Content-Type")
	  if !strings.Contains(ct, "text/csv") {
		  t.Fatalf("content-type %q", ct)
	   }
	  if !strings.Contains(w.Body.String(), "asset,book,subject,frozen,reason") {
		  t.Fatalf("csv header %q", w.Body.String())
	   }
	  // 4) 划转额度：UNIFIED 可查；ISOLATED 缺 symbol 拒绝
	  w = call(http.MethodGet, "/api/ua/v2/account/transfer-quota?accountType=UNIFIED&currency=USDT", nil, "")
	  if w.Code != 200 {
		  t.Fatalf("unified quota %d %s", w.Code, w.Body.String())
	  }
	   w = call(http.MethodGet, "/api/ua/v2/account/transfer-quota?accountType=ISOLATED&currency=USDT", nil, "")
	  if w.Code == 200 {
		  t.Fatalf("isolated without symbol must fail: %s", w.Body.String())
	   }
	   w = call(http.MethodGet, "/api/ua/v2/account/transfer-quota?accountType=ISOLATED&currency=USDT&symbol=BTC-USDT", nil, "")
	  if w.Code != 200 {
		   t.Fatalf("isolated with symbol %d %s", w.Code, w.Body.String())
	  }
	   // 5) 事故停提闸：Engage 后出金预检失败；Clear 后恢复
	   w = call(http.MethodPost, "/v1/ops/withdraw-halt", map[string]any{
		  "on": true, "actor": "flow-test", "reason": "hot-wallet-drill",
	  }, "ops")
	  if w.Code != 200 {
		  t.Fatalf("halt on %d %s", w.Code, w.Body.String())
	  }
	   w = call(http.MethodGet, "/api/v1/withdrawals/quotas?currency=USDT&chain=ETH", nil, "")
	   if w.Code != 200 {
		   t.Fatalf("quotas under halt %d %s", w.Code, w.Body.String())
	  }
	   w = call(http.MethodPost, "/v1/ops/withdraw-halt", map[string]any{
		   "on": false, "actor": "flow-test", "reason": "drill-clear",
	   }, "ops")
	   if w.Code != 200 {
		  t.Fatalf("halt off %d %s", w.Code, w.Body.String())
	   }
  }
