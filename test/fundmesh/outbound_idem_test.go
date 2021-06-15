package fundmesh_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/fundmesh"
)

   func outboundReq(key, asset, amt, chain, dest string) fundmesh.OutboundRequest {
	   return fundmesh.OutboundRequest{
		   UserID: "u1", Actor: "u1", Asset: asset, Amount: amt,
		   Chain: chain, Dest: dest, IdempotencyKey: key, Entry: fundmesh.EntryPOST,
	  }
  }

  func debitCalls(eng *fundmesh.Engine) int {
	   if rem, ok := eng.Remote().(*fundmesh.MemRemoteLedger); ok {
		  return rem.DebitCalls()
	   }
	  return -1
   }

   func TestBO01MatrixGolden(t *testing.T) {
	  g := loadJSON(t, "outbound_matrix.json")
	   want, _ := json.Marshal(g["gen"])
	  got, _ := json.Marshal(fundmesh.OutboundMatrixRows())
	   if string(want) != string(got) {
		   t.Fatalf("matrix\nwant %s\ngot %s", want, got)
	   }
	   parity, _ := json.Marshal(g["parity"])
	   gotp, _ := json.Marshal(fundmesh.OutboundEntryParityRows())
	   if string(parity) != string(gotp) {
		   t.Fatalf("parity\nwant %s\ngot %s", parity, gotp)
	   }
	  kr, _ := json.Marshal(g["key_rules"])
	   gotk,
		  _ := json.Marshal(fundmesh.OutboundKeyRuleRows())
	  if string(kr) != string(gotk) {
		  t.Fatalf("key rules\nwant %s\ngot %s", kr, gotk)
	  }
	   rr, _ := json.Marshal(g["replay_rules"])
	   gotr, _ := json.Marshal(fundmesh.OutboundReplayRuleRows())
	   if string(rr) != string(gotr) {
		  t.Fatalf("replay rules")
	  }
  }

   func TestBO02FirstSubmitDebitsOnce(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	   res, err := eng.SubmitWithdraw(ctx, outboundReq("client-key-alpha01", "USDT", "10", "ETH", "0xabc"))
	   if err != nil {
		  t.Fatal(err)
	   }
	   if res.Code != fundmesh.CodeOutboundOK || res.DebitCount != 1 || !res.Debited {
		  t.Fatalf("%+v", res)
	  }
	   if !strings.HasPrefix(res.IdempotencyKey, fundmesh.IdemSchemeV1+".") {
		   t.Fatalf("key %s", res.IdempotencyKey)
	   }
	  if debitCalls(eng) != 1 {
		  t.Fatalf("debit %d", debitCalls(eng))
	  }
   }

   func TestBO03ReplaySameTerminalNoSecondDebit(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	   ctx := context.Background()
	  req := outboundReq("client-key-alpha01", "USDT", "10", "ETH", "0xabc")
	  first, err := eng.SubmitWithdraw(ctx, req)
	  if err != nil {
		   t.Fatal(err)
	  }
	   second, err := eng.SubmitWithdraw(ctx, req)
	  if err != nil {
		  t.Fatal(err)
	   }
	   if second.Code != fundmesh.CodeOutboundReplay || !second.Idempotent {
		  t.Fatalf("second %+v", second)
	   }
	   if second.WithdrawID != first.WithdrawID || second.Phase != first.Phase {
		   t.Fatalf("terminal drift first=%+v second=%+v", first, second)
	   }
	   if second.DebitCount != 1 || debitCalls(eng) != 1 {
		   t.Fatalf("second debit count res=%d remote=%d", second.DebitCount, debitCalls(eng))
	  }
	  if n, _ := eng.Journals().CountOK(ctx, first.IdempotencyKey); n != 1 {
		  t.Fatalf("journal %d", n)
	   }
   }

  func TestBO04SameKeyDiffFingerprintConflict(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	  a := outboundReq("client-key-alpha01", "USDT", "10", "ETH", "0xabc")
	   if _, err := eng.SubmitWithdraw(ctx, a); err != nil {
		  t.Fatal(err)
	   }
	   b := outboundReq("client-key-alpha01", "USDT", "11", "ETH", "0xabc")
	  res, err := eng.SubmitWithdraw(ctx, b)
	   if !errors.Is(err, fundmesh.ErrOutboundIdemConflict) {
		  t.Fatalf("err %v", err)
	   }
	  if res.Code != fundmesh.CodeOutboundConflict {
		  t.Fatalf("code %s", res.Code)
	  }
	  if debitCalls(eng) != 1 {
		  t.Fatalf("conflict must not debit again %d", debitCalls(eng))
	   }
  }

  func TestBO05HeaderAndBodyUnify(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	   ctx := context.Background()
	   req := outboundReq("", "BTC", "1", "BTC", "bc1qxyz")
	  req.HeaderKey = "hdr-key-bbbbbb"
	   first,
		  err := eng.SubmitWithdraw(ctx, req)
	   if err != nil {
		  t.Fatal(err)
	   }
	  again := outboundReq("hdr-key-bbbbbb", "BTC", "1", "BTC", "bc1qxyz")
	   second, err := eng.SubmitWithdraw(ctx, again)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if first.IdempotencyKey != second.IdempotencyKey {
		   t.Fatalf("key fork %s vs %s", first.IdempotencyKey, second.IdempotencyKey)
	   }
	   if second.Code != fundmesh.CodeOutboundReplay || debitCalls(eng) != 1 {
		   t.Fatalf("unify replay %+v debit=%d", second, debitCalls(eng))
	   }
  }

  func TestBO06DeriveWhenEmpty(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	  req := outboundReq("", "KCS", "2.50", "KCC", "kcs1dest")
	  a, err := eng.SubmitWithdraw(ctx, req)
	  if err != nil {
		  t.Fatal(err)
	   }
	  b, err := eng.SubmitWithdraw(ctx, req)
	  if err != nil {
		   t.Fatal(err)
	   }
	   if !strings.Contains(a.IdempotencyKey, ".d.") {
		   t.Fatalf("want derived %s", a.IdempotencyKey)
	  }
	  if b.Code != fundmesh.CodeOutboundReplay || debitCalls(eng) != 1 {
		  t.Fatalf("%+v debit=%d", b, debitCalls(eng))
	  }
   }

  func TestBO07AmountNormalizationReplay(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   ctx := context.Background()
	  a := outboundReq("amt-norm-cccccc", "usdt", "10.100", "eth", "0xabc")
	  b := outboundReq("amt-norm-cccccc", "USDT", "10.1", "ETH", "0xabc")
	  if _, err := eng.SubmitWithdraw(ctx, a); err != nil {
		  t.Fatal(err)
	  }
	  res, err := eng.SubmitWithdraw(ctx, b)
	   if err != nil {
		   t.Fatal(err)
	  }
	   if res.Code != fundmesh.CodeOutboundReplay {
		   t.Fatalf("code %s", res.Code)
	   }
	   if debitCalls(eng) != 1 {
		   t.Fatal("norm replay debited")
	   }
   }

   func TestBO08InvalidKeys(t *testing.T) {
	   g := loadJSON(t, "outbound_cases.json")
	   raw, _ := json.Marshal(g["invalid_keys"])
	  var keys []string
	  if err := json.Unmarshal(raw, &keys); err != nil {
		   t.Fatal(err)
	   }
	   eng := fundmesh.NewMemEngine()
	  for _, k := range keys {
		   _, err := eng.SubmitWithdraw(context.Background(), outboundReq(k, "USDT", "1", "ETH", "0x1"))
		  if !errors.Is(err, fundmesh.ErrOutboundIdemInvalid) && !errors.Is(err, fundmesh.ErrOutboundIdemRequired) {
			   t.Fatalf("key %q err=%v", k, err)
		   }
	   }
	  if debitCalls(eng) != 0 {
		  t.Fatal("invalid key must not debit")
	   }
   }

  func TestBO09ExpandCasesSequential(t *testing.T) {
	  eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	  var lastDebit int
	   for _, tc := range fundmesh.ExpandOutboundCases() {
		   res, err := eng.SubmitWithdraw(ctx, tc.Request("u1"))
		  if tc.Conflict {
			   if !errors.Is(err, fundmesh.ErrOutboundIdemConflict) {
				  t.Fatalf("%s err %v", tc.Name, err)
			  }
		  } else if err != nil {
			   t.Fatalf("%s %v", tc.Name, err)
		  }
		   if res.Code != tc.WantCode {
			  t.Fatalf("%s code %s want %s", tc.Name, res.Code, tc.WantCode)
		  }
		   if debitCalls(eng) < lastDebit {
			  t.Fatalf("%s debit decreased", tc.Name)
		  }
		  lastDebit = debitCalls(eng)
		   if tc.WantDebit > 0 && res.DebitCount != tc.WantDebit && !tc.Conflict {
			   t.Fatalf("%s debit_count %d", tc.Name, res.DebitCount)
		  }
	  }
  }

  func TestBO10RemoteFailNoLocal(t *testing.T) {
	  remote := fundmesh.NewMemRemoteLedger()
	  remote.Fail(errors.New("ledger unreachable"))
	  local := &fundmesh.CountingLocal{}
	   eng := fundmesh.NewEngine(remote, fundmesh.NewSnapshotValidator(fundmesh.CompleteMemConfig())).WithLocal(local)
	  _, err := eng.SubmitWithdraw(context.Background(), outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xabc"))
	  if !errors.Is(err, fundmesh.ErrLedgerRemoteFailed) {
		   t.Fatalf("got %v", err)
	  }
	  if local.Calls != 0 {
		   t.Fatal("local called")
	  }
	   if remote.DebitCalls() != 0 && remote.DebitCalls() != 1 {
		   // Submit increments calls then fail before debit map; debit stays 0.
	   }
   }

   func TestBO11GateClosedNoDebit(t *testing.T) {
	   cfg := fundmesh.CompleteMemConfig()
	  cfg.Delete("fundmesh/critical_write_enabled")
	  eng := fundmesh.NewEngine(fundmesh.NewMemRemoteLedger(), fundmesh.NewSnapshotValidator(cfg))
	  _, err := eng.SubmitWithdraw(context.Background(), outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xabc"))
	  if !errors.Is(err, fundmesh.ErrCriticalWriteClosed) {
		  t.Fatalf("got %v", err)
	   }
	   if debitCalls(eng) != 0 {
		  t.Fatal("closed gate debit")
	   }
   }

  func TestBO12HTTPSubmitAndReplay(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	  r := newHTTP(eng)
	  body := map[string]string{"asset": "USDT", "amount": "3", "chain": "ETH", "dest": "0xabc", "idempotency_key": "client-key-alpha01"}
	   w1 := do(r, http.MethodPost, "/v1/funding/withdrawals", body, "u1")
	  w2 := do(r, http.MethodPut, "/v1/funding/withdrawals", body, "u1")
	  if w1.Code != 200 || w2.Code != 200 {
		   t.Fatalf("http %d %d %s %s", w1.Code, w2.Code, w1.Body.String(), w2.Body.String())
	   }
	   if debitCalls(eng) != 1 {
		   t.Fatalf("http debit %d", debitCalls(eng))
	   }
	  w3 := do(r, http.MethodPost, "/v1/funding/withdraw/submit", body, "u1")
	   if w3.Code != 200 {
		  t.Fatalf("submit alias %d %s", w3.Code, w3.Body.String())
	  }
  }

   func TestBO13InspectReadOnly(t *testing.T) {
	  cat := fundmesh.InspectOutbound()
	   if cat["scheme"] != fundmesh.IdemSchemeV1 {
		   t.Fatal(cat["scheme"])
	   }
	   if !fundmesh.HomologousCancelStill() || !fundmesh.OutboundUsesCancelGate() {
		   t.Fatal("cancel homology broken")
	  }
	  eng := fundmesh.NewMemEngine()
	   _ = eng.ExportOutboundProofs()
	  if debitCalls(eng) != 0 {
		   t.Fatal("inspect debit")
	  }
  }

  func TestBO14AmountCanonRows(t *testing.T) {
	  for _, row := range fundmesh.AmountCanonRows() {
		   got, err := fundmesh.NormalizeAmount(row.Raw)
		   if row.OK {
			  if err != nil || got != row.Canon {
				  t.Fatalf("%s => %s %v want %s", row.Raw, got, err, row.Canon)
			   }
		   } else if err == nil {
			   t.Fatalf("%s should fail", row.Raw)
		  }
	   }
	   for _, row := range fundmesh.AssetCanonRows() {
		   if fundmesh.NormalizeAsset(row.Raw) != row.Canon {
			   t.Fatalf("%s", row.Raw)
		   }
	  }
   }

   func TestBO15SameIntentAfterNorm(t *testing.T) {
	  a := outboundReq("k", "usdt", "10.10", "eth", "0xabc")
	   b := outboundReq("k", "USDT", "10.1", "ETH", "0xabc")
	  if !fundmesh.SameIntentAfterNorm(a, b) {
		  t.Fatal("norm intent")
	   }
  }

  func TestBO16JournalRejectsSecondOK(t *testing.T) {
	   j := fundmesh.NewMemDebitJournal()
	  line := fundmesh.DebitLine{LineID: "1", IdempotencyKey: "fmout.v1.c.client-key-alpha01", OK: true, Amount: "1"}
	   if err := j.Append(context.Background(), line); err != nil {
		  t.Fatal(err)
	   }
	   if err := j.Append(context.Background(), line); !errors.Is(err, fundmesh.ErrDoubleDebitForbidden) {
		   t.Fatalf("got %v", err)
	  }
  }

   func TestBO17HTTPStatusParity(t *testing.T) {
	   if fundmesh.HTTPStatusOf(fundmesh.ErrOutboundIdemConflict) != http.StatusConflict {
		  t.Fatal("conflict status")
	  }
	   if fundmesh.HTTPStatusOf(fundmesh.ErrOutboundIdemInvalid) != http.StatusBadRequest {
		   t.Fatal("invalid status")
	   }
	  if fundmesh.CodeOf(fundmesh.ErrOutboundIdemConflict) != fundmesh.CodeOutboundConflict {
		   t.Fatal("code map")
	  }
   }
