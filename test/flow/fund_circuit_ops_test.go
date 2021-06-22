package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundcircuit"
	"account-assets.kucoin.net/funding/internal/usage"
)

// freshCircuitBus 换一条干净的单例总线，并在用例结束后复位。
  func freshCircuitBus(t *testing.T) *fundcircuit.Bus {
	  t.Helper()
	  bus := fundcircuit.New()
	  fundcircuit.SetShared(bus)
	   t.Cleanup(func() { fundcircuit.SetShared(fundcircuit.New()) })
	  return bus
   }

// opsJSON 带 ops 身份发请求，返回状态码与解开的 JSON 体。

   func opsJSON(t *testing.T, rt *app.Runtime, h http.Handler, method, path, actor string, body any) (int, map[string]any) {
	  t.Helper()
	   var req *http.Request
	   if body != nil {
		  raw, err := json.Marshal(body)
		   if err != nil {
			  t.Fatal(err)
		  }
		  req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		  req.Header.Set("Content-Type", "application/json")
	  } else {
		   req = httptest.NewRequest(method, path, nil)
	  }
	  if len(rt.Cfg.Auth.BearerTokens) > 0 {
		  req.Header.Set("Authorization", "Bearer "+rt.Cfg.Auth.BearerTokens[0])
	   }
	  if actor != "" {
		  req.Header.Set("X-Actor", actor)
		   req.Header.Set("X-Role", "ops")
	  }
	   w := httptest.NewRecorder()
	   h.ServeHTTP(w, req)
	   out := map[string]any{}
	   if w.Body.Len() > 0 {
		   if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			   t.Fatalf("%s %s body %q: %v", method, path, w.Body.String(), err)
		  }
	  }
	   return w.Code, out
  }

  func fenceOf(t *testing.T, body map[string]any) (int64, string) {
	  t.Helper()
	   req, ok := body["request"].(map[string]any)
	  if !ok {
		   t.Fatalf("request missing: %+v", body)
	  }
	   fence, ok := req["fence"].(map[string]any)
	  if !ok {
		  t.Fatalf("fence missing: %+v", req)
	  }
	  epoch, _ := fence["Epoch"].(float64)
	   token, _ := fence["Token"].(string)
	  return int64(epoch), token
  }

  func reqIDOf(t *testing.T, body map[string]any) string {
	   t.Helper()
	   req, ok := body["request"].(map[string]any)
	  if !ok {
		  t.Fatalf("request missing: %+v", body)
	   }
	  id, _ := req["id"].(string)
	   if id == "" {
		  t.Fatalf("request id missing: %+v", req)
	   }
	   return id
  }

