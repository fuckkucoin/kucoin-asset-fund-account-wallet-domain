package fundmesh_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundmesh"
	"github.com/gin-gonic/gin"
)

   func loadJSON(t *testing.T, name string) map[string]any {
	   t.Helper()
	   p := filepath.Join("testdata", "fundmesh", name)
	   if _, err := os.Stat(p); err != nil {
		  p = filepath.Join("..", "..", "testdata", "fundmesh", name)
	  }
	  b, err := os.ReadFile(p)
	  if err != nil {
		  t.Fatalf("golden %s: %v", err, name)
	   }
	  var m map[string]any
	  if err := json.Unmarshal(b, &m); err != nil {
		  t.Fatalf("json %s: %v", name, err)
	   }
	   return m
  }

   func seedFrozen(eng *fundmesh.Engine, id, uid string) {
	   if rem, ok := eng.Remote().(*fundmesh.MemRemoteLedger); ok {
		   rem.Put(fundmesh.WithdrawView{ID: id, UserID: uid, Phase: fundmesh.WithdrawFrozen})
	   }
   }

  func seedPhase(eng *fundmesh.Engine, id, uid string, p fundmesh.WithdrawPhase) {
	   if rem, ok := eng.Remote().(*fundmesh.MemRemoteLedger); ok {
		  rem.Put(fundmesh.WithdrawView{ID: id, UserID: uid, Phase: p})
	   }
  }

   func newHTTP(eng *fundmesh.Engine) *gin.Engine {
	  gin.SetMode(gin.ReleaseMode)
	  r := gin.New()
	  v1 := r.Group("/v1")
	   fundmesh.RegisterWire(v1, eng, callerid.Opt{Mem: true, Seed: "u1"})
	   fundmesh.RegisterObserveRoot(r)
	  fundmesh.BindEngine(eng)
	  return r
  }

   func do(r http.Handler, method, path string, body any, uid string) *httptest.ResponseRecorder {
	   var rd io.Reader
	  if body != nil {
		  b, _ := json.Marshal(body)
		   rd = bytes.NewReader(b)
	   }
	   req := httptest.NewRequest(method, path, rd)
	   if body != nil {
		   req.Header.Set("Content-Type", "application/json")
	   }
	   req.Header.Set("X-User-Id", uid)
	  req.Header.Set("X-Role", "user")
	  w := httptest.NewRecorder()
	   r.ServeHTTP(w, req)
	   return w
   }

   func cancelReq(id, entry string) fundmesh.CancelRequest {
	   e := fundmesh.EntryPOST
	   m := http.MethodPost
	  if entry == "DELETE" {
		   e = fundmesh.EntryDELETE
		   m = http.MethodDelete
	  }
	   return fundmesh.CancelRequest{WithdrawID: id, UserID: "u1", Actor: "u1", Reason: "cancel", Entry: e, Method: m}
   }

// B-T01 DELETE 与 POST 同源同一 CancelWithdraw。
  func TestBT01DeletePostSameCancelWithdraw(t *testing.T) {
	  g := loadJSON(t, "cancel_matrix.json")
	   want, _ := json.Marshal(g["gen"])
	   got, _ := json.Marshal(fundmesh.CancelMatrixRows())
	  if string(want) != string(got) {
		   t.Fatalf("matrix\nwant %s\ngot %s", want, got)
	   }
	   parity, _ := json.Marshal(g["parity"])
	  gotp,
		   _ := json.Marshal(fundmesh.EntryParityRows())
	   if string(parity) != string(gotp) {
		  t.Fatalf("parity\nwant %s\ngot %s", parity, gotp)
	   }
	   eng := fundmesh.NewMemEngine()
	  seedFrozen(eng, "wd-a", "u1")
	   seedFrozen(eng, "wd-b", "u1")
	  ctx := context.Background()
	   del, err := eng.CancelWithdraw(ctx, cancelReq("wd-a", "DELETE"))
	  if err != nil {
		  t.Fatal(err)
	  }
	   post, err := eng.CancelWithdraw(ctx, cancelReq("wd-b", "POST"))
	   if err != nil {
		  t.Fatal(err)
	   }
	   if del.Code != post.Code || del.Code != fundmesh.CodeOK {
		  t.Fatalf("code delete=%s post=%s", del.Code, post.Code)
	   }
	  if !fundmesh.SameCancelCode(del, post) {
		   t.Fatal("entry must not fork code")
	  }
	   r := newHTTP(eng)
	  seedFrozen(eng, "wd-http-d", "u1")
	  seedFrozen(eng, "wd-http-p", "u1")
	   wd := do(r, http.MethodDelete, "/v1/funding/withdrawals/wd-http-d", nil, "u1")
	  wp := do(r, http.MethodPost, "/v1/funding/withdrawals/wd-http-p/cancel", map[string]string{"reason": "cancel"}, "u1")
	   if wd.Code != http.StatusOK || wp.Code != http.StatusOK {
		   t.Fatalf("http delete=%d post=%d bodyD=%s bodyP=%s", wd.Code, wp.Code, wd.Body.String(), wp.Body.String())
	  }
   }

