package fiatbalgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fiatbalgate"
)

// ─── stub JournalReader ───────────────────────────────────────────────────────

type stubReader struct {
	entries []fiatbalgate.JournalEntry
	err error
}

   func (s *stubReader) ListEntries(_ context.Context, asset, _ string, _ int) ([]fiatbalgate.JournalEntry, error) {
	  if s.err != nil {
		   return nil, s.err
	   }
	  var out []fiatbalgate.JournalEntry
	  for _, e := range s.entries {
		  if asset == "" || e.Asset == asset {
			   out = append(out, e)
		   }
	   }
	  return out, nil
  }

// ─── 辅助 ─────────────────────────────────────────────────────────────────────

   func newEngine(t *testing.T, reader fiatbalgate.JournalReader, opts ...fiatbalgate.EngineOption) (*fiatbalgate.Engine, *fiatbalgate.MemStore) {
	  t.Helper()
	  store := fiatbalgate.NewMemStore()
	  e := fiatbalgate.NewEngine(store, reader, opts...)
	  return e, store
  }

  func fixedClock(base time.Time) func() time.Time {
	  return func() time.Time { return base }
   }

   func mustOpenCase(t *testing.T, engine *fiatbalgate.Engine, asset string, genSeq uint64) *fiatbalgate.FiatImbalanceCase {
	  t.Helper()
	  c, err := engine.OpenCase(context.Background(), fiatbalgate.OpenCaseReq{
		   Asset: asset,
		  ScanGenSeq:      genSeq,
		  DetectedEntryID: "entry-001",
		   SumDebit:        "100",
		   SumCredit:       "90",
		   ImbalanceKind: fiatbalgate.ImbalanceBalancedMismatch,
		   ScannerID: "test-scanner",
	   })
	   if err != nil {
		   t.Fatalf("mustOpenCase: %v", err)
	  }
	   return c
   }

// ─── CheckBlock 表驱动 ────────────────────────────────────────────────────

  func TestCheckBlock(t *testing.T) {
	  base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   cases := []struct {
		  name    string
		  setupFn func(e *fiatbalgate.Engine)
		   asset string
		  wantBlocked bool
		   wantErr error
	  }{
		  {
			  name:    "clean asset no block",
			   setupFn: func(_ *fiatbalgate.Engine) {},
			   asset:   "USDT",
			   wantBlocked: false,
		  },
		  {
			   name: "open case blocks",
			  setupFn: func(e *fiatbalgate.Engine) {
				   mustOpenCase(t, e, "USDT", 1)
			   },
			  asset:       "USDT",
			   wantBlocked: true,
			  wantErr:     fiatbalgate.ErrBlockedByImbalance,
		   },
		  {
			  name: "blocking case blocks",
			  setupFn: func(e *fiatbalgate.Engine) {
				   c := mustOpenCase(t, e, "USDT", 1)
				  _, _ = e.PromoteToBlocking(context.Background(), c.CaseID)
			   },
			   asset: "USDT",
			   wantBlocked: true,
			  wantErr: fiatbalgate.ErrBlockedByImbalance,
		  },
		  {
			   name: "different asset not blocked",
			  setupFn: func(e *fiatbalgate.Engine) {
				   mustOpenCase(t, e, "USDT", 1)
			  },
			  asset: "BTC",
			   wantBlocked: false,
		   },
		  {
			   name: "empty asset returns error",
			   setupFn: func(_ *fiatbalgate.Engine) {},
			  asset:   "",
			  wantErr: fiatbalgate.ErrAssetRequired,
		  },
	  }
	   for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  e,
				  _ := newEngine(t, nil, fiatbalgate.WithClock(fixedClock(base)))
			   tc.setupFn(e)
			  dec, err := e.CheckBlock(context.Background(), tc.asset, 1)
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					  t.Errorf("want err %v got %v", tc.wantErr, err)
				  }
			  } else if err != nil && tc.asset != "" {
				  t.Errorf("unexpected err: %v", err)
			  }
			   if dec.Blocked != tc.wantBlocked {
				  t.Errorf("want Blocked=%v got=%v", tc.wantBlocked, dec.Blocked)
			   }
		  })
	  }
  }