// TestFundCircuitOpsSurface 运营面全链路：策略、强开、读面、单人拒绝、双人解除与审计。
  func TestFundCircuitOpsSurface(t *testing.T) {
	  bus := freshCircuitBus(t)
	   rt, h := router(t)
	  const user = "u-ops"
	   // 普通用户角色读不到运营面。
	  if code, _ := opsJSON(t, rt, h, http.MethodGet, "/v1/ops/fundcircuit/policy", "", nil); code != http.StatusUnauthorized {
		   t.Fatalf("policy without ops role %d", code)
	  }
	   code, body := opsJSON(t, rt, h, http.MethodGet, "/v1/ops/fundcircuit/policy", "desk-a", nil)
	  if code != http.StatusOK {
		   t.Fatalf("get policy %d %+v", code, body)
	  }
	  pol, ok := body["policy"].(map[string]any)
	   if !ok {
		   t.Fatalf("policy body %+v", body)
	  }
	   if unfreeze, _ := pol["auto_unfreeze"].(bool); unfreeze {
		   t.Fatal("policy advertises auto unfreeze")
	  }
	  // 带解冻语义的策略落版必须被拒。
	  code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/policy", "desk-a", map[string]any{
		   "auto_unfreeze": true, "note": "试图开解冻",
	   })
	   if code != http.StatusForbidden {
		   t.Fatalf("policy with unfreeze accepted %d %+v", code, body)
	   }
	  // 合法策略落版：收紧免评笔数与跳闸分。
	   code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/policy", "desk-a", map[string]any{
		  "path_floor": 2, "trip_score": 80, "warn_score": 60, "watch_score": 30, "halt_score": 200,
		  "note": "收紧刷量阈值",
	  })
	  if code != http.StatusOK {
		  t.Fatalf("put policy %d %+v", code, body)
	  }
	   if bus.Policy().Current().PathFloor != 2 {
		   t.Fatalf("policy not applied: %+v", bus.Policy().Current())
	  }
	   // 强开 user 作用域。
	  code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/force-open", "desk-a", map[string]any{
		   "scope": "user", "user_id": user, "reason": "疑似刷量，人工先挡",
	  })
	   if code != http.StatusOK {
		   t.Fatalf("force open %d %+v", code, body)
	  }
	   key, err := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	   if err != nil {
		  t.Fatal(err)
	  }
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen || !got.Forced {
		  t.Fatalf("circuit %+v", got)
	  }
	   // 读面：三条入口全被挡，且恒标不解冻。
	   code, body = opsJSON(t, rt, h, http.MethodGet, "/v1/ops/fundcircuit/board?user="+user+"&chain=ETH&asset=USDT", "desk-a", nil)
	  if code != http.StatusOK {
		   t.Fatalf("board %d %+v", code, body)
	  }
	   board, _ := body["board"].(map[string]any)
	   rows, _ := board["rows"].([]any)
	  if len(rows) != len(fundcircuit.Paths()) {
		  t.Fatalf("board rows %+v", board)
	  }
	   for _, raw := range rows {
		  row, _ := raw.(map[string]any)
		   if blocked, _ := row["blocked"].(bool); !blocked {
			   t.Fatalf("path not blocked: %+v", row)
		  }
	  }
	   if unfreeze, _ := board["auto_unfreeze"].(bool); unfreeze {
		   t.Fatal("board advertises auto unfreeze")
	  }
	   // 在途相位自检：读面必须拒绝解冻。
	   code, body = opsJSON(t, rt, h, http.MethodGet,
		   "/v1/ops/fundcircuit/unfreeze-check?transfer_id=tr-1&freeze_state=broadcast", "desk-a", nil)
	  if code != http.StatusOK {
		   t.Fatalf("unfreeze check %d %+v", code, body)
	   }
	  if refused, _ := body["refused"].(bool); !refused {
		   t.Fatalf("broadcast unfreeze not refused: %+v", body)
	  }
	  // 单人解除：拒绝。
	  code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/clear", "desk-a", map[string]any{
		   "key": key, "reason": "单人直接解除",
	   })
	   if code != http.StatusForbidden {
		   t.Fatalf("single clear accepted %d %+v", code, body)
	  }
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		  t.Fatalf("circuit changed by rejected clear: %+v", got)
	   }
	  // 双控三步：开单 → 两人投票 → 执行。
	  code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/clear/request", "desk-a", map[string]any{
		   "key": key, "reason": "核实为压测流量",
	   })
	   if code != http.StatusOK {
		  t.Fatalf("clear request %d %+v", code, body)
	  }
	  reqID := reqIDOf(t, body)
	   epoch, token := fenceOf(t, body)
	  code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/clear/vote", "desk-a", map[string]any{
		   "req_id": reqID, "allow": true, "epoch": epoch, "token": token,
	   })
	  if code != http.StatusOK {
		  t.Fatalf("vote a %d %+v", code, body)
	   }
	  epoch, token = fenceOf(t, body)
	   // 票未齐时执行必须失败。
	  code, _ = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/clear/execute", "desk-a", map[string]any{
		  "req_id": reqID, "epoch": epoch, "token": token,
	  })
	   if code == http.StatusOK {
		  t.Fatal("execute without quorum accepted")
	   }
	   code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/clear/vote", "desk-b", map[string]any{
		  "req_id": reqID, "allow": true, "epoch": epoch, "token": token,
	   })
	   if code != http.StatusOK {
		   t.Fatalf("vote b %d %+v", code, body)
	  }
	   epoch, token = fenceOf(t, body)
	   code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/clear/execute", "desk-b", map[string]any{
		   "req_id": reqID, "epoch": epoch, "token": token,
	   })
	  if code != http.StatusOK {
		  t.Fatalf("clear execute %d %+v", code, body)
	   }
	  if unfreeze, _ := body["auto_unfreeze"].(bool); unfreeze {
		  t.Fatal("clear response advertises auto unfreeze")
	   }
	   if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateClosed {
		  t.Fatalf("circuit after dual clear %+v", got)
	  }
	   // 审计留痕：拒绝与执行各一条。
	  code, body = opsJSON(t, rt, h, http.MethodGet, "/v1/ops/fundcircuit/audit", "desk-a", nil)
	  if code != http.StatusOK {
		  t.Fatalf("audit %d %+v", code, body)
	   }
	  events, _ := body["events"].([]any)
	  actions := map[string]int{}
	   for _, raw := range events {
		   ev, _ := raw.(map[string]any)
		  action, _ := ev["action"].(string)
		   actions[action]++
	   }
	  if actions["clear_denied"] == 0 || actions["clear_executed"] == 0 || actions["force_open"] == 0 {
		   t.Fatalf("audit actions %+v", actions)
	  }
	   // 探针配置改版落新策略代次。
	  before := bus.Policy().Current().Version
	  code, body = opsJSON(t, rt, h, http.MethodPost, "/v1/ops/fundcircuit/probe-config", "desk-a", map[string]any{
		   "half_open_probes": 1, "probe_ttl_sec": 90, "cooldown_sec": 120, "note": "缩短半开探针窗口",
	   })
	   if code != http.StatusOK {
		  t.Fatalf("probe config %d %+v", code, body)
	   }
	   if bus.Policy().Current().Version <= before {
		   t.Fatalf("probe config did not bump policy version: %d", bus.Policy().Current().Version)
	  }
	  if bus.Policy().Current().ProbeTTL != 90*time.Second {
		   t.Fatalf("probe ttl %s", bus.Policy().Current().ProbeTTL)
	   }
	  // 矩阵读面列出受保护的在途相位。
	  code, body = opsJSON(t, rt, h, http.MethodGet, "/v1/ops/fundcircuit/matrix", "desk-a", nil)
	   if code != http.StatusOK {
		  t.Fatalf("matrix %d %+v", code, body)
	   }
	  states, _ := body["protected_inflight_states"].([]any)
	   if len(states) == 0 {
		   t.Fatalf("matrix missing protected states: %+v", body)
	  }
  }