// B-T02 幂等：已取消再取消同码。
  func TestBT02IdempotentCancel(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	  seedFrozen(eng, "wd-id", "u1")
	  ctx := context.Background()
	  first, err := eng.CancelWithdraw(ctx, cancelReq("wd-id", "DELETE"))
	  if err != nil {
		   t.Fatal(err)
	   }
	   second, err := eng.CancelWithdraw(ctx, cancelReq("wd-id", "POST"))
	  if err != nil {
		  t.Fatal(err)
	  }
	  if first.Code != fundmesh.CodeOK || second.Code != fundmesh.CodeIdempotent {
		   t.Fatalf("first=%s second=%s", first.Code, second.Code)
	  }
	  if !second.Idempotent {
		  t.Fatal("second must be idempotent")
	  }
  }

// B-T03 终态错误码双入口一致。
  func TestBT03TerminalCodesParity(t *testing.T) {
	  cases := []struct {
		  phase fundmesh.WithdrawPhase
		   code string
		   err error
	  }{
		   {fundmesh.WithdrawBroadcast, fundmesh.CodeBroadcast, fundmesh.ErrAlreadyBroadcast},
		  {fundmesh.WithdrawConfirmed, fundmesh.CodeConfirmed, fundmesh.ErrAlreadyConfirmed},
		   {fundmesh.WithdrawFailed, fundmesh.CodeFailed, fundmesh.ErrAlreadyFailed},
	  }
	  for _, tc := range cases {
		  eng := fundmesh.NewMemEngine()
		   seedPhase(eng, "wd-t", "u1", tc.phase)
		  ctx := context.Background()
		   a, errA := eng.CancelWithdraw(ctx, cancelReq("wd-t", "DELETE"))
		   b, errB := eng.CancelWithdraw(ctx, cancelReq("wd-t", "POST"))
		  if !errors.Is(errA, tc.err) || !errors.Is(errB, tc.err) {
			   t.Fatalf("%s err A=%v B=%v", tc.phase, errA, errB)
		   }
		  if a.Code != tc.code || b.Code != tc.code {
			   t.Fatalf("%s code A=%s B=%s", tc.phase, a.Code, b.Code)
		   }
		   if fundmesh.HTTPStatusOf(errA) != fundmesh.HTTPStatusOf(errB) || fundmesh.HTTPStatusOf(errA) != http.StatusConflict {
			   t.Fatalf("status fork %s", tc.phase)
		   }
	   }
   }

// B-T04 /observe/ping 无资金写。
  func TestBT04ObservePingNoWrite(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	   seedFrozen(eng, "wd-keep", "u1")
	   before := eng.ObserveWrites()
	   p1 := eng.PingObserve()
	   p2 := eng.PingObserve()
	  if !p1.OK || p1.Group != fundmesh.ObserveGroup {
		  t.Fatalf("ping %+v", p1)
	  }
	  if eng.ObserveWrites() != before {
		  t.Fatal("ping must not bump write counter")
	  }
	  if p2.Writes != p1.Writes {
		  t.Fatal("observe writes drifted")
	   }
	  r := newHTTP(eng)
	  w := do(r, http.MethodGet, "/v1/observe/ping", nil, "u1")
	  if w.Code != 200 {
		   t.Fatalf("ping http %d %s", w.Code, w.Body.String())
	  }
	  w2 := do(r, http.MethodGet, "/observe/ping", nil, "u1")
	  if w2.Code != 200 {
		   t.Fatalf("root ping %d", w2.Code)
	   }
	  if rem, ok := eng.Remote().(*fundmesh.MemRemoteLedger); ok && rem.Calls() != 0 {
		   t.Fatal("ping must not cancel remote")
	  }
	   if len(eng.Proofs().ListFallback()) != 0 {
		  t.Fatal("ping must not write fallback proof")
	   }
  }