// ─── OpenCase 表驱动 ──────────────────────────────────────────────────────

   func TestOpenCase(t *testing.T) {
	  base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   cases := []struct {
		   name string
		   req  fiatbalgate.OpenCaseReq
		   wantErr error
	   }{
		  {
			   name: "success",
			   req: fiatbalgate.OpenCaseReq{
				  Asset:           "USDT",
				  ScanGenSeq:      1,
				  DetectedEntryID: "entry-001",
				   SumDebit: "100",
				   SumCredit: "90",
				   ImbalanceKind: fiatbalgate.ImbalanceBalancedMismatch,
				   ScannerID:     "scanner-01",
			   },
		   },
		   {
			  name: "missing asset",
			  req: fiatbalgate.OpenCaseReq{
				   ScanGenSeq: 1,
				   ScannerID: "scanner-01",
			  },
			  wantErr: fiatbalgate.ErrAssetRequired,
		   },
		  {
			   name: "missing scanner id",
			  req: fiatbalgate.OpenCaseReq{
				   Asset:      "USDT",
				  ScanGenSeq: 1,
			   },
			  wantErr: fiatbalgate.ErrScannerIDRequired,
		  },
	   }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  e, _ := newEngine(t, nil, fiatbalgate.WithClock(fixedClock(base)))
			  c, err := e.OpenCase(context.Background(), tc.req)
			  if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					  t.Errorf("want err %v got %v", tc.wantErr, err)
				   }
				  return
			   }
			  if err != nil {
				  t.Fatalf("unexpected err: %v", err)
			   }
			  if c.CaseID == "" {
				   t.Error("CaseID should not be empty")
			  }
			   if c.State != fiatbalgate.CaseOpen {
				   t.Errorf("want state=open got=%s", c.State)
			  }
			  if c.Asset != tc.req.Asset {
				  t.Errorf("want asset=%s got=%s", tc.req.Asset, c.Asset)
			   }
		   })
	   }
  }

// ─── 幂等性：同资产重复开案 ────────────────────────────────────────────────────

  func TestOpenCaseIdempotent(t *testing.T) {
	  base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   e, _ := newEngine(t, nil, fiatbalgate.WithClock(fixedClock(base)))
	  // 第一次开案成功
	   _, err := e.OpenCase(context.Background(), fiatbalgate.OpenCaseReq{
		   Asset: "USDT",
		  ScanGenSeq: 1,
		  ImbalanceKind: fiatbalgate.ImbalanceBalancedMismatch,
		   ScannerID:     "scanner-01",
	  })
	   if err != nil {
		   t.Fatalf("first open: %v", err)
	  }
	  // 第二次同资产应返回 ErrAlreadyBlocking
	  _, err = e.OpenCase(context.Background(), fiatbalgate.OpenCaseReq{
		   Asset:      "USDT",
		   ScanGenSeq: 2,
		   ImbalanceKind: fiatbalgate.ImbalanceBalancedMismatch,
		  ScannerID:     "scanner-01",
	   })
	   if !errors.Is(err, fiatbalgate.ErrAlreadyBlocking) {
		  t.Errorf("second open want ErrAlreadyBlocking got %v", err)
	   }
   }

// ─── PromoteToBlocking ───────────────────────────────────────────────────

  func TestPromoteToBlocking(t *testing.T) {
	   base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   e, store := newEngine(t, nil, fiatbalgate.WithClock(fixedClock(base)))
	   c := mustOpenCase(t, e, "USDT", 1)
	  if c.State != fiatbalgate.CaseOpen {
		  t.Fatalf("want open got %s", c.State)
	   }
	   updated, err := e.PromoteToBlocking(context.Background(), c.CaseID)
	  if err != nil {
		  t.Fatalf("promote: %v", err)
	   }
	  if updated.State != fiatbalgate.CaseBlocking {
		   t.Errorf("want blocking got %s", updated.State)
	   }
	   // 幂等：重复 promote 应失败（CAS 校验 open→blocking，当前已是 blocking）
	  _, err = e.PromoteToBlocking(context.Background(), c.CaseID)
	  if !errors.Is(err, fiatbalgate.ErrInvalidTransition) {
		  t.Errorf("repeat promote want ErrInvalidTransition got %v", err)
	  }
	  // Store 中状态一致
	   fresh, err := store.GetCase(context.Background(), c.CaseID)
	   if err != nil {
		  t.Fatalf("get case: %v", err)
	   }
	  if fresh.State != fiatbalgate.CaseBlocking {
		  t.Errorf("store want blocking got %s", fresh.State)
	  }
  }

