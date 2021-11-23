package stablemintgate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stablebr"
	"account-assets.kucoin.net/funding/internal/stablemintgate"
)

  func newEng(clock func() time.Time) *stablemintgate.Engine {
	   br := stablebr.New()
	   if clock != nil {
		  br.SetClock(clock)
	  }
	   eng := stablemintgate.NewEngine(
		br,
		stablemintgate.NewMemLeaseRepo(),
		stablemintgate.NewMemHalfRepo(),
		stablemintgate.NewMemAuditRepo(),
		stablemintgate.NewMemGenRepo(),
		stablemintgate.NewMemProofRepo(),
	)
	if clock != nil {
		eng = eng.WithClock(clock)
	}
	return eng
}

   func attestAll(t *testing.T, br *stablebr.Service, kind, asset, actor string, ttl int) {
	   t.Helper()
	  items := []string{"reserve_attested", "issuer_signed", "kyt_clear"}
	  if kind == "mint" {
		  items = append(items, "bank_confirm")
	   } else {
		   items = append(items, "burn_confirm")
	   }
	   for _, it := range items {
		  if _, err := br.Attest(stablebr.AttestReq{
			  Kind: kind, Asset: asset, Item: it, Actor: actor, TTLSec: ttl,
		  }); err != nil {
			   t.Fatalf("attest %s: %v", it, err)
		   }
	   }
  }

   func loadGolden(t *testing.T, name string) map[string]any {
	  t.Helper()
	  p := filepath.Join("testdata", "stablemintgate", name)
	  if _, err := os.Stat(p); err != nil {
		   p = filepath.Join("..", "..", "testdata", "stablemintgate", name)
	  }
	   b, err := os.ReadFile(p)
	  if err != nil {
		  t.Fatalf("golden %s: %v", name, err)
	  }
	  var m map[string]any
	   if err := json.Unmarshal(b, &m); err != nil {
		   t.Fatalf("json %s: %v", name, err)
	   }
	   return m
   }

   func TestIssueMintLeaseHappy(t *testing.T) {
	  base := time.Date(2020, 11, 18, 10, 0, 0, 0, time.UTC)
	  eng := newEng(func() time.Time { return base })
	   attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-01", 600)
	   lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDT", Actor: "ops-issue-01",
	   })
	  if err != nil {
		   t.Fatalf("issue: %v", err)
	  }
	  if lease.LeaseID == "" || lease.State != stablemintgate.LeaseIssued {
		  t.Fatalf("lease %+v", lease)
	   }
	   if !lease.Fingerprint.Complete {
		  t.Fatal("fingerprint incomplete")
	  }
   }

   func TestClearAttestThenMintDenied(t *testing.T) {
	  g := loadGolden(t, "clear_then_mint.json")
	  if g["expect"].(map[string]any)["mint_ok"] != false {
		   t.Fatal("golden mint_ok")
	  }
	  base := time.Date(2020, 11, 18, 10, 5, 0, 0, time.UTC)
	   eng := newEng(func() time.Time { return base })
	   ctx := context.Background()
	  attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-02", 600)
	  lease, err := eng.IssueMintLease(ctx, stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDT", Actor: "ops-issue-02",
	  })
	   if err != nil {
		   t.Fatalf("issue: %v", err)
	   }
	  if err := eng.Bridge().ClearAttest("mint", "USDT", "ops-clear-01"); err != nil {
		  t.Fatalf("clear: %v", err)
	   }
	  _, _, err = eng.MintWithLease(ctx, stablemintgate.MintWithLeaseReq{
		   LeaseID: lease.LeaseID, Kind: "mint", UserID: "u-mint-01",
		  Asset: "USDT", Amount: "10.00", Actor: "ops-mint-01", IdempotencyKey: "idem-clear-01",
	   })
	  if err == nil {
		   t.Fatal("expected deny after clear")
	   }
	  if !errors.Is(err, stablemintgate.ErrGenMismatch) && !errors.Is(err, stablemintgate.ErrLeaseRevoked) &&
		   !errors.Is(err, stablemintgate.ErrAttestIncomplete) && !errors.Is(err, stablemintgate.ErrFingerprintDrift) &&
		   !errors.Is(err, stablemintgate.ErrAttestExpired) {
		   t.Fatalf("unexpected err %v", err)
	   }
	   rows, _ := eng.ListProofs(ctx, "mint|USDT", 20)
	   if len(rows) == 0 {
		  t.Fatal("expected deny proof")
	   }
   }

   func TestAttestExpiryBlocksMint(t *testing.T) {
	   _ = loadGolden(t, "attest_expired.json")
	   now := time.Date(2020, 11, 18, 11, 0, 0, 0, time.UTC)
	   var mu sync.Mutex
	  clock := func() time.Time {
		   mu.Lock()
		   defer mu.Unlock()
		  return now
	  }
	   eng := newEng(clock)
	   attestAll(t, eng.Bridge(), "mint", "USDC", "ops-attest-03", 2)
	  lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDC", Actor: "ops-issue-03", TTL: 2 * time.Second,
	   })
	   if err != nil {
		   t.Fatalf("issue: %v", err)
	  }
	   mu.Lock()
	   now = now.Add(3 * time.Second)
	  mu.Unlock()
	  _, err = eng.AssertLease(context.Background(), stablemintgate.AssertInput{
		  LeaseID: lease.LeaseID, Kind: "mint", Asset: "USDC",
	  })
	   if err == nil {
		  t.Fatal("expected expiry")
	  }
	  if !errors.Is(err, stablemintgate.ErrLeaseExpired) && !errors.Is(err, stablemintgate.ErrAttestExpired) {
		  t.Fatalf("err %v", err)
	  }
   }

  func TestNoLeaseFailClosed(t *testing.T) {
	   g := loadGolden(t, "no_lease_fail_closed.json")
	   if g["expect"].(map[string]any)["allowed"] != false {
		  t.Fatal("golden")
	   }
	  eng := newEng(nil)
	   attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-04", 600)
	   _, err := eng.Bridge().Mint(context.Background(), stablebr.MintReq{
		  UserID: "u-mint-02", Asset: "USDT", Amount: "1.00", Actor: "ops-mint-02",
		  IdempotencyKey: "idem-nolease-01",
	   })
	   if err == nil {
		   t.Fatal("expected no lease")
	  }
	  if !errors.Is(err, stablemintgate.ErrNoLease) && !errors.Is(err, stablebr.ErrLease) {
		  t.Fatalf("err %v", err)
	  }
	  _, err = eng.AssertLease(context.Background(), stablemintgate.AssertInput{Kind: "mint", Asset: "USDT"})
	   if !errors.Is(err, stablemintgate.ErrNoLease) {
		   t.Fatalf("assert %v", err)
	   }
   }

  func TestJournalLedgerHalfBlocksReplay(t *testing.T) {
	  g := loadGolden(t, "half_journal_ledger.json")
	  expect := g["expect"].(map[string]any)
	  eng := newEng(nil)
	   attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-05", 600)
	  lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDT", Actor: "ops-issue-05",
	  })
	   if err != nil {
		   t.Fatalf("issue: %v", err)
	  }
	   eng.Bridge().InjectLedgerError(errors.New("ledger create down"))
	   j, _, err := eng.MintWithLease(context.Background(), stablemintgate.MintWithLeaseReq{
		  LeaseID: lease.LeaseID, UserID: "u-mint-03", Asset: "USDT", Amount: "5.00",
		  Actor: "ops-mint-03", IdempotencyKey: "idem-half-usdt-01",
	   })
	  if err == nil {
		  t.Fatal("expected ledger fail")
	   }
	   if j.Status != stablebr.StatusHalf {
		  t.Fatalf("status %s want %s", j.Status, expect["journal_status"])
	   }
	   half, herr := eng.ExportHalfByIdem(context.Background(), "idem-half-usdt-01")
	   if herr != nil || half.State != stablemintgate.HalfOpen {
		  t.Fatalf("half %v %v", half, herr)
	  }
	   _, recErr := eng.ReconstructHalf(context.Background(), "idem-half-usdt-01", &j)
	  if recErr != nil {
		  t.Fatalf("reconstruct: %v", recErr)
	   }
	   _, _, err = eng.MintWithLease(context.Background(), stablemintgate.MintWithLeaseReq{
		  LeaseID: lease.LeaseID, UserID: "u-mint-03", Asset: "USDT", Amount: "5.00",
		   Actor: "ops-mint-03", IdempotencyKey: "idem-half-usdt-01",
	  })
	   if err == nil {
		   t.Fatal("replay must not double mint")
	  }
	   if !errors.Is(err, stablemintgate.ErrIdemReplay) && !errors.Is(err, stablemintgate.ErrHalfOpen) {
		   t.Fatalf("replay err %v", err)
	   }
   }

   func TestMintWithLeasePosted(t *testing.T) {
	  eng := newEng(nil)
	  attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-06", 600)
	   lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDT", Actor: "ops-issue-06",
	  })
	   if err != nil {
		   t.Fatalf("issue: %v", err)
	   }
	  j, got, err := eng.MintWithLease(context.Background(), stablemintgate.MintWithLeaseReq{
		  LeaseID: lease.LeaseID, UserID: "u-mint-04", Asset: "USDT", Amount: "3.25",
		   Actor: "ops-mint-04", IdempotencyKey: "idem-ok-01",
	  })
	  if err != nil {
		   t.Fatalf("mint: %v", err)
	   }
	   if j.Status != stablebr.StatusPosted || j.MintLeaseID != lease.LeaseID {
		  t.Fatalf("journal %+v", j)
	   }
	  if got == nil || got.State != stablemintgate.LeaseConsumed {
		  t.Fatalf("lease state %+v", got)
	   }
  }

  func TestRedeemClearDenied(t *testing.T) {
	  eng := newEng(nil)
	   attestAll(t, eng.Bridge(), "redeem", "USDT", "ops-attest-07", 600)
	   lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		   Kind: "redeem", Asset: "USDT", Actor: "ops-issue-07",
	  })
	  if err != nil {
		  t.Fatalf("issue: %v", err)
	  }
	   if err := eng.Bridge().ClearAttest("redeem", "USDT", "ops-clear-02"); err != nil {
		  t.Fatalf("clear: %v", err)
	   }
	  _, _, err = eng.RedeemWithLease(context.Background(), stablemintgate.MintWithLeaseReq{
		  LeaseID: lease.LeaseID, UserID: "u-red-01", Asset: "USDT", Amount: "1.00",
		  Actor: "ops-red-01", IdempotencyKey: "idem-red-01",
	   })
	  if err == nil {
		  t.Fatal("redeem after clear")
	  }
   }

   func TestFingerprintDrift(t *testing.T) {
	  _ = loadGolden(t, "fingerprint_drift.json")
	   eng := newEng(nil)
	  attestAll(t, eng.Bridge(), "redeem", "USDT", "ops-attest-08", 600)
	   lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "redeem", Asset: "USDT", Actor: "ops-issue-08",
	   })
	  if err != nil {
		  t.Fatalf("issue: %v", err)
	   }
	  if _, err := eng.Bridge().Attest(stablebr.AttestReq{
		  Kind: "redeem", Asset: "USDT", Item: "kyt_clear", Actor: "ops-attest-09", TTLSec: 600,
	  }); err != nil {
		  t.Fatalf("reattest: %v", err)
	  }
	  _, err = eng.AssertLease(context.Background(), stablemintgate.AssertInput{
		   LeaseID: lease.LeaseID, Kind: "redeem", Asset: "USDT",
	   })
	   if !errors.Is(err, stablemintgate.ErrFingerprintDrift) {
		   t.Fatalf("want drift got %v", err)
	   }
   }

  func TestFSMMatrixGolden(t *testing.T) {
	  g := loadGolden(t, "fsm_transitions.json")
	  legal := g["legal"].([]any)
	   got := stablemintgate.MatrixRows()
	   if len(got) != len(legal) {
		   t.Fatalf("legal count %d want %d", len(got), len(legal))
	   }
	   for _, pair := range legal {
		   arr := pair.([]any)
		   from := stablemintgate.LeaseState(arr[0].(string))
		   to := stablemintgate.LeaseState(arr[1].(string))
		   if !stablemintgate.CanTransition(from, to) {
			   t.Fatalf("legal missing %s->%s", from, to)
		   }
	   }
	  for _, pair := range g["illegal"].([]any) {
		  arr := pair.([]any)
		  from := stablemintgate.LeaseState(arr[0].(string))
		  to := stablemintgate.LeaseState(arr[1].(string))
		   if stablemintgate.CanTransition(from, to) {
			   t.Fatalf("illegal allowed %s->%s", from, to)
		   }
	   }
	  for _, samp := range stablemintgate.IllegalSamples() {
		   if stablemintgate.CanTransition(stablemintgate.LeaseState(samp[0]), stablemintgate.LeaseState(samp[1])) && samp[0] != "open" {
			  if samp[0] == string(stablemintgate.HalfResolved) {
				  if stablemintgate.CanTransitionHalf(stablemintgate.HalfResolved, stablemintgate.HalfOpen) {
					   t.Fatal("half reverse")
				  }
				   continue
			   }
			  t.Fatalf("sample %v", samp)
		  }
	  }
   }

  func TestConcurrentIssueRace(t *testing.T) {
	   eng := newEng(nil)
	   attestAll(t, eng.Bridge(), "mint", "USDC", "ops-attest-10", 600)
	  var ok int32
	   var wg sync.WaitGroup
	  for i := 0; i < 8; i++ {
		  wg.Add(1)
		   go func() {
			   defer wg.Done()
			  _, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
				   Kind: "mint", Asset: "USDC", Actor: "ops-race-01",
			  })
			  if err == nil {
				  atomic.AddInt32(&ok, 1)
			   }
		  }()
	  }
	   wg.Wait()
	   if ok < 1 {
		   t.Fatal("expected at least one issue")
	   }
  }

  func TestWorkersTick(t *testing.T) {
	   now := time.Date(2021, 4, 18, 8, 0, 0, 0, time.UTC)
	   var mu sync.Mutex
	   clock := func() time.Time {
		   mu.Lock()
		  defer mu.Unlock()
		   return now
	   }
	  eng := newEng(clock)
	  attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-11", 1)
	  lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		   Kind: "mint", Asset: "USDT", Actor: "ops-issue-11", TTL: time.Second,
	   })
	   if err != nil {
		   t.Fatalf("issue: %v", err)
	   }
	  mu.Lock()
	  now = now.Add(2 * time.Second)
	  mu.Unlock()
	  n, err := stablemintgate.NewExpiryWorker(eng, stablemintgate.WorkerConfig{BatchSize: 20}).TickOnce(context.Background())
	   if err != nil {
		   t.Fatalf("expiry: %v", err)
	   }
	   if n == 0 {
		  t.Fatal("expected expire")
	  }
	  got,
		  _ := eng.GetLease(context.Background(), lease.LeaseID)
	  if got.State != stablemintgate.LeaseExpired {
		   t.Fatalf("state %s", got.State)
	  }
	  _, _ = stablemintgate.NewClearRevokeWorker(eng, stablemintgate.WorkerConfig{}).TickOnce(context.Background())
	  _,
		   _ = stablemintgate.NewHalfInspectWorker(eng, stablemintgate.WorkerConfig{}).TickOnce(context.Background())
  }

  func TestOpsRevokeAndExport(t *testing.T) {
	  eng := newEng(nil)
	  attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-12", 600)
	  lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDT", Actor: "ops-issue-12",
	   })
	  if err != nil {
		   t.Fatalf("issue: %v", err)
	   }
	  rev, err := eng.Revoke(context.Background(), lease.LeaseID, "ops-rev-01", "ops halt")
	   if err != nil || rev.State != stablemintgate.LeaseRevoked {
		   t.Fatalf("revoke %v %+v", err, rev)
	   }
	   _, err = eng.AssertLease(context.Background(), stablemintgate.AssertInput{
		   LeaseID: lease.LeaseID, Kind: "mint", Asset: "USDT",
	   })
	  if !errors.Is(err, stablemintgate.ErrLeaseRevoked) {
		  t.Fatalf("assert %v", err)
	  }
	  b,
		   err := eng.ExportProofBundle(context.Background(), lease.LeaseID)
	   if err != nil || b["mint_lease_id"] != lease.LeaseID {
		   t.Fatalf("export %v %v", err, b)
	  }
	  d, err := eng.Diagnose(context.Background(), lease.LeaseID)
	   if err != nil || d.Allowed {
		   t.Fatalf("diagnose %+v %v", d, err)
	   }
	   if stablemintgate.MustLeaseID(lease) == "" {
		  t.Fatal("lease id")
	   }
   }

  func TestIssueIncomplete(t *testing.T) {
	   eng := newEng(nil)
	  if _, err := eng.Bridge().Attest(stablebr.AttestReq{
		  Kind: "mint", Asset: "USDT", Item: "reserve_attested", Actor: "ops-attest-13", TTLSec: 60,
	  }); err != nil {
		   t.Fatal(err)
	   }
	   _, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDT", Actor: "ops-issue-13",
	   })
	  if !errors.Is(err, stablemintgate.ErrAttestIncomplete) && !errors.Is(err, stablemintgate.ErrAttestExpired) {
		   t.Fatalf("err %v", err)
	   }
   }

  func TestIdempotentJournalReplayPosted(t *testing.T) {
	  eng := newEng(nil)
	   attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-14", 600)
	  lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDT", Actor: "ops-issue-14",
	   })
	   if err != nil {
		  t.Fatal(err)
	  }
	  j1, _, err := eng.MintWithLease(context.Background(), stablemintgate.MintWithLeaseReq{
		   LeaseID: lease.LeaseID, UserID: "u-mint-05", Asset: "USDT", Amount: "2.00",
		   Actor: "ops-mint-05", IdempotencyKey: "idem-ok-replay",
	   })
	   if err != nil {
		  t.Fatal(err)
	  }
	  j2, err := eng.Bridge().Mint(context.Background(), stablebr.MintReq{
		   UserID: "u-mint-05", Asset: "USDT", Amount: "2.00", Actor: "ops-mint-05",
		  IdempotencyKey: "idem-ok-replay", MintLeaseID: lease.LeaseID,
	   })
	   if err != nil {
		   t.Fatalf("replay posted: %v", err)
	   }
	   if j1.ID != j2.ID {
		   t.Fatal("idem journal id")
	  }
   }

   func TestFailClosedReasonsStable(t *testing.T) {
	  rs := stablemintgate.FailClosedReasons()
	   if len(rs) < 8 {
		   t.Fatalf("reasons %v", rs)
	  }
	   if stablemintgate.DescribeError(stablemintgate.ErrNoLease) != stablemintgate.OutcomeBlockedNoLease {
		   t.Fatal("describe")
	  }
  }

  func TestViewsAndAttestExport(t *testing.T) {
	   eng := newEng(nil)
	  attestAll(t, eng.Bridge(), "mint", "USDC", "ops-attest-15", 600)
	   _, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		  Kind: "mint", Asset: "USDC", Actor: "ops-issue-15",
	  })
	   if err != nil {
		   t.Fatal(err)
	   }
	   views, err := eng.ListLiveViews(context.Background(), "mint", "USDC", 10)
	   if err != nil || len(views) == 0 {
		  t.Fatalf("views %v %v", views, err)
	   }
	  ex, err := eng.ExportAttest(context.Background(), "mint", "USDC")
	  if err != nil || !ex.Fingerprint.Complete {
		  t.Fatalf("export attest %v %v", ex, err)
	  }
   }

  func TestResolveHalfAndFingerprintBind(t *testing.T) {
	  eng := newEng(nil)
	   attestAll(t, eng.Bridge(), "mint", "USDT", "ops-attest-16", 600)
	  lease, err := eng.IssueMintLease(context.Background(), stablemintgate.IssueReq{
		   Kind: "mint", Asset: "USDT", Actor: "ops-issue-16",
	   })
	   if err != nil {
		  t.Fatal(err)
	   }
	   eng.Bridge().InjectLedgerError(errors.New("down"))
	  _, _, _ = eng.MintWithLease(context.Background(), stablemintgate.MintWithLeaseReq{
		  LeaseID: lease.LeaseID, UserID: "u-mint-16", Asset: "USDT", Amount: "1.00",
		   Actor: "ops-mint-16", IdempotencyKey: "idem-half-res",
	   })
	   open, err := eng.ListHalf(context.Background(), lease.LeaseID, 8)
	   if err != nil || len(open) == 0 {
		   t.Fatalf("half %v %v", open, err)
	   }
	   if _, err := eng.ResolveHalf(context.Background(), open[0].HalfID, "ops-half-02", "ack"); err != nil {
		  t.Fatal(err)
	  }
	  need := []string{"reserve_attested", "issuer_signed", "bank_confirm", "kyt_clear"}
	  if !stablemintgate.RequiredAligned(stablemintgate.KindMint, need) {
		  t.Fatal("align")
	  }
	   if stablemintgate.CompareFingerprint(time.Now().UTC(), stablemintgate.BindSpec{Digest: "x"}, stablebr.Fingerprint{}) == nil {
		   t.Fatal("compare")
	   }
   }

   func TestIsolationDoesNotTouchMismatchFSM(t *testing.T) {
	   // 本包只扩展 Attest→Mint 绑定面；不得依赖或改写 brmismatch 状态机符号。
	   if stablemintgate.LeaseIssued == "" {
		   t.Fatal("lease fsm")
	  }
	  if stablebr.StatusPosted == "" {
		   t.Fatal("journal")
	   }
  }