// B-T05 未注册 sideCall 名失败封闭。
  func TestBT05UnknownSideCallFails(t *testing.T) {
	   g := loadJSON(t, "sidecall_catalog.json")
	   eng := fundmesh.NewMemEngine()
	   unknown, _ := g["unknown_must_fail"].([]any)
	   for _, n := range unknown {
		  name, _ := n.(string)
		  if err := fundmesh.MustKnownSideCall(name); err != fundmesh.ErrSideCallUnknown {
			   t.Fatalf("%s want unknown", name)
		   }
		  _, err := eng.Sides().Call(context.Background(), name)
		  if !errors.Is(err, fundmesh.ErrSideCallUnknown) {
			  t.Fatalf("call %s err=%v", name, err)
		   }
		  if _, err := eng.Sides().Lookup(name); !errors.Is(err, fundmesh.ErrSideCallUnknown) {
			  t.Fatalf("lookup %s", name)
		   }
	  }
	  r := newHTTP(eng)
	  w := do(r, http.MethodPost, "/v1/funding/sidecall/silent-withdraw", nil, "u1")
	  if w.Code != http.StatusBadRequest {
		  t.Fatalf("unknown sidecall http %d %s", w.Code, w.Body.String())
	  }
  }

// B-T06 已注册名可查；无拨号实例失败而不是成功。
   func TestBT06RegisteredSideCallNoSilentOK(t *testing.T) {
	  g := loadJSON(t, "sidecall_catalog.json")
	   names, _ := g["names"].([]any)
	   got := fundmesh.SideCallNames()
	   if len(got) != len(names) {
		  t.Fatalf("catalog size %d want %d", len(got), len(names))
	   }
	  reg := fundmesh.NewSideCallRegistry(nil)
	  _, err := reg.Call(context.Background(), "risk-kyt")
	   if !errors.Is(err, fundmesh.ErrSideCallFailed) {
		  t.Fatalf("nil dialer must fail closed, got %v", err)
	  }
  }

// B-T07 etcd 缺键 → 关键写 fail-closed。
  func TestBT07MissingKeyFailClosed(t *testing.T) {
	  cfg := fundmesh.CompleteMemConfig()
	   cfg.Delete("fundmesh/critical_write_enabled")
	   eng := fundmesh.NewEngine(fundmesh.NewMemRemoteLedger(), fundmesh.NewSnapshotValidator(cfg))
	  seedFrozen(eng, "wd-miss", "u1")
	   _, err := eng.CancelWithdraw(context.Background(), cancelReq("wd-miss", "POST"))
	  if !errors.Is(err, fundmesh.ErrCriticalWriteClosed) {
		   t.Fatalf("want fail-closed, got %v", err)
	   }
	  if rem, ok := eng.Remote().(*fundmesh.MemRemoteLedger); ok && rem.Calls() != 0 {
		   t.Fatal("missing key must not hit ledger")
	   }
   }

// B-T08 配置漂移（静默开关被改）fail-closed。

   func TestBT08ConfigDriftFailClosed(t *testing.T) {
	   g := loadJSON(t, "required_keys.json")
	  keys, _ := g["keys"].([]any)
	  if len(keys) != len(fundmesh.RequiredCriticalKeys()) {
		   t.Fatal("required keys drift from golden")
	  }
	   cfg := fundmesh.CompleteMemConfig()
	   cfg.Put("fundmesh/silent_local_ledger", "true")
	  eng := fundmesh.NewEngine(fundmesh.NewMemRemoteLedger(), fundmesh.NewSnapshotValidator(cfg))
	   seedFrozen(eng, "wd-drift", "u1")
	   res, err := eng.CancelWithdraw(context.Background(), cancelReq("wd-drift", "DELETE"))
	   if !errors.Is(err, fundmesh.ErrCriticalWriteClosed) {
		   t.Fatalf("want closed, got %v", err)
	  }
	   if res.Code != fundmesh.CodeDrift {
		  t.Fatalf("code %s", res.Code)
	  }
   }