// ─── 四眼清障完整路径 ──────────────────────────────────────────────────────────

   func TestClearanceHappyPath(t *testing.T) {
	   base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   e, _ := newEngine(t, nil, fiatbalgate.WithClock(fixedClock(base)))
	   svc := fiatbalgate.NewClearanceService(e, fiatbalgate.NewMemStore())
	   // 需共享同一个 store
	   store := fiatbalgate.NewMemStore()
	  e2 := fiatbalgate.NewEngine(store,
		   nil, fiatbalgate.WithClock(fixedClock(base)))
	  svc2 := fiatbalgate.NewClearanceService(e2, store)
	   c := mustOpenCase(t, e2, "USDT", 1)
	   _, err := e2.PromoteToBlocking(context.Background(), c.CaseID)
	  if err != nil {
		  t.Fatalf("promote: %v", err)
	   }
	   // Step1 第一眼
	   status, err := svc2.SubmitStep1(context.Background(), fiatbalgate.ClearReq{
		  CaseID: c.CaseID,
		   Actor:  "ops-alice",
		  Note:   "已核查凭证借贷差异，确认属实，提交第一眼清障",
	   })
	  if err != nil {
		   t.Fatalf("step1: %v", err)
	   }
	   if len(status.Approvals) != 1 {
		  t.Errorf("want 1 approval got %d", len(status.Approvals))
	  }
	  // Step1 同 actor 重复提交应失败
	   _, err = svc2.SubmitStep1(context.Background(), fiatbalgate.ClearReq{
		  CaseID: c.CaseID,
		   Actor:  "ops-alice",
		   Note: "重复提交不应成功，应被拒绝以防自我双签",
	  })
	  if !errors.Is(err, fiatbalgate.ErrAlreadyApproved) {
		  t.Errorf("dup step1 want ErrAlreadyApproved got %v", err)
	  }
	   // Step2 第二眼（不同 actor）
	  clearStatus, proof, err := svc2.SubmitStep2(context.Background(), fiatbalgate.ClearReq{
		  CaseID: c.CaseID,
		   Actor: "ops-bob",
		  Note: "第二眼确认：凭证失衡已记录，清障后将重新扫描，授权放行",
	   })
	   if err != nil {
		   t.Fatalf("step2: %v", err)
	  }
	  if clearStatus.CaseState != fiatbalgate.CaseCleared {
		   t.Errorf("want cleared got %s", clearStatus.CaseState)
	  }
	   if proof == nil || proof.ProofID == "" {
		  t.Error("proof should not be nil")
	  }
	   // 清障后 CheckBlock 不再挡写
	  dec, _ := e2.CheckBlock(context.Background(), "USDT", 2)
	  if dec.Blocked {
		  t.Error("after clearance CheckBlock should not block")
	  }
	  _ = svc
  }

// ─── 四眼清障：同 actor 自我双签失败 ──────────────────────────────────────────

  func TestClearanceSelfSignForbidden(t *testing.T) {
	   base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   store := fiatbalgate.NewMemStore()
	   e := fiatbalgate.NewEngine(store, nil, fiatbalgate.WithClock(fixedClock(base)))
	  svc := fiatbalgate.NewClearanceService(e, store)
	   c := mustOpenCase(t, e, "USDT", 1)
	   _, _ = e.PromoteToBlocking(context.Background(), c.CaseID)
	   _, err := svc.SubmitStep1(context.Background(), fiatbalgate.ClearReq{
		  CaseID: c.CaseID, Actor: "ops-alice",
		  Note: "第一眼确认：凭证失衡已记录，申请清障授权",
	  })
	   if err != nil {
		   t.Fatalf("step1: %v", err)
	   }
	   // 同 actor 提交 step2 必须失败
	   _, _, err = svc.SubmitStep2(context.Background(), fiatbalgate.ClearReq{
		   CaseID: c.CaseID, Actor: "ops-alice",
		  Note: "自我双签必须被拒绝，这行注释是预期",
	  })
	  if !errors.Is(err, fiatbalgate.ErrAlreadyApproved) {
		  t.Errorf("self-sign step2 want ErrAlreadyApproved got %v", err)
	   }
  }