// TestFundCircuitUsageSourceFloor 占用表速度超过免评笔数后才产生信号并跳闸。

  func TestFundCircuitUsageSourceFloor(t *testing.T) {
	  bus, _, use := newFCBus(t)
	   ctx := context.Background()
	  const user, chain, asset = "u-floor", "ETH", "USDT"
	  depKey, err := usage.DepVelocityKey(user, chain, asset)
	  if err != nil {
		  t.Fatal(err)
	   }
	  floor := bus.Policy().Current().PathFloor
	   // 免评区间内：占用表有速度但不产生信号，也不跳闸。
	  for i := 0; i < floor; i++ {
		   if err := use.AddDep(depKey, "1"); err != nil {
			   t.Fatal(err)
		   }
	   }
	   if err := bus.AdmitDepositObserve(ctx, user, chain, asset); err != nil {
		  t.Fatalf("within floor blocked: %v", err)
	  }
	  if n := bus.Signals().Len(); n != 0 {
		  t.Fatalf("signals emitted within floor: %d", n)
	   }
	  // 超出免评笔数：入金与出金两条入口各出一条信号，跨路径放大后跳闸。
	   for i := 0; i < 3; i++ {
		  if err := use.AddDep(depKey, "1"); err != nil {
			  t.Fatal(err)
		  }
	   }
	   for i := 0; i < floor+3; i++ {
		   if err := use.Add(usage.KindWithdraw, user, asset, "0xdest", "1"); err != nil {
			  t.Fatal(err)
		  }
	  }
	   usageRows := use.Len()
	  if err := bus.AdmitDepositObserve(ctx, user, chain, asset); err != nil {
		  t.Fatalf("deposit admit above floor: %v", err)
	  }
	  if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); err != nil {
		  t.Fatalf("withdraw admit above floor: %v", err)
	   }
	  if bus.Signals().Len() < 2 {
		   t.Fatalf("usage samples missing: %+v", bus.Signals().Stats())
	  }
	   key, _ := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	   row := bus.Circuits().Snapshot(key)
	  if row.State != fundcircuit.StateOpen {
		  t.Fatalf("usage-driven trip missing: %+v", row)
	  }
	   if !row.CrossPath {
		  t.Fatal("cross path evidence missing on usage-driven trip")
	  }
	  // 总线只读占用表：条数与 dep 速度都没有被改写。
	  if use.Len() != usageRows {
		  t.Fatalf("usage rows mutated: %d -> %d", usageRows, use.Len())
	   }
	  if got := use.DepVelocity(depKey, time.Minute); got != floor+3 {
		  t.Fatalf("dep velocity mutated: %d", got)
	  }
   }
