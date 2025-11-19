package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/auditchain"
	"account-assets.kucoin.net/funding/internal/balproof"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/por"
	"account-assets.kucoin.net/funding/internal/solvency"
	"github.com/shopspring/decimal"
)

  func solvPipe(st ledger.Store, b bus.Bus, secret string) *solvency.Pipeline {
	   return solvency.New(solvency.Options{
		   Books: st, Secret: secret, AllowOverride: true, Bus: b,
		  Audit: solvency.ChainAudit{Chain: auditchain.New()},
	   })
  }

   func fixedClock(t *testing.T, p *solvency.Pipeline, at time.Time) *time.Time {
	  t.Helper()
	  cur := at.UTC().Truncate(time.Millisecond)
	   p.SetClock(func() time.Time { return cur })
	   return &cur
  }

  func mustSched(t *testing.T, p *solvency.Pipeline, name string, window time.Duration) solvency.Schedule {
	   t.Helper()
	  sch, err := p.PutSchedule(context.Background(), solvency.Schedule{
		  Name: name, Window: window, Interval: time.Hour, Enabled: true,
	  })
	   if err != nil {
		   t.Fatal(err)
	  }
	  return sch
  }

   func TestSolvencyScheduleSealsAndProves(t *testing.T) {
	  st := ledger.NewMemStore()
	   st.Seed("user-a", ledger.AccountSpot, "USDT", "1000")
	   st.Seed("user-b", ledger.AccountWallet, "USDT", "250")
	  st.Seed("system", ledger.AccountBridge, "USDT", "5000")
	  st.Seed("system", ledger.AccountWarm, "USDT", "0")
	   st.Seed("system", ledger.AccountCold, "USDT", "0")
	   p := solvPipe(st, bus.NewMemory(), "sched-secret")
	   start := time.Date(2022,
		   9, 21, 10, 0, 0, 0, time.UTC)
	  fixedClock(t, p, start)
	   sch := mustSched(t, p, "hourly-usdt", 2*time.Minute)
	   att, err := p.RunSchedule(context.Background(), sch.ID)
	  if err != nil {
		   t.Fatal(err)
	  }
	  if att.Status != solvency.StatusSealed {
		   t.Fatalf("status %s", att.Status)
	  }
	  if att.SealedAt == nil || att.SealedAt.After(att.WindowEnd) {
		   t.Fatalf("seal outside window sealed=%v end=%s", att.SealedAt, att.WindowEnd)
	  }
	  if !att.Solvency || att.Deprecated || att.Class != solvency.ClassPublishable {
		  t.Fatalf("class %+v", att)
	   }
	   if att.Coverage != solvency.CoverageCovered || att.SealDigest == "" || att.Root == "" {
		  t.Fatalf("incomplete seal %+v", att)
	  }
	   pr, got, err := p.Prove(context.Background(), att.ID, "user-a", "funding", "USDT")
	   if err != nil {
		   t.Fatal(err)
	   }
	   if pr.Leaf.Available != "1000" || pr.Leaf.Total != "1000" || pr.Leaf.Book != "spot" {
		  t.Fatalf("leaf %+v", pr.Leaf)
	  }
	   if err := solvency.VerifyProof(got.Root, pr); err != nil {
		  t.Fatal(err)
	   }
	  if len(pr.Path) == 0 {
		   t.Fatal("expected a merkle path")
	  }
	   pr.Path[0].Sibling = "00"
	  if err := solvency.VerifyProof(got.Root, pr); err == nil {
		   t.Fatal("tampered path accepted")
	  }
	  pub, err := p.Publish(context.Background(), att.ID, false)
	  if err != nil {
		   t.Fatal(err)
	   }
	   if pub.Status != solvency.StatusPublished || pub.AuditRef == "" {
		  t.Fatalf("publish %+v", pub)
	   }
	  if err := st.UpsertAccount(context.Background(), &ledger.Account{
		  UserID: "user-a", AccountType: ledger.AccountSpot, Asset: "USDT",
		   Available: decimal.RequireFromString("1"), Frozen: decimal.Zero,
	   }); err != nil {
		   t.Fatal(err)
	   }
	   again, _, err := p.Prove(context.Background(), att.ID, "user-a", "spot", "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if again.Leaf.Total != "1000" {
		   t.Fatalf("published proof followed live book: %+v", again.Leaf)
	  }
	  if err := solvency.VerifyProof(pub.Root, again); err != nil {
		   t.Fatal(err)
	  }
	   if _, err := p.Collect(context.Background(), att.ID); !errors.Is(err, solvency.ErrImmutable) {
		  t.Fatalf("collect after publish: %v", err)
	   }
	  if _, err := p.Publish(context.Background(), att.ID, false); !errors.Is(err, solvency.ErrImmutable) {
		   t.Fatalf("second publish: %v", err)
	  }
  }

   func TestSolvencyReducedReserveOpensDrift(t *testing.T) {
	  ctx := context.Background()
	   st := ledger.NewMemStore()
	   st.Seed("user-a", ledger.AccountSpot, "USDT", "1000")
	  st.Seed("system", ledger.AccountBridge, "USDT", "1000")
	  st.Seed("system", ledger.AccountWarm, "USDT", "0")
	   st.Seed("system", ledger.AccountCold, "USDT", "0")
	  before, err := st.GetAccount(ctx, "system", ledger.AccountBridge, "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  hotBefore := before.Available.String()
	  b := bus.NewMemory()
	  gotBus := make(chan []byte, 1)
	   b.Subscribe(ctx, solvency.TopicDrift, "desk", func(_ context.Context, _, value []byte) error {
		  gotBus <- append([]byte(nil), value...)
		   return nil
	   })
	  p := solvPipe(st, b, "drift-secret")
	  fixedClock(t, p, time.Date(2022, 9, 21, 11, 0, 0, 0, time.UTC))
	  if err := p.SetReserveOverride([]solvency.ReserveLeg{{
		   Asset: "USDT", Hot: "100", Warm: "0", Cold: "0",
	  }}); err != nil {
		   t.Fatal(err)
	   }
	  sch := mustSched(t, p, "watch-reserve", 5*time.Minute)
	   att, err := p.RunSchedule(ctx, sch.ID)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if att.Status != solvency.StatusSealed || att.Class != solvency.ClassInternal || att.Coverage != solvency.CoverageShort {
		   t.Fatalf("att %+v", att)
	   }
	   after, err := st.GetAccount(ctx, "system", ledger.AccountBridge, "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if after.Available.String() != hotBefore {
		  t.Fatalf("custody book changed %s -> %s", hotBefore, after.Available.String())
	  }
	   drifts,
		  err := p.ListDrifts(ctx, solvency.DriftOpen, 20)
	   if err != nil {
		   t.Fatal(err)
	   }
	   var d solvency.DriftCase
	   for _, row := range drifts {
		  if row.Asset == "USDT" && row.AttestationID == att.ID {
			  d = row
			  break
		  }
	   }
	  if d.ID == "" {
		   t.Fatal("drift not opened")
	  }
	   if d.Severity != solvency.SevDeficit || d.Liability != "1000" || d.Observed != "100" || d.Deficit != "900" || d.RatioBps != 9000 {
		  t.Fatalf("drift fields %+v", d)
	  }
	  if d.Hot != "100" || d.Warm != "0" || d.Cold != "0" {
		   t.Fatalf("legs %+v", d)
	   }
	  for _, part := range []string{"asset=USDT", "liability=1000", "observed=100", "deficit=900", "ratio_bps=9000", "severity=deficit", "hot=100"} {
		  if !bytes.Contains([]byte(d.Explain), []byte(part)) {
			  t.Fatalf("explain missing %s: %s", part, d.Explain)
		   }
	   }
	   select {
	  case body := <-gotBus:
		  if !bytes.Contains(body, []byte(`"deficit":"900"`)) || !bytes.Contains(body, []byte(`"severity":"deficit"`)) {
			   t.Fatalf("bus %s", body)
		  }
	  case <-time.After(2 * time.Second):
		   t.Fatal("drift signal missing")
	   }
	  if _, err := p.Publish(ctx, att.ID, false); !errors.Is(err, solvency.ErrDeficitOpen) {
		  t.Fatalf("publish without ack: %v", err)
	   }
	   if _, err := p.CloseDrift(ctx, d.ID, "ops-a", "too early"); !errors.Is(err, solvency.ErrDrift) {
		  t.Fatalf("close from open: %v", err)
	  }
	  acked, err := p.AckDrift(ctx, d.ID, "ops-a", "seen")
	   if err != nil || acked.Status != solvency.DriftAcked {
		   t.Fatalf("ack %v %+v", err, acked)
	  }
	   closed, err := p.CloseDrift(ctx, d.ID, "ops-a", "books restored offline")
	  if err != nil || closed.Status != solvency.DriftClosed {
		   t.Fatalf("close %v %+v", err, closed)
	   }
	  pub, err := p.Publish(ctx, att.ID, true)
	   if err != nil {
		   t.Fatal(err)
	  }
	   if pub.Status != solvency.StatusPublished || pub.Class != solvency.ClassInternal || !pub.Solvency {
		   t.Fatalf("internal publish %+v", pub)
	   }
  }

   func TestSolvencyKillWhileBuildingDoesNotPublish(t *testing.T) {
	   ctx := context.Background()
	  st := ledger.NewMemStore()
	   st.Seed("user-a", ledger.AccountSpot, "USDT", "40")
	   st.Seed("system", ledger.AccountBridge, "USDT", "40")
	   p := solvPipe(st, nil, "kill-secret")
	  start := time.Date(2022, 9, 21, 12, 0, 0, 0, time.UTC)
	   cur := fixedClock(t, p, start)
	  sch, err := p.PutSchedule(ctx, solvency.Schedule{
		  Name: "fragile", Window: 2 * time.Minute, Interval: time.Hour, Enabled: true, AutoPublish: true,
	   })
	  if err != nil {
		   t.Fatal(err)
	  }
	   att, err := p.Begin(ctx, sch)
	   if err != nil {
		  t.Fatal(err)
	   }
	  att, err = p.Collect(ctx, att.ID)
	   if err != nil {
		  t.Fatal(err)
	  }
	   if att.Status != solvency.StatusBuilding || att.Root == "" || att.SealDigest != "" {
		   t.Fatalf("partial %+v", att)
	   }
	  if _, err := p.Publish(ctx, att.ID, true); !errors.Is(err, solvency.ErrPartial) {
		  t.Fatalf("publish partial: %v", err)
	  }
	   if _, err := p.Store().CasAttestation(ctx, att.ID, solvency.StatusBuilding, solvency.AttestPatch{
		  Status: solvency.StatusPublished, UpdatedAt: *cur, PublishedAt: cur,
	  }); !errors.Is(err, solvency.ErrTransition) {
		   t.Fatalf("store publish from building: %v", err)
	   }
	   n, err := p.PublishDue(ctx)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if n != 0 {
		  t.Fatalf("auto published %d", n)
	   }
	  resumed := solvency.New(solvency.Options{
		  Books: st, Store: p.Store(), Secret: "kill-secret", AllowOverride: true,
	   })
	   *cur = start.Add(3 * time.Minute)
	  resumed.SetClock(func() time.Time { return *cur })
	  if _, err := resumed.Publish(ctx, att.ID, true); !errors.Is(err, solvency.ErrPartial) {
		   t.Fatalf("resumed publish: %v", err)
	  }
	   rep, err := resumed.Patrol(ctx)
	   if err != nil {
		   t.Fatal(err)
	  }
	   if len(rep.Abandoned) != 1 || rep.Abandoned[0] != att.ID {
		   t.Fatalf("patrol %+v", rep)
	   }
	   final, err := resumed.Get(ctx, att.ID)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if final.Status != solvency.StatusAbandoned {
		   t.Fatalf("status %s", final.Status)
	  }
	  if _, err := resumed.Publish(ctx, att.ID, true); !errors.Is(err, solvency.ErrNotSealed) {
		   t.Fatalf("publish abandoned: %v", err)
	  }
	  if _, err := resumed.Store().CasAttestation(ctx, att.ID, solvency.StatusAbandoned, solvency.AttestPatch{
		  Status: solvency.StatusPublished, UpdatedAt: *cur,
	   }); !errors.Is(err, solvency.ErrTransition) {
		  t.Fatalf("cas abandoned: %v", err)
	  }
   }

   func TestSolvencyLeafMismatchAndFenceMoveRefuseSeal(t *testing.T) {
	   ctx := context.Background()
	  st := ledger.NewMemStore()
	  st.Seed("user-a", ledger.AccountSpot, "USDT", "80")
	   st.Seed("system", ledger.AccountBridge, "USDT", "80")
	   p := solvPipe(st, nil, "leaf-secret")
	   fixedClock(t, p, time.Date(2022, 9, 21, 13, 0, 0, 0, time.UTC))
	  sch := mustSched(t, p, "leaf-check", 10*time.Minute)
	  att, err := p.Begin(ctx, sch)
	   if err != nil {
		  t.Fatal(err)
	  }
	   if _, err = p.Collect(ctx, att.ID); err != nil {
		   t.Fatal(err)
	   }
	  if _, err = p.Compare(ctx, att.ID); err != nil {
		  t.Fatal(err)
	  }
	  if err := st.UpsertAccount(ctx, &ledger.Account{
		   UserID: "user-a", AccountType: ledger.AccountSpot, Asset: "USDT",
		  Available: decimal.RequireFromString("81"), Frozen: decimal.Zero,
	  }); err != nil {
		  t.Fatal(err)
	  }
	  if _, err := p.Seal(ctx, att.ID); !errors.Is(err, solvency.ErrLeafMismatch) {
		  t.Fatalf("seal mismatch: %v", err)
	  }
	   cur, err := p.Get(ctx, att.ID)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if cur.Status != solvency.StatusBuilding || cur.SealDigest != "" {
		  t.Fatalf("status after mismatch %+v", cur)
	  }
	   st2 := ledger.NewMemStore()
	   st2.Seed("user-a", ledger.AccountSpot, "USDT", "80")
	   st2.Seed("system", ledger.AccountBridge, "USDT", "80")
	  p2 := solvPipe(st2, nil, "fence-secret")
	   fixedClock(t, p2, time.Date(2022, 9, 21, 13, 30, 0, 0, time.UTC))
	  sch2 := mustSched(t, p2, "fence-check", 10*time.Minute)
	   att2, err := p2.Begin(ctx, sch2)
	   if err != nil {
		  t.Fatal(err)
	  }
	  if _, err = p2.Collect(ctx, att2.ID); err != nil {
		  t.Fatal(err)
	  }
	  if _, err = p2.Compare(ctx, att2.ID); err != nil {
		   t.Fatal(err)
	  }
	  if err := st2.InsertTransfer(ctx, &ledger.Transfer{
		  ID: "tr-fence", IdempotencyKey: "idem-fence", UserID: "user-a",
		  State: ledger.StateInitiated, Asset: "USDT", FenceEpoch: 4,
	   }); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := p2.Seal(ctx, att2.ID); !errors.Is(err, solvency.ErrFenceMoved) {
		  t.Fatalf("seal after fence: %v", err)
	  }
	   cur, err = p2.Get(ctx, att2.ID)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if cur.Status != solvency.StatusBuilding {
		  t.Fatalf("fence status %s", cur.Status)
	   }
   }

   func TestSolvencyLegacyDisclaimerIsNotAttestation(t *testing.T) {
	  ctx := context.Background()
	  st := ledger.NewMemStore()
	   st.Seed("user-a", ledger.AccountSpot, "USDT", "15")
	   snap, err := por.BuildFromStore(ctx, st, []string{"user-a"}, time.Date(2022, 9, 21, 8, 0, 0, 0, time.UTC))
	  if err != nil {
		   t.Fatal(err)
	  }
	   if snap.IsSolvency() || snap.Solvency || !snap.Deprecated || snap.Class != por.EngineeringClass {
		  t.Fatalf("por snap %+v", snap)
	   }
	   if snap.Disclaimer != por.Disclaimer {
		   t.Fatalf("disclaimer %s", snap.Disclaimer)
	   }
	  if solvency.AcceptsAsSolvency(snap.Disclaimer, snap.Class, snap.Solvency) {
		   t.Fatal("por snapshot accepted as solvency")
	   }
	  if solvency.AcceptsAsSolvency(por.PriorDisclaimer, por.EngineeringClass, false) {
		   t.Fatal("prior por disclaimer accepted")
	   }
	  legacy := solvency.ClassifyDisclaimer(por.PriorDisclaimer)
	  if legacy.Solvency || !legacy.Deprecated || legacy.Class != solvency.ClassEngineering {
		   t.Fatalf("classify %+v", legacy)
	   }
	   svc := balproof.New("legacy-secret")
	  proof, err := svc.Issue(ctx, balproof.IssueReq{UserID: "user-a", Book: "spot", Asset: "USDT", Nonce: "nonce-12345678"})
	  if err != nil {
		   t.Fatal(err)
	  }
	   if proof.Solvency || !proof.Deprecated || proof.Class != balproof.EngineeringClass || proof.Disclaimer != balproof.Disclaimer {
		   t.Fatalf("balproof %+v", proof)
	  }
	  if solvency.AcceptsAsSolvency(proof.Disclaimer, proof.Class, proof.Solvency) {
		   t.Fatal("balance proof accepted as solvency")
	   }
	  if solvency.AcceptsAsSolvency(balproof.PriorDisclaimer, balproof.EngineeringClass, false) {
		   t.Fatal("prior balance disclaimer accepted")
	   }
	  p := solvPipe(st, nil, "legacy-solvency")
	  fixedClock(t, p, time.Date(2022, 9, 21, 8, 0, 0, 0, time.UTC))
	  st.Seed("system", ledger.AccountBridge, "USDT", "15")
	  sch := mustSched(t, p, "beside-legacy", time.Minute)
	  att, err := p.RunSchedule(ctx, sch.ID)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if att.Root == snap.Root {
		  t.Fatal("solvency root collided with engineering merkle")
	   }
	  if !solvency.AcceptsAsSolvency(att.Disclaimer, att.Class, att.Solvency) {
		   t.Fatalf("real attestation rejected %+v", att)
	   }
   }

  func TestSolvencyHTTPDesk(t *testing.T) {
	  rt, h := router(t)
	  uid := seedUser(rt)
	  schRec := solvCall(t, rt, h, http.MethodPost, "/v1/solvency/ops/schedules", map[string]any{
		   "name": "desk", "window_sec": 120, "interval_sec": 3600, "enabled": true,
	   }, "ops")
	   if schRec.Code != 200 {
		   t.Fatalf("schedule %d %s", schRec.Code, schRec.Body.String())
	  }
	  var sch solvency.Schedule
	   if err := json.Unmarshal(schRec.Body.Bytes(), &sch); err != nil {
		   t.Fatal(err)
	  }
	   runRec := solvCall(t, rt, h, http.MethodPost, "/v1/solvency/ops/run", map[string]any{"schedule_id": sch.ID}, "ops")
	   if runRec.Code != 200 {
		   t.Fatalf("run %d %s", runRec.Code, runRec.Body.String())
	  }
	  var att solvency.Attestation
	   if err := json.Unmarshal(runRec.Body.Bytes(), &att); err != nil {
		  t.Fatal(err)
	  }
	   if att.Status != solvency.StatusSealed || !att.Solvency || att.Deprecated || att.SealedAt == nil || att.SealedAt.After(att.WindowEnd) {
		  t.Fatalf("http seal %+v", att)
	   }
	  proofRec := solvCall(t, rt, h, http.MethodPost, "/v1/solvency/attestations/"+att.ID+"/proof", map[string]any{
		   "user_id": uid, "book": "funding", "asset": "USDT",
	  }, "user")
	   if proofRec.Code != 200 {
		  t.Fatalf("proof %d %s", proofRec.Code, proofRec.Body.String())
	  }
	   var proofBody struct {
		   Verified bool `json:"verified"`
		  Solvency bool `json:"solvency"`
		   Proof solvency.Proof `json:"proof"`
		  Root string `json:"root"`
	  }
	  if err := json.Unmarshal(proofRec.Body.Bytes(), &proofBody); err != nil {
		  t.Fatal(err)
	  }
	  if !proofBody.Verified || !proofBody.Solvency || proofBody.Proof.Leaf.UserID != uid {
		   t.Fatalf("proof body %+v", proofBody)
	  }
	   if err := solvency.VerifyProof(proofBody.Root, proofBody.Proof); err != nil {
		   t.Fatal(err)
	  }
	  q := url.Values{}
	  q.Set("disclaimer", por.Disclaimer)
	   leg := solvCall(t, rt, h, http.MethodGet, "/v1/solvency/legacy?"+q.Encode(), nil, "user")
	   if leg.Code != 200 {
		   t.Fatalf("legacy %d %s", leg.Code, leg.Body.String())
	  }
	   var cls solvency.LegacyClass
	  if err := json.Unmarshal(leg.Body.Bytes(), &cls); err != nil {
		  t.Fatal(err)
	  }
	  if cls.Solvency || !cls.Deprecated {
		   t.Fatalf("legacy class %+v", cls)
	  }
	   old := solvCall(t, rt, h, http.MethodPost, "/v1/por/snapshot", map[string]any{"users": []string{uid}}, "ops")
	  if old.Code != 200 {
		   t.Fatalf("por %d %s", old.Code, old.Body.String())
	  }
	  if old.Header().Get("Deprecation") != "true" {
		  t.Fatalf("deprecation header %q", old.Header().Get("Deprecation"))
	  }
	  var oldBody struct {
		  Solvency   bool `json:"solvency"`
		   Deprecated bool `json:"deprecated"`
		  Class      string `json:"class"`
		  Disclaimer string `json:"disclaimer"`
	   }
	   if err := json.Unmarshal(old.Body.Bytes(), &oldBody); err != nil {
		   t.Fatal(err)
	   }
	   if oldBody.Solvency || !oldBody.Deprecated || oldBody.Class != por.EngineeringClass || oldBody.Disclaimer != por.Disclaimer {
		  t.Fatalf("old api %+v", oldBody)
	   }
	   issue := solvCall(t, rt, h, http.MethodPost, "/v1/balproof/issue", map[string]any{
		  "user_id": uid, "book": "funding", "asset": "USDT", "nonce": "http-nonce-01",
	  }, "user")
	  if issue.Code != 200 {
		  t.Fatalf("issue %d %s", issue.Code, issue.Body.String())
	   }
	   var issued balproof.Proof
	   if err := json.Unmarshal(issue.Body.Bytes(), &issued); err != nil {
		   t.Fatal(err)
	   }
	  if issued.Solvency || !issued.Deprecated {
		  t.Fatalf("issued %+v", issued)
	  }
   }

   func solvCall(t *testing.T, rt *app.Runtime, h http.Handler, method, path string, body any, role string) *httptest.ResponseRecorder {
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
	   if len(rt.Cfg.Auth.BearerTokens) > 0 {
		   r.Header.Set("Authorization", "Bearer "+rt.Cfg.Auth.BearerTokens[0])
	  }
	  r.Header.Set("X-Role", role)
	  r.Header.Set("X-Actor", "ops-desk")
	  w := httptest.NewRecorder()
	  h.ServeHTTP(w, r)
	  return w
   }

   func TestSolvencyOverrideRefusedOutsideMem(t *testing.T) {
	   p := solvency.New(solvency.Options{Secret: "x", AllowOverride: false, Books: ledger.NewMemStore()})
	   err := p.SetReserveOverride([]solvency.ReserveLeg{{Asset: "USDT", Hot: "1"}})
	  if !errors.Is(err, solvency.ErrOverride) {
		   t.Fatalf("override: %v", err)
	   }
  }

type unreadBooks struct{ inner *ledger.MemStore }

  func (u unreadBooks) ListAccounts(ctx context.Context, limit int) ([]ledger.Account, error) {
	  return u.inner.ListAccounts(ctx,
		  limit)
   }

  func (u unreadBooks) ListByState(ctx context.Context, st ledger.State, limit int) ([]*ledger.Transfer, error) {
	   return u.inner.ListByState(ctx, st, limit)
  }

   func (u unreadBooks) ListUserBooks(context.Context, string) ([]ledger.Account, error) {
	   return nil, errors.New("custody books unreadable")
   }

  func TestSolvencyUnreadReserveRefusesSeal(t *testing.T) {
	  ctx := context.Background()
	   st := ledger.NewMemStore()
	   st.Seed("user-a", ledger.AccountSpot, "USDT", "20")
	   p := solvency.New(solvency.Options{
		   Books: unreadBooks{inner: st}, Secret: "unread-secret", AllowOverride: false,
	  })
	  fixedClock(t,
		   p, time.Date(2022, 9, 21, 14, 0, 0, 0, time.UTC))
	  sch := mustSched(t, p, "unread", time.Minute)
	  att, err := p.Begin(ctx, sch)
	  if err != nil {
		   t.Fatal(err)
	   }
	   if _, err = p.Collect(ctx, att.ID); err != nil {
		  t.Fatal(err)
	  }
	  att, err = p.Compare(ctx, att.ID)
	  if err != nil {
		   t.Fatal(err)
	  }
	   if att.Coverage != solvency.CoverageUnread {
		   t.Fatalf("coverage %s", att.Coverage)
	  }
	   if _, err := p.Seal(ctx, att.ID); !errors.Is(err, solvency.ErrReserveUnread) {
		  t.Fatalf("seal unread: %v", err)
	  }
	  cur, err := p.Get(ctx, att.ID)
	   if err != nil {
		   t.Fatal(err)
	  }
	  if cur.Status != solvency.StatusBuilding || cur.SealDigest != "" {
		  t.Fatalf("sealed despite unread %+v", cur)
	   }
	   rows, err := p.ListDrifts(ctx, solvency.DriftOpen, 10)
	  if err != nil || len(rows) == 0 || rows[0].Severity != solvency.SevDeficit {
		  t.Fatalf("unread drift %v %+v", err, rows)
	  }
	  if !bytes.Contains([]byte(rows[0].Explain), []byte("不可读")) {
		  t.Fatalf("explain %s", rows[0].Explain)
	  }
  }