// ─── SyncGen ─────────────────────────────────────────────────────────────

  func TestSyncGen(t *testing.T) {
	  base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   e, store := newEngine(t, nil, fiatbalgate.WithClock(fixedClock(base)))
	  req := fiatbalgate.SyncGenReq{
		  Seq:     5,
		  EntryID: "entry-005",
		  Kind:    "post",
		   Asset: "USDT",
		  BumpedAt: base,
	   }
	   if err := e.SyncGen(context.Background(), req); err != nil {
		  t.Fatalf("sync gen: %v", err)
	  }
	   // 幂等：重复写同 Seq 应静默成功
	  if err := e.SyncGen(context.Background(), req); err != nil {
		  t.Fatalf("sync gen idempotent: %v", err)
	   }
	  maxSeq,
		   err := store.MaxGenSeq(context.Background())
	  if err != nil {
		  t.Fatalf("max gen seq: %v", err)
	   }
	   if maxSeq != 5 {
		   t.Errorf("want maxSeq=5 got=%d", maxSeq)
	  }
   }

// ─── ExportProof + VerifyBundle ──────────────────────────────────────────

   func TestExportProofAndVerify(t *testing.T) {
	  base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	  store := fiatbalgate.NewMemStore()
	   e := fiatbalgate.NewEngine(store,
		   nil, fiatbalgate.WithClock(fixedClock(base)))
	   svc := fiatbalgate.NewClearanceService(e,
		   store)
	   exporter := fiatbalgate.NewProofExporter(e, store)
	  // 开案 → 推进 → 清障
	   c := mustOpenCase(t, e, "USDT", 1)
	   _, _ = e.PromoteToBlocking(context.Background(), c.CaseID)
	  _, err := svc.SubmitStep1(context.Background(), fiatbalgate.ClearReq{
		   CaseID: c.CaseID, Actor: "ops-alice",
		   Note: "第一眼：凭证失衡已审核，准予清障",
	   })
	  if err != nil {
		  t.Fatalf("step1: %v", err)
	   }
	  _, proof, err := svc.SubmitStep2(context.Background(), fiatbalgate.ClearReq{
		   CaseID: c.CaseID, Actor: "ops-bob",
		   Note: "第二眼：二次复核完成，清障执行",
	   })
	  if err != nil {
		  t.Fatalf("step2: %v", err)
	  }
	   // 导出证明
	   bundle, err := exporter.Export(context.Background(), proof.ProofID, "ops-alice")
	  if err != nil {
		   t.Fatalf("export: %v", err)
	   }
	   if bundle.State != fiatbalgate.ProofExported {
		   t.Errorf("want exported got %s", bundle.State)
	   }
	  // 离线复验摘要
	   if err := fiatbalgate.VerifyBundle(bundle); err != nil {
		  t.Errorf("verify bundle: %v", err)
	  }
	  // 幂等导出（重复调用应直接返回当前状态）
	  bundle2, err := exporter.Export(context.Background(), proof.ProofID, "ops-alice")
	   if err != nil {
		   t.Fatalf("re-export: %v", err)
	   }
	  if bundle2.DigestSHA256 != bundle.DigestSHA256 {
		  t.Error("re-export digest should match")
	  }
   }

