package test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/attribrt"
	"account-assets.kucoin.net/funding/internal/attribrt/collide"
	"account-assets.kucoin.net/funding/internal/attribrt/desk"
	"github.com/gin-gonic/gin"
)

  func TestAttribReplayAll(t *testing.T) {
	   if _, err := attribrt.ReplayAll(); err != nil {
		  t.Fatal(err)
	   }
  }

   func TestAttribClearDeniesStranger(t *testing.T) {
	   c := collide.Case{State: collide.StClear, Origin: "user-a"}
	  if c.Allows("user-b") {
		   t.Fatal("stranger allowed on clear case")
	   }
	  if !c.Allows("user-a") {
		   t.Fatal("origin denied")
	   }
	   if (collide.Case{State: collide.StClear}).Allows("user-a") {
		   t.Fatal("empty origin allowed")
	  }
	  if (collide.Case{State: collide.StReleased, Winner: ""}).Allows("user-a") {
		   t.Fatal("empty winner allowed")
	  }
	   bad := collide.NormalizeLoaded(collide.Case{
		   ID: "c1", Chain: "xrp", Fold: "123", Origin: "user-a", State: "nope",
	  })
	   if bad.State != collide.StBlocked || bad.Allows("user-a") {
		   t.Fatalf("unknown state loaded open: %+v", bad)
	  }
  }

  func TestAttribPreviewDoesNotOccupyFold(t *testing.T) {
	  eng, err := attribrt.New()
	  if err != nil {
		   t.Fatal(err)
	   }
	   eng.SetBinder(memoBind{"XRP|memo|12345": "user-a"})
	  s := attribrt.Sight{
		   Chain: "XRP", TxHash: "aabbccdd11223350", Amount: "20", Asset: "XRP",
		   To: "rSharedAddrExample001", Memo: "12345", Preview: true,
	  }
	   if _, err := eng.Resolve(s); err != nil {
		  t.Fatal(err)
	   }
	  if n := len(eng.Collisions().List()); n != 0 {
		   t.Fatalf("preview wrote %d cases", n)
	   }
	   s.Preview = false
	  d, err := eng.Resolve(s)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if !d.CreditIntent {
		  t.Fatalf("first claimant refused: %v", d.Reasons)
	  }
	   eng.SetBinder(memoBind{"XRP|memo|12345": "user-b"})
	   s2 := s
	   s2.Preview = true
	  s2.TxHash = "aabbccdd11223351"
	  d2, err := eng.Resolve(s2)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if d2.CreditIntent {
		   t.Fatal("preview of second claimant credited")
	  }
	   found := false
	  for _, r := range d2.Reasons {
		   if r == "collide_blocked" {
			  found = true
		   }
	  }
	   if !found {
		   t.Fatalf("preview missed collision: %v", d2.Reasons)
	   }
	   if n := len(eng.Collisions().List()); n != 0 {
		  t.Fatalf("preview opened %d cases", n)
	   }
   }

   func TestAttribRestoreKeepsBlock(t *testing.T) {
	   book := collide.NewBook()
	   if !book.Restore(collide.Case{
		  ID: "c-block", Chain: "XRP", Namespace: "XRP", Fold: "123",
		  Origin: "user-a", State: collide.StBlocked, Claimants: []string{"user-a", "user-b"},
	  }) {
		   t.Fatal("restore")
	  }
	  if book.Allows("XRP", "XRP", "123", "user-a") || book.Allows("XRP", "XRP", "123", "user-b") {
		   t.Fatal("restored block allowed a claimant")
	  }
  }

  func TestAttribDeskResolveRequiresOps(t *testing.T) {
	  gin.SetMode(gin.TestMode)
	  eng, err := attribrt.New()
	  if err != nil {
		  t.Fatal(err)
	  }
	  r := gin.New()
	   desk.Register(r.Group("/v1"), eng)
	  body := `{"chain":"ETH","tx_hash":"aabbccdd11223345","amount":"1.5","asset":"ETH","preview":true}`
	   req := httptest.NewRequest(http.MethodPost, "/v1/attrib/resolve", strings.NewReader(body))
	   req.Header.Set("Content-Type", "application/json")
	  req.RemoteAddr = "10.18.0.9:9"
	   w := httptest.NewRecorder()
	  r.ServeHTTP(w, req)
	   if w.Code != http.StatusForbidden {
		  t.Fatalf("user role code=%d body=%s", w.Code, w.Body.String())
	  }
	  req = httptest.NewRequest(http.MethodPost, "/v1/attrib/resolve", strings.NewReader(body))
	  req.Header.Set("Content-Type", "application/json")
	  req.Header.Set("X-Role", "ops")
	   req.RemoteAddr = "10.18.0.9:9"
	  w = httptest.NewRecorder()
	  r.ServeHTTP(w, req)
	   if w.Code != http.StatusOK {
		  t.Fatalf("ops code=%d body=%s", w.Code, w.Body.String())
	  }
	  req = httptest.NewRequest(http.MethodGet, "/v1/attrib/fixtures", nil)
	   req.RemoteAddr = "10.18.0.9:9"
	   w = httptest.NewRecorder()
	   r.ServeHTTP(w, req)
	   if w.Code != http.StatusForbidden {
		   t.Fatalf("fixture user code=%d", w.Code)
	  }
	  req = httptest.NewRequest(http.MethodGet, "/v1/attrib/fixtures", nil)
	  req.Header.Set("X-Role", "reviewer")
	  req.RemoteAddr = "10.18.0.9:9"
	  w = httptest.NewRecorder()
	   r.ServeHTTP(w, req)
	   if w.Code != http.StatusOK {
		  t.Fatalf("fixture reviewer code=%d body=%s", w.Code, w.Body.String())
	  }
   }

type memoBind map[string]string

  func (m memoBind) Lookup(chain, kind, key string) (string, bool) {
	  u, ok := m[chain+"|"+kind+"|"+key]
	  return u, ok
  }
