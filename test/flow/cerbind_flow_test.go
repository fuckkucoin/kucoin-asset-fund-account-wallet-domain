package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/cerbind"
	"github.com/gin-gonic/gin"
)

   func TestCebindUnsigned(t *testing.T) {
	  if err := cerbind.DrillUnsigned(); err != nil {
		  t.Fatal(err)
	  }
  }

   func TestCebindSignHook(t *testing.T) {
	   if err := cerbind.DrillSignHook(); err != nil {
		  t.Fatal(err)
	  }
  }

  func TestCebindShortQuorum(t *testing.T) {
	   if err := cerbind.DrillShortQuorum(); err != nil {
		  t.Fatal(err)
	   }
  }

  func TestCebindTimeoutAbort(t *testing.T) {
	  if err := cerbind.DrillTimeout(); err != nil {
		   t.Fatal(err)
	  }
   }

  func TestCebindDoubleBind(t *testing.T) {
	  if err := cerbind.DrillDoubleBind(); err != nil {
		   t.Fatal(err)
	   }
  }

  func TestCebindManyBind(t *testing.T) {
	   if err := cerbind.DrillManyBind(); err != nil {
		   t.Fatal(err)
	   }
  }

  func TestCebindAbortReplan(t *testing.T) {
	  if err := cerbind.DrillAbort(); err != nil {
		   t.Fatal(err)
	   }
  }

  func TestCebindExecutors(t *testing.T) {
	  if err := cerbind.DrillExec(); err != nil {
		  t.Fatal(err)
	   }
   }

  func TestCebindFloor(t *testing.T) {
	   if err := cerbind.DrillFloor(); err != nil {
		   t.Fatal(err)
	  }
  }

  func TestCebindHTTP(t *testing.T) {
	   gin.SetMode(gin.TestMode)
	  r := gin.New()
	  eng := cerbind.Register(r.Group("/v1"), nil)
	   defer cerbind.Detach()
	   if eng == nil {
		   t.Fatal("engine")
	  }
	   w := cerReq(t, r, "ops", "dave", http.MethodPost, "/v1/cerbind/policies", map[string]any{
		   "actor": "dave", "id": "cer-pol-http", "name": "http cold", "asset": "*", "chain": "*",
		   "purpose": "withdraw", "min_amount": "0.01", "dest_risk": "any", "side": "cold",
		   "force_cold": true, "threshold": 2, "shares": 3, "bind_mode": "one", "priority": 500, "enabled": true,
	   })
	   if w.Code != 200 {
		   t.Fatalf("policy %d %s", w.Code, w.Body.String())
	  }
	  w = cerReq(t, r, "custody", "alice", http.MethodPost, "/v1/cerbind/sessions/plan", map[string]any{
		  "actor": "alice", "batch_id": "batch-http", "asset": "USDT", "chain": "ETH", "amount": "20",
		   "dest": "0xabc", "side": "cold", "purpose": "withdraw", "item_refs": []string{"idem-http"},
	   })
	  if w.Code != 200 {
		   t.Fatalf("plan %d %s", w.Code, w.Body.String())
	   }
	   var ses cerbind.Session
	  if err := json.Unmarshal(w.Body.Bytes(), &ses); err != nil {
		   t.Fatal(err)
	  }
	   w = cerPostFence(t, r, "custody", "alice", "/v1/cerbind/sessions/open", ses)
	  if w.Code != 200 {
		   t.Fatalf("open %d %s", w.Code, w.Body.String())
	   }
	  if err := json.Unmarshal(w.Body.Bytes(), &ses); err != nil {
		   t.Fatal(err)
	   }
	   w = cerPostSlot(t, r, "alice", "/v1/cerbind/sessions/receipt", ses, "slot-a")
	  if w.Code != 200 {
		  t.Fatalf("receipt alice %d %s", w.Code, w.Body.String())
	   }
	   if err := json.Unmarshal(w.Body.Bytes(), &ses); err != nil {
		  t.Fatal(err)
	   }
	   w = cerPostSlot(t, r, "bob", "/v1/cerbind/sessions/receipt", ses, "slot-b")
	  if w.Code != 200 {
		   t.Fatalf("receipt bob %d %s", w.Code, w.Body.String())
	   }
	  if err := json.Unmarshal(w.Body.Bytes(), &ses); err != nil {
		   t.Fatal(err)
	  }
	   if ses.State != cerbind.StQuorum {
		   t.Fatalf("state %s", ses.State)
	   }
	  w = cerReq(t, r, "custody", "bob", http.MethodPost, "/v1/cerbind/sessions/bind", map[string]any{
		  "id": ses.ID, "actor": "bob", "batch_id": "batch-http", "epoch": ses.FenceEpoch, "token": ses.FenceToken,
	   })
	   if w.Code != 200 {
		  t.Fatalf("bind %d %s", w.Code, w.Body.String())
	  }
	   w = cerReq(t, r, "custody", "alice", http.MethodGet, "/v1/cerbind/sessions/"+ses.ID, nil)
	  if w.Code != 200 {
		   t.Fatalf("view %d %s", w.Code, w.Body.String())
	  }
	   var view cerbind.View
	  if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		   t.Fatal(err)
	   }
	   w = cerPostFence(t, r, "custody", "alice", "/v1/cerbind/sessions/sign-ready", view.Session)
	   if w.Code != 200 {
		  t.Fatalf("ready %d %s", w.Code, w.Body.String())
	   }
	   w = cerReq(t, r, "custody", "cara", http.MethodPost, "/v1/cerbind/gate/check", map[string]any{
		  "batch_id": "batch-http", "chain": "ETH", "amount": "20", "dest": "0xabc",
	   })
	   if w.Code != 200 {
		  t.Fatalf("gate %d %s", w.Code, w.Body.String())
	   }
	   w = cerReq(t, r, "ops", "dave", http.MethodPost, "/v1/cerbind/sessions/abort", map[string]any{
		  "id": ses.ID, "actor": "dave", "reason": "desk",
	   })
	  if w.Code != 200 {
		   t.Fatalf("abort %d %s", w.Code, w.Body.String())
	   }
	   w = cerReq(t, r, "custody", "cara", http.MethodPost, "/v1/cerbind/gate/check", map[string]any{
		   "batch_id": "batch-http", "chain": "ETH", "amount": "20", "dest": "0xabc",
	   })
	   if w.Code == 200 {
		   t.Fatalf("gate allowed after abort %s", w.Body.String())
	  }
  }

   func cerPostFence(t *testing.T, r http.Handler, role, actor, path string, ses cerbind.Session) *httptest.ResponseRecorder {
	  t.Helper()
	  return cerReq(t, r, role, actor, http.MethodPost, path, map[string]any{
		   "id": ses.ID, "actor": actor, "epoch": ses.FenceEpoch, "token": ses.FenceToken,
	   })
   }

   func cerPostSlot(t *testing.T, r http.Handler, actor, path string, ses cerbind.Session, slot string) *httptest.ResponseRecorder {
	  t.Helper()
	  return cerReq(t, r, "custody", actor, http.MethodPost, path, map[string]any{
		  "id": ses.ID, "actor": actor, "slot": slot, "epoch": ses.FenceEpoch, "token": ses.FenceToken,
	   })
   }

  func cerReq(t *testing.T, h http.Handler, role, actor, method, path string, body any) *httptest.ResponseRecorder {
	  t.Helper()
	   var r *http.Request
	   if body != nil {
		   b, err := json.Marshal(body)
		   if err != nil {
			   t.Fatal(err)
		   }
		  r = httptest.NewRequest(method, path, bytes.NewReader(b))
		   r.Header.Set("Content-Type", "application/json")
	  } else {
		  r = httptest.NewRequest(method, path, nil)
	   }
	  r.Header.Set("X-Actor", actor)
	   r.Header.Set("X-Role", role)
	  w := httptest.NewRecorder()
	   h.ServeHTTP(w, r)
	  return w
  }