// ─── Scanner 幂等扫描 ──────────────────────────────────────────────────────

  func TestScannerIdempotent(t *testing.T) {
	   base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   store := fiatbalgate.NewMemStore()
	   reader := &stubReader{
		   entries: []fiatbalgate.JournalEntry{
			   {
				   ID:    "entry-001",
				   Asset: "USDT",
				   Kind: "post",
				   Lines: []fiatbalgate.JournalLine{
					  {Asset: "USDT", Debit: "100"},
					  {Asset: "USDT", Credit: "90"}, // 不平衡
				   },
				   Balanced: false,
			   },
		  },
	  }
	   e := fiatbalgate.NewEngine(store, reader, fiatbalgate.WithClock(fixedClock(base)))
	   _ = e.SyncGen(context.Background(), fiatbalgate.SyncGenReq{
		   Seq: 1, EntryID: "entry-001", Kind: "post", Asset: "USDT", BumpedAt: base,
	  })
	  sc := fiatbalgate.NewScanner(e, store, reader)
	   // 第一次扫描：应开 1 个新案
	   result1, err := sc.Scan(context.Background(), fiatbalgate.ScanReq{Asset: "USDT", ScannerID: "scanner-01"})
	   if err != nil {
		  t.Fatalf("scan1: %v", err)
	  }
	  if result1.NewCases != 1 {
		  t.Errorf("scan1 want 1 new case got %d", result1.NewCases)
	  }
	   // 第二次扫描：幂等，不重复开案
	   result2, err := sc.Scan(context.Background(), fiatbalgate.ScanReq{Asset: "USDT", ScannerID: "scanner-01"})
	   if err != nil {
		   t.Fatalf("scan2: %v", err)
	  }
	  if result2.NewCases != 0 {
		   t.Errorf("scan2 want 0 new case (idempotent) got %d", result2.NewCases)
	   }
	  if result2.AlreadyOpen != 1 {
		  t.Errorf("scan2 want 1 already_open got %d", result2.AlreadyOpen)
	   }
   }

// ─── Post 挡写后清障恢复路径 ────────────────────────────────────────────────────

   func TestBlockThenClear(t *testing.T) {
	  base := time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)
	   store := fiatbalgate.NewMemStore()
	   e := fiatbalgate.NewEngine(store,
		   nil, fiatbalgate.WithClock(fixedClock(base)))
	   svc := fiatbalgate.NewClearanceService(e, store)
	   // 开案 → 推进到 blocking
	   c := mustOpenCase(t, e, "USDT", 1)
	   _, _ = e.PromoteToBlocking(context.Background(), c.CaseID)
	   // Post 路径被挡写
	   dec, err := e.CheckBlock(context.Background(), "USDT", 2)
	  if !dec.Blocked {
		  t.Error("want blocked before clearance")
	  }
	  if !errors.Is(err, fiatbalgate.ErrBlockedByImbalance) {
		  t.Errorf("want ErrBlockedByImbalance got %v", err)
	   }
	   // 执行四眼清障
	   _, _ = svc.SubmitStep1(context.Background(), fiatbalgate.ClearReq{
		  CaseID: c.CaseID, Actor: "ops-carol",
		  Note: "第一眼：不平衡已记录，授权清障",
	  })
	  _, _, err = svc.SubmitStep2(context.Background(), fiatbalgate.ClearReq{
		  CaseID: c.CaseID, Actor: "ops-dave",
		   Note: "第二眼：二次确认完成，放行恢复 Post",
	   })
	  if err != nil {
		   t.Fatalf("step2: %v", err)
	  }
	   // 清障后 CheckBlock 放行
	   dec2, err2 := e.CheckBlock(context.Background(), "USDT", 3)
	  if dec2.Blocked {
		   t.Error("after clearance want not blocked")
	   }
	  if err2 != nil {
		   t.Errorf("after clearance err: %v", err2)
	  }
   }

// ─── CaseState Terminal ──────────────────────────────────────────────────

  func TestCaseStateTerminal(t *testing.T) {
	   cases := []struct {
		  state    fiatbalgate.CaseState
		   terminal bool
	  }{
		   {fiatbalgate.CaseOpen, false},
		   {fiatbalgate.CaseBlocking, false},
		  {fiatbalgate.CaseCleared, true},
	   }
	   for _, tc := range cases {
		   if tc.state.Terminal() != tc.terminal {
			   t.Errorf("state %s Terminal() want=%v got=%v", tc.state, tc.terminal, tc.state.Terminal())
		  }
	   }
  }

// ─── VerifyBundle 摘要篡改 ────────────────────────────────────────────────

   func TestVerifyBundleTampered(t *testing.T) {
	  b := &fiatbalgate.ProofBundle{
		   ProofID: "p-001",
		  CaseID:          "c-001",
		   DigestSHA256:    "aabbcc",
		  EntriesSnapshot: `["e1","e2"]`,
		   LinesSnapshot:   `[{"asset":"USDT"}]`,
	   }
	   if err := fiatbalgate.VerifyBundle(b); err == nil {
		   t.Error("tampered bundle should fail verification")
	  }
  }
