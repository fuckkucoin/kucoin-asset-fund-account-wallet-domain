package subfreezecas_test

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
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/subfreezecas"
	"github.com/google/uuid"
)

  func newEng(clock func() time.Time) *subfreezecas.Engine {
	  books := subacct.New()
	   eng := subfreezecas.NewEngine(
		books,
		subfreezecas.NewMemCASRepo(),
		subfreezecas.NewMemProofRepo(),
		subfreezecas.NewMemRevokeRepo(),
		subfreezecas.NewMemAuditRepo(),
		subfreezecas.NewMemJournalRepo(),
	)
	if clock != nil {
		eng = eng.WithClock(clock)
	}
	return eng
}

   func loadGolden(t *testing.T, name string) map[string]any {
	  t.Helper()
	   p := filepath.Join("testdata", "subfreezecas", name)
	  if _, err := os.Stat(p); err != nil {
		  p = filepath.Join("..", "..", "testdata", "subfreezecas", name)
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

   func openTree(t *testing.T, eng *subfreezecas.Engine) (parent subacct.Parent, child subacct.Child) {
	  t.Helper()
	   p, err := eng.Books().OpenParent(subacct.OpenParentReq{UserID: "u-parent-cas-01", Label: "main"})
	  if err != nil {
		   t.Fatalf("open parent: %v", err)
	   }
	   c, err := eng.Books().OpenChild(subacct.OpenChildReq{
		  ParentID: p.ID, UserID: "u-child-cas-01", Label: "mm",
	  })
	  if err != nil {
		  t.Fatalf("open child: %v", err)
	  }
	  if _, err := eng.Books().Credit(subacct.PostReq{
		   OwnerID: c.ID, Asset: "USDT", Amount: "100", IdempotencyKey: "c1",
	   }); err != nil {
		   t.Fatalf("credit: %v", err)
	   }
	   return p, c
  }

   func TestFSMMatrixMatchesGolden(t *testing.T) {
	  g := loadGolden(t, "fsm_transitions.json")
	  cas := subfreezecas.CASMatrixRows()
	  want, _ := json.Marshal(g["cas"])
	   got, _ := json.Marshal(cas)
	   if string(want) != string(got) {
		   t.Fatalf("cas matrix\nwant %s\ngot %s", want, got)
	   }
	   pr := subfreezecas.ProofMatrixRows()
	  wantp, _ := json.Marshal(g["proof"])
	  gotp, _ := json.Marshal(pr)
	  if string(wantp) != string(gotp) {
		   t.Fatalf("proof matrix\nwant %s\ngot %s", wantp, gotp)
	  }
	  if subfreezecas.CanTransitionCAS(subfreezecas.CASFrozen, subfreezecas.CASOpen) {
		  t.Fatal("frozen->open forbidden")
	   }
	   if subfreezecas.CanTransitionCAS(subfreezecas.CASThawed, subfreezecas.CASFrozen) {
		  t.Fatal("thawed->frozen forbidden")
	  }
   }

   func TestFreezeParentThenDebitDenied(t *testing.T) {
	   base := time.Date(2020,
		  11, 18, 10, 0, 0, 0, time.UTC)
	  eng := newEng(func() time.Time { return base })
	  p, c := openTree(t, eng)
	   cas, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		  ParentID: p.ID, Reason: "risk", Actor: "ops-freeze-01",
	  })
	   if err != nil {
		  t.Fatalf("freeze: %v", err)
	   }
	   if cas.FreezeCASID == "" || cas.State != subfreezecas.CASFrozen {
		   t.Fatalf("cas %+v", cas)
	   }
	  _, err = eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
		   OwnerID: c.ID, Asset: "USDT", Amount: "1", FreezeCASID: cas.FreezeCASID, IdempotencyKey: "d1",
	   })
	   if err == nil {
		   t.Fatal("debit after freeze must fail")
	  }
	   ch, _ := eng.Books().GetChild(c.ID)
	   if ch.Status != subacct.StatusFrozen {
		  t.Fatalf("child status %s", ch.Status)
	  }
	   bal, _ := eng.Books().BookOf(c.ID, "USDT")
	   if bal.Available != "100" {
		  t.Fatalf("avail %s", bal.Available)
	   }
  }

  func TestDebitBeforeFreezeOK(t *testing.T) {
	  eng := newEng(nil)
	   p, c := openTree(t, eng)
	  open, err := eng.UnfreezeParent(context.Background(), p.ID, "ops-thaw-01")
	   if err == nil && open != nil && open.State == subfreezecas.CASOpen {
		   // 无冻记录时 Unfreeze 可能失败
	  }
	   _ = open
	   // 签发 open 世代：先冻再解，得到 open cas
	   _, err = eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "tmp", Actor: "ops-freeze-02", IdempotencyKey: "fp-tmp",
	   })
	  if err != nil {
		  t.Fatalf("freeze: %v", err)
	   }
	  opened, err := eng.UnfreezeParent(context.Background(), p.ID, "ops-thaw-02")
	  if err != nil {
		  t.Fatalf("thaw: %v", err)
	  }
	  // 子户仍冻，须在母已解后单独解
	  if err := eng.UnfreezeChild(context.Background(), c.ID, "ops-thaw-02"); err != nil {
		   t.Fatalf("unfreeze child: %v", err)
	   }
	   got, err := eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
		   OwnerID: c.ID, Asset: "USDT", Amount: "3", FreezeCASID: opened.FreezeCASID, IdempotencyKey: "d-ok",
	   })
	   if err != nil {
		   t.Fatalf("debit: %v", err)
	   }
	  if got.FreezeCASID != opened.FreezeCASID {
		   t.Fatalf("cas id %s", got.FreezeCASID)
	  }
   }

  func TestMissingCASFailClosed(t *testing.T) {
	  g := loadGolden(t, "fail_closed.json")
	   if g["cases"] == nil {
		  t.Fatal("golden cases")
	   }
	  eng := newEng(nil)
	   _, c := openTree(t, eng)
	   _, err := eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
		  OwnerID: c.ID, Asset: "USDT", Amount: "1", FreezeCASID: "", IdempotencyKey: "d-miss",
	  })
	  if !errors.Is(err, subfreezecas.ErrCASMissing) {
		  t.Fatalf("want missing cas, got %v", err)
	   }
	   _, err = eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
		   OwnerID: c.ID, Asset: "USDT", Amount: "1", FreezeCASID: "not-a-cas", IdempotencyKey: "d-miss2",
	  })
	  if err == nil {
		   t.Fatal("unknown cas must fail")
	   }
  }

  func TestStaleGenerationDenied(t *testing.T) {
	   eng := newEng(nil)
	   p, c := openTree(t, eng)
	  first, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "a", Actor: "ops-freeze-03",
	  })
	   if err != nil {
		  t.Fatalf("freeze1: %v", err)
	  }
	   if _, err := eng.UnfreezeParent(context.Background(), p.ID, "ops-thaw-03"); err != nil {
		  t.Fatalf("thaw: %v", err)
	   }
	  if err := eng.UnfreezeChild(context.Background(), c.ID, "ops-thaw-03"); err != nil {
		   t.Fatalf("unfreeze child: %v", err)
	  }
	  _, err = eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
		  OwnerID: c.ID, Asset: "USDT", Amount: "1", FreezeCASID: first.FreezeCASID, IdempotencyKey: "stale",
	  })
	   if err == nil {
		  t.Fatal("stale cas must fail")
	  }
  }

  func TestVelocityHitRevokedAfterFreeze(t *testing.T) {
	   g := loadGolden(t, "velocity_revoke.json")
	   exp := g["expect"].(map[string]any)
	   if exp["post_freeze_debit_ok"] != false {
		  t.Fatal("golden")
	  }
	   eng := newEng(nil)
	  p, c := openTree(t, eng)
	  _, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "tmp", Actor: "ops-freeze-04",
	  })
	   if err != nil {
		  t.Fatalf("freeze tmp: %v", err)
	  }
	  opened, err := eng.UnfreezeParent(context.Background(), p.ID, "ops-thaw-04")
	  if err != nil {
		  t.Fatalf("thaw: %v", err)
	   }
	   if err := eng.UnfreezeChild(context.Background(), c.ID, "ops-thaw-04"); err != nil {
		   t.Fatalf("unfreeze child: %v", err)
	  }
	   if _, err := eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
		   OwnerID: c.ID, Asset: "USDT", Amount: "2", FreezeCASID: opened.FreezeCASID, IdempotencyKey: "hit1",
	   }); err != nil {
		  t.Fatalf("pre hit: %v", err)
	  }
	  cas, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "risk2", Actor: "ops-freeze-05",
	  })
	  if err != nil {
		   t.Fatalf("freeze: %v", err)
	   }
	   revs, err := eng.ListRevocations(context.Background(), cas.FreezeCASID)
	  if err != nil || len(revs) == 0 {
		   t.Fatalf("revocations %v %d", err, len(revs))
	  }
	  _, err = eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
		   OwnerID: c.ID, Asset: "USDT", Amount: "2", FreezeCASID: cas.FreezeCASID, IdempotencyKey: "hit2",
	  })
	   if err == nil {
		   t.Fatal("post freeze debit must fail")
	  }
  }

  func TestUnfreezeChildWhileParentFrozen(t *testing.T) {
	   g := loadGolden(t, "unfreeze_child_parent_frozen.json")
	  exp := g["expect"].(map[string]any)
	   if exp["unfreeze_child_ok"] != false {
		   t.Fatal("golden")
	  }
	  eng := newEng(nil)
	  p, c := openTree(t, eng)
	  if _, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "risk", Actor: "ops-freeze-06",
	  }); err != nil {
		  t.Fatalf("freeze: %v", err)
	  }
	   err := eng.UnfreezeChild(context.Background(), c.ID, "ops-thaw-06")
	  if !errors.Is(err, subfreezecas.ErrUnfreezeChild) {
		  t.Fatalf("want unfreeze denied, got %v", err)
	   }
	   audits,
		  err := eng.ListAudit(context.Background(), p.ID, 20)
	  if err != nil {
		  t.Fatalf("audit: %v", err)
	   }
	  found := false
	  for _, a := range audits {
		   if a.Action == subfreezecas.ActionUnfreezeChild && a.Outcome == subfreezecas.OutcomeDenied {
			  found = true
			   break
		  }
	  }
	   if !found {
		   t.Fatal("missing unfreeze deny audit")
	   }
	   ch, _ := eng.Books().GetChild(c.ID)
	   if ch.Status != subacct.StatusFrozen {
		  t.Fatalf("child thawed illegally: %s", ch.Status)
	  }
   }

   func TestFreezeParentIdempotent(t *testing.T) {
	  eng := newEng(nil)
	  p, _ := openTree(t, eng)
	  a, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		  ParentID: p.ID, Reason: "risk", Actor: "ops-freeze-07", IdempotencyKey: "idem-fp",
	  })
	   if err != nil {
		  t.Fatalf("freeze1: %v", err)
	   }
	  b, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		  ParentID: p.ID, Reason: "risk", Actor: "ops-freeze-07", IdempotencyKey: "idem-fp",
	   })
	  if err != nil {
		   t.Fatalf("freeze2: %v", err)
	  }
	  if a.FreezeCASID != b.FreezeCASID {
		   t.Fatalf("idem cas %s %s", a.FreezeCASID, b.FreezeCASID)
	   }
  }

   func TestCreditWhileFrozenOK(t *testing.T) {
	  eng := newEng(nil)
	  p, c := openTree(t, eng)
	  if _, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		  ParentID: p.ID, Reason: "risk", Actor: "ops-freeze-08",
	  }); err != nil {
		  t.Fatalf("freeze: %v", err)
	   }
	  if _, err := eng.Books().Credit(subacct.PostReq{
		   OwnerID: c.ID, Asset: "USDT", Amount: "5", IdempotencyKey: "cr-frozen",
	   }); err != nil {
		  t.Fatalf("credit frozen: %v", err)
	  }
   }

  func TestInspectPunchCompensates(t *testing.T) {
	   g := loadGolden(t, "punch_compensate.json")
	   exp := g["expect"].(map[string]any)
	   if exp["compensated"] != true {
		  t.Fatal("golden")
	  }
	   eng := newEng(nil)
	   p, c := openTree(t, eng)
	   cas, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "risk", Actor: "ops-freeze-09",
	   })
	   if err != nil {
		  t.Fatalf("freeze: %v", err)
	  }
	  if err := eng.SeedJournal(context.Background(), subfreezecas.DebitJournal{
		   JournalID: uuid.NewString(), FreezeCASID: cas.FreezeCASID, OwnerID: c.ID, ParentID: p.ID, ChildID: c.ID,
		  Asset: "USDT", Amount: "7", Idem: "punch-1", OK: true, AfterFrozen: true, At: time.Now().UTC(),
	  }); err != nil {
		   t.Fatalf("seed: %v", err)
	   }
	  // 击穿已扣账
	  if _, err := eng.Books().UnfreezeChild(c.ID); err == nil {
		  t.Fatal("unfreeze child while parent frozen must fail on books")
	   }
	   n, err := eng.InspectPunch(context.Background(), 20)
	  if err != nil {
		   t.Fatalf("inspect: %v", err)
	   }
	  if n < 1 {
		   t.Fatalf("fixed %d", n)
	   }
	  proofs, err := eng.ListProofsByCAS(context.Background(), cas.FreezeCASID, 10)
	  if err != nil || len(proofs) == 0 {
		   t.Fatalf("proofs %v %d", err, len(proofs))
	   }
	   if !proofs[0].Compensated {
		   t.Fatal("not compensated")
	  }
	   acked, err := eng.AckProof(context.Background(), proofs[0].ProofID, "ops-ack-01", "ok")
	   if err != nil {
		  t.Fatalf("ack: %v", err)
	   }
	  if acked.State != subfreezecas.ProofAcked {
		   t.Fatalf("state %s", acked.State)
	  }
   }

   func TestFreezeDebitRace(t *testing.T) {
	   g := loadGolden(t, "freeze_debit_race.json")
	   exp := g["expect"].(map[string]any)
	   if exp["frozen_debit_ok"] != false {
		   t.Fatal("golden")
	   }
	   eng := newEng(nil)
	  p, c := openTree(t, eng)
	   _, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "tmp", Actor: "ops-freeze-10",
	  })
	   if err != nil {
		  t.Fatalf("tmp freeze: %v", err)
	  }
	  opened, err := eng.UnfreezeParent(context.Background(), p.ID, "ops-thaw-10")
	   if err != nil {
		  t.Fatalf("thaw: %v", err)
	  }
	   if err := eng.UnfreezeChild(context.Background(), c.ID, "ops-thaw-10"); err != nil {
		  t.Fatalf("unfreeze child: %v", err)
	   }
	   var debitOK atomic.Int32
	  var debitErr atomic.Int32
	   var wg sync.WaitGroup
	  wg.Add(2)
	   go func() {
		   defer wg.Done()
		   _, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
			   ParentID: p.ID, Reason: "race", Actor: "ops-freeze-11",
		   })
		  if err != nil {
			   t.Errorf("freeze race: %v", err)
		   }
	  }()
	  go func() {
		  defer wg.Done()
		   _, err := eng.DebitAssertCAS(context.Background(), subfreezecas.DebitCASReq{
			  OwnerID: c.ID, Asset: "USDT", Amount: "4", FreezeCASID: opened.FreezeCASID, IdempotencyKey: "race-d",
		   })
		   if err != nil {
			   debitErr.Add(1)
			   return
		  }
		  debitOK.Add(1)
	  }()
	   wg.Wait()
	   ch, _ := eng.Books().GetChild(c.ID)
	   bal, _ := eng.Books().BookOf(c.ID, "USDT")
	  if ch.Status != subacct.StatusFrozen {
		  t.Fatalf("child not frozen: %s", ch.Status)
	  }
	  if debitOK.Load() > 0 {
		   // 借记先于冻结完成：冻前花出合法，余额 96。
		  if bal.Available != "96" {
			  t.Fatalf("pre-freeze debit avail=%s", bal.Available)
		  }
		  return
	   }
	   if debitErr.Load() == 0 {
		   t.Fatal("race produced neither debit ok nor error")
	  }
	  if bal.Available != "100" {
		  proofs, _ := eng.ListOpenProofs(context.Background(), 20)
		  if len(proofs) == 0 {
			  t.Fatalf("frozen debit leftover avail=%s without proof", bal.Available)
		   }
	  }
   }

   func TestWorkersTick(t *testing.T) {
	   eng := newEng(nil)
	  p, _ := openTree(t, eng)
	  if _, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		  ParentID: p.ID, Reason: "w", Actor: "ops-freeze-12",
	  }); err != nil {
		   t.Fatalf("freeze: %v", err)
	  }
	  pw := subfreezecas.NewPunchWorker(eng, subfreezecas.WorkerConfig{BatchSize: 10})
	  if _, err := pw.TickOnce(context.Background()); err != nil {
		  t.Fatalf("punch tick: %v", err)
	  }
	   rw := subfreezecas.NewRevokeWorker(eng, subfreezecas.WorkerConfig{BatchSize: 10})
	  if _, err := rw.TickOnce(context.Background()); err != nil {
		   t.Fatalf("revoke tick: %v", err)
	  }
	  ew := subfreezecas.NewExportWorker(eng, subfreezecas.WorkerConfig{BatchSize: 10})
	   if _, err := ew.TickOnce(context.Background()); err != nil {
		   t.Fatalf("export tick: %v", err)
	   }
  }

  func TestExportContainsCASID(t *testing.T) {
	   eng := newEng(nil)
	   p, _ := openTree(t, eng)
	  cas, err := eng.FreezeParentWithCAS(context.Background(), subfreezecas.FreezeParentReq{
		   ParentID: p.ID, Reason: "x", Actor: "ops-freeze-13",
	  })
	  if err != nil {
		   t.Fatalf("freeze: %v", err)
	  }
	  b, err := eng.ExportCAS(context.Background(), cas.FreezeCASID)
	  if err != nil {
		   t.Fatalf("export: %v", err)
	  }
	   if b.FreezeCASID != cas.FreezeCASID || b.CAS == nil {
		  t.Fatalf("bundle %+v", b)
	   }
   }

   func TestFingerprintStable(t *testing.T) {
	  a := subfreezecas.ChildFingerprint([]string{"b", "a", "a"})
	  b := subfreezecas.ChildFingerprint([]string{"a", "b"})
	   if a != b || a == "" {
		   t.Fatalf("fp %s %s", a, b)
	   }
   }