// B-T09 远端失败不走进程内 ledger。

   func TestBT09RemoteFailNoLocalFallback(t *testing.T) {
	   remote := fundmesh.NewMemRemoteLedger()
	  remote.Put(fundmesh.WithdrawView{ID: "wd-rf", UserID: "u1", Phase: fundmesh.WithdrawFrozen})
	  remote.Fail(errors.New("ledger unreachable"))
	   local := &fundmesh.CountingLocal{}
	  eng := fundmesh.NewEngine(remote, fundmesh.NewSnapshotValidator(fundmesh.CompleteMemConfig())).WithLocal(local)
	  _, err := eng.CancelWithdraw(context.Background(), cancelReq("wd-rf", "POST"))
	   if !errors.Is(err, fundmesh.ErrLedgerRemoteFailed) {
		   t.Fatalf("want remote fail, got %v", err)
	  }
	  if local.Calls != 0 {
		   t.Fatalf("local ledger called %d", local.Calls)
	   }
	   sum := fundmesh.SummarizeFallback(eng.Proofs().ListFallback())
	   if sum["denied"] == 0 || sum["local_tried"] != 0 {
		  t.Fatalf("proof %+v", sum)
	   }
  }

// B-T10 回退开关关死。

  func TestBT10SilentFallbackSealed(t *testing.T) {
	  g := loadJSON(t, "fallback_seal.json")
	  if fundmesh.AllowSilentLocalLedgerFallback {
		  t.Fatal("switch must be compiled off")
	  }
	  if g["allow_silent_local_ledger_fallback"] != false {
		  t.Fatal("golden seal")
	  }
	  eng := fundmesh.NewMemEngine()
	   seedFrozen(eng, "wd-seal", "u1")
	   if _, err := eng.CancelWithdraw(context.Background(), cancelReq("wd-seal", "DELETE")); err != nil {
		  t.Fatal(err)
	  }
	   t.Setenv("FUNDMESH_SILENT_LOCAL_LEDGER", "true")
	   if err := fundmesh.DefaultPolicy().RejectSilentOpen(); !errors.Is(err, fundmesh.ErrPolicySealed) {
		   t.Fatalf("env must not open switch: %v", err)
	  }
	   if fundmesh.DefaultPolicy().SilentLocalAllowed() {
		   t.Fatal("policy leaked")
	  }
	   seedFrozen(eng, "wd-seal-2", "u1")
	  _, err := eng.CancelWithdraw(context.Background(), cancelReq("wd-seal-2", "POST"))
	  if !errors.Is(err, fundmesh.ErrCriticalWriteClosed) && !errors.Is(err, fundmesh.ErrPolicySealed) {
		   t.Fatalf("forcing env open must fail-closed, got %v", err)
	   }
  }

  func TestInspectAndExportReadOnly(t *testing.T) {
	  cat := fundmesh.InspectCatalog()
	   if cat["silent_local_allowed"] != false {
		  t.Fatal("inspect leaked switch")
	   }
	   eng := fundmesh.NewMemEngine()
	   snap := eng.Snapshot()
	  if snap["write_count"].(int64) != 0 {
		   t.Fatal("inspect wrote")
	  }
	  _ = eng.ExportProofs()
   }

  func TestMissingWithdrawID(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	  _, err := eng.CancelWithdraw(context.Background(), fundmesh.CancelRequest{})
	   if !errors.Is(err, fundmesh.ErrWithdrawIDRequired) {
		   t.Fatalf("got %v", err)
	   }
   }

  func TestActorMismatch(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	  seedFrozen(eng, "wd-x", "owner")
	   req := cancelReq("wd-x", "POST")
	  req.UserID = "other"
	   _, err := eng.CancelWithdraw(context.Background(), req)
	   if !errors.Is(err, fundmesh.ErrActorDenied) {
		   t.Fatalf("got %v", err)
	   }
   }

  func TestBodyCancelAlias(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	  seedFrozen(eng, "wd-body", "u1")
	  r := newHTTP(eng)
	   w := do(r, http.MethodPost, "/v1/funding/withdraw/cancel", map[string]string{"withdrawId": "wd-body"}, "u1")
	   if w.Code != 200 {
		  t.Fatalf("body cancel %d %s", w.Code, w.Body.String())
	  }
   }

   func TestNilConfigClosesWrite(t *testing.T) {
	  eng := fundmesh.NewEngine(fundmesh.NewMemRemoteLedger(),
		  nil)
	   seedFrozen(eng, "wd-n", "u1")
	   _, err := eng.CancelWithdraw(context.Background(), cancelReq("wd-n", "POST"))
	   if !errors.Is(err, fundmesh.ErrCriticalWriteClosed) && !errors.Is(err, fundmesh.ErrEtcdUnreachable) {
		  t.Fatalf("got %v", err)
	   }
   }
