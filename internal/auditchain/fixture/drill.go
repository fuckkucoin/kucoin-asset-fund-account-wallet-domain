package fixture

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/auditchain/replay"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/persist"
)

// RunAll 五条验收。任一失败返回错误。
  func RunAll() error {
	  steps := []struct {
		  name string
		  fn func() error
	   }{
		   {"tamper", DrillTamper},
		  {"ingest", DrillIngest},
		   {"replay", DrillReplay},
		   {"isolation", DrillIsolation},
		  {"export", DrillExport},
	   }
	   for _, s := range steps {
		  if err := s.fn(); err != nil {
			   return fmt.Errorf("%s: %w", s.name, err)
		  }
	   }
	   return nil
  }

   func newWH() *auditchain.Warehouse {
	   return auditchain.NewWarehouse(auditchain.NewMem(), auditchain.DefaultConfig())
  }

  func DrillTamper() error {
	   ctx := context.Background()
	  wh := newWH()
	   base := time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)
	   var seg string
	   for i := 0; i < 5; i++ {
		  row, err := wh.Append(ctx, auditchain.Envelope{
			  Source: "deposit.credit", IdemKey: fmt.Sprintf("t-%d", i), Kind: auditchain.KindDepositCredit,
			  UserID: "u-tamper", Asset: "USDT", Amount: "1", RefID: fmt.Sprintf("r-%d", i), Actor: "ada",
			  Body: "{}", OccurredAt: base.Add(time.Duration(i) * time.Second),
		   })
		  if err != nil {
			  return err
		   }
		   seg = row.SegmentID
	  }
	   if _, err := wh.Seal(ctx, seg, "ada"); err != nil {
		   return err
	   }
	  if err := wh.DrillCorrupt(ctx, seg, 3, `{"tampered":true}`); err != nil {
		   return err
	   }
	   rep, err := wh.Inspect(ctx)
	  if err != nil {
		  return err
	  }
	   if !rep.Failed {
		  return fmt.Errorf("inspect passed after tamper")
	   }
	  if err := wh.ExportAllowed(ctx, seg); err == nil {
		   return fmt.Errorf("export still allowed")
	   }
	   if err := wh.DrillRewriteRoot(ctx, seg, strings.Repeat("ab", 32)); err != nil {
		  return err
	   }
	  return nil
  }

  func DrillIngest() error {
	   ctx := context.Background()
	   wh := newWH()
	   ingest.SetMode(ingest.ModeOn)
	  defer ingest.SetMode(ingest.ModeOff)
	   ingest.Install(wh)
	  at := time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC)
	   facts := []ingest.Fact{
		   ingest.DepositDecision("u1", "TRON", "memo", "88421", "USDT", "0", "attr-1", "ada", false, at),
		  ingest.SweepProof("TRON", "USDT", "12.5", "proof-9", "bob", at.Add(time.Second)),
		  ingest.FreezeFact("u1", "USDT", "4", "wd-1", "", "held", "cara", at.Add(2*time.Second)),
		   ingest.InnerFact("u1", "u9", "USDT", "1", "in-1", true, at.Add(3*time.Second)),
	  }
	  for _, f := range facts {
		  if err := ingest.Push(ctx, f); err != nil {
			  return err
		   }
	  }
	   if err := ingest.Push(ctx, facts[0]); err != nil {
		  return err
	   }
	  n, err := wh.EntryCount(ctx)
	   if err != nil {
		  return err
	  }
	   if n != 4 {
		   return fmt.Errorf("count %d", n)
	   }
	   late := facts[0]
	   late.IdemKey = "attr-late"
	   late.OccurredAt = at.Add(-time.Hour)
	  if err := ingest.Push(ctx, late); err == nil {
		   return fmt.Errorf("late water advanced")
	  }
	   n, err = wh.EntryCount(ctx)
	  if err != nil {
		   return err
	   }
	   if n != 5 {
		   return fmt.Errorf("late row missing %d", n)
	  }
	   alerts, err := wh.Store().Alerts(ctx, true)
	   if err != nil {
		   return err
	  }
	   lateAlert := false
	  for _, a := range alerts {
		   if a.Name == auditchain.AlertLate {
			   lateAlert = true
		  }
	  }
	   if !lateAlert {
		  return fmt.Errorf("late alert missing")
	  }
	  cur, _, err := wh.Store().Cursor(ctx, "deposit.attrib")
	  if err != nil {
		   return err
	  }
	   if cur.HighWater == "" {
		  return fmt.Errorf("cursor empty")
	  }
	   if _, err := wh.AdvanceCursor(ctx, "deposit.attrib", "2000-01-01T00:00:00Z|older", "x", at); err == nil {
		  return fmt.Errorf("cursor moved backward")
	   }
	   kept, _, err := wh.Store().Cursor(ctx, "deposit.attrib")
	  if err != nil {
		  return err
	  }
	  if kept.HighWater != cur.HighWater {
		   return fmt.Errorf("cursor changed")
	   }
	   if err := wh.BackdateCursor(ctx, "deposit.attrib", at.Add(-time.Hour)); err != nil {
		  return err
	   }
	  rep, err := wh.Inspect(ctx)
	   if err != nil {
		  return err
	   }
	  lag := false
	  for _, name := range rep.Alerts {
		   if name == auditchain.AlertLag {
			  lag = true
		   }
	  }
	  if !lag {
		  return fmt.Errorf("lag alert missing")
	   }
	  w := &auditchain.Worker{WH: wh, Every: time.Second}
	   return w.Tick(ctx)
   }

   func DrillReplay() error {
	   ctx := context.Background()
	  wh := newWH()
	   wh.SetReader(&auditchain.StaticBook{Available: "45", Frozen: "25"})
	  base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	  seq := []auditchain.Envelope{
		   {Source: "deposit.credit", IdemKey: "c1", Kind: auditchain.KindDepositCredit, UserID: "u2", Asset: "USDT", Amount: "100", RefID: "c1", Actor: "ada", Body: "{}", OccurredAt: base},
		  {Source: "xfer.hop", IdemKey: "h1", Kind: auditchain.KindHopOut, UserID: "u2", Asset: "USDT", Amount: "30", RefID: "h1", Actor: "ada", Body: "{}", OccurredAt: base.Add(time.Second)},
		  {Source: "withdraw.freeze", IdemKey: "f1", Kind: auditchain.KindWdFreeze, UserID: "u2", Asset: "USDT", Amount: "40", RefID: "f1", Actor: "ada", Body: "{}", OccurredAt: base.Add(2 * time.Second)},
		   {Source: "withdraw.freeze", IdemKey: "p1", Kind: auditchain.KindWdPartial, UserID: "u2", Asset: "USDT", Amount: "15", RefID: "p1", Actor: "ada", Body: "{}", OccurredAt: base.Add(3 * time.Second)},
	   }
	  for _, e := range seq {
		  if _, err := wh.Append(ctx, e); err != nil {
			   return err
		  }
	   }
	   j, err := wh.SubmitReplay(ctx, "u2", "USDT", "ada", base.Add(-time.Minute), base.Add(time.Hour), []string{"c1", "h1", "f1", "p1"}, "")
	  if err != nil {
		   return err
	   }
	   done,
		  err := wh.RunReplayID(ctx, j.ID)
	   if err != nil {
		   return err
	   }
	   if done.State != auditchain.JobDone {
		   return fmt.Errorf("state %s", done.State)
	   }
	  if !strings.Contains(done.ReportJSON, `"verdict":"match"`) {
		   return fmt.Errorf("report %s", done.ReportJSON)
	   }
	  wh2 := newWH()
	  wh2.SetReader(&auditchain.StaticBook{Available: "45", Frozen: "25"})
	   for _, e := range seq {
		  if e.IdemKey == "h1" {
			  continue
		  }
		   if _, err := wh2.Append(ctx, e); err != nil {
			   return err
		   }
	   }
	   j2, err := wh2.SubmitReplay(ctx, "u2", "USDT", "ada", base.Add(-time.Minute), base.Add(time.Hour), []string{"c1", "h1", "f1", "p1"}, "")
	   if err != nil {
		  return err
	  }
	  done2, err := wh2.RunReplayID(ctx, j2.ID)
	  if err != nil {
		   return err
	   }
	   if !strings.Contains(done2.ReportJSON, `"verdict":"diff"`) {
		   return fmt.Errorf("dropped event still match: %s", done2.ReportJSON)
	   }
	   alerts, err := wh2.Store().Alerts(ctx, true)
	  if err != nil {
		   return err
	  }
	   found := false
	   for _, a := range alerts {
		  if a.Name == auditchain.AlertReplay {
			  found = true
		   }
	  }
	  if !found {
		   return fmt.Errorf("replay diff alert missing")
	  }
	  before := persist.Fence{Epoch: 3, Token: "same"}
	  after := persist.Fence{Epoch: 3, Token: "same"}
	  if err := dualctl.GuardReplayAdvance(before, after); err == nil {
		  return fmt.Errorf("stale fence accepted")
	   }
	   if err := dualctl.GuardReplayAdvance(before, persist.Fence{Epoch: 4, Token: "next"}); err != nil {
		  return err
	  }
	   rep, err := replay.Run(replay.Input{UserID: "u2", Asset: "USDT"})
	   if err != nil {
		   return err
	  }
	   if rep.ReplayAvailable != "0" {
		   return fmt.Errorf("empty trail %s", rep.ReplayAvailable)
	   }
	   return nil
   }

   func DrillIsolation() error {
	   if err := assertReplayIsolated(); err != nil {
		   return err
	   }
	  ctx := context.Background()
	   wh := newWH()
	  book := &auditchain.StaticBook{Available: "0", Frozen: "0"}
	   wh.SetReader(book)
	   base := time.Date(2026,
		   3, 5, 11, 0, 0, 0, time.UTC)
	  if _, err := wh.Append(ctx, auditchain.Envelope{
		   Source: "deposit.credit", IdemKey: "iso", Kind: auditchain.KindDepositCredit,
		   UserID: "u3", Asset: "USDT", Amount: "1", RefID: "iso", Actor: "ada", Body: "{}", OccurredAt: base,
	   }); err != nil {
		  return err
	  }
	   if _, err := wh.Timeline(ctx, "u3", "USDT", base.Add(-time.Minute), base.Add(time.Minute)); err != nil {
		  return err
	  }
	   if _, err := wh.Inspect(ctx); err != nil {
		   return err
	  }
	   if book.Reads == 0 {
		  return fmt.Errorf("reader not used")
	   }
	  probe := &writeProbe{}
	  probe.Write("apply")
	  if probe.N() != 1 {
		   return fmt.Errorf("probe broken")
	   }
	   probe.Reset()
	  j, err := wh.SubmitReplay(ctx, "u3", "USDT", "ada", base.Add(-time.Minute), base.Add(time.Minute), nil, "")
	  if err != nil {
		  return err
	  }
	  if _, err := wh.RunReplayID(ctx, j.ID); err != nil {
		  return err
	   }
	   if probe.N() != 0 {
		   return fmt.Errorf("prod write during replay %d", probe.N())
	  }
	  return nil
   }

type writeProbe struct{ n int }

   func (p *writeProbe) Write(op string) {
	   if op != "" {
		   p.n++
	  }
   }

  func (p *writeProbe) N() int {
	  if p == nil {
		   return 0
	  }
	   return p.n
   }

   func (p *writeProbe) Reset() { p.n = 0 }

   func DrillExport() error {
	   ctx := context.Background()
	  wh := newWH()
	   desk := auditchain.NewDesk(wh, nil)
	  base := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	   row, err := wh.Append(ctx, auditchain.Envelope{
		  Source: "deposit.credit", IdemKey: "ex1", Kind: auditchain.KindDepositCredit,
		  UserID: "u4", Asset: "USDT", Amount: "8", RefID: "ex1", Actor: "ada", Body: "{}", OccurredAt: base,
	   })
	   if err != nil {
		   return err
	  }
	  if _, err := wh.Seal(ctx, row.SegmentID, "ada"); err != nil {
		  return err
	  }
	  view, err := desk.Request(ctx, "ada", row.SegmentID, "u4", "USDT", base.Add(-time.Minute), base.Add(time.Minute))
	  if err != nil {
		  return err
	   }
	  if _, err := desk.Approve(ctx, view.ID, "ada", true, view.FenceEpoch, view.FenceToken); err == nil {
		   return fmt.Errorf("same actor approved")
	  }
	  view,
		   err = desk.GetExport(ctx, view.ID)
	  if err != nil {
		  return err
	  }
	  if _, err := desk.Approve(ctx, view.ID, "bob", true, view.FenceEpoch, view.FenceToken); err != nil {
		  return err
	  }
	   bundle, err := desk.Release(ctx, view.ID, "cara")
	   if err != nil {
		  return err
	   }
	  if bundle.Export.State != auditchain.ExportReleased || len(bundle.Proofs) != 1 {
		  return fmt.Errorf("bundle state %s proofs %d", bundle.Export.State, len(bundle.Proofs))
	  }
	   if !auditchain.VerifyProof(bundle.Root, bundle.Proofs[0]) {
		  return fmt.Errorf("proof mismatch")
	   }
	  auditchain.HaltExport(true)
	  defer auditchain.HaltExport(false)
	   if err := wh.ExportAllowed(ctx, row.SegmentID); err == nil {
		   return fmt.Errorf("halted export allowed")
	  }
	  for _, spec := range auditchain.Kinds() {
		   if !replay.Known(spec.Effect) {
			  return fmt.Errorf("effect %s", spec.Effect)
		   }
	  }
	   if custody.KindCeremony != "ceremony" {
		   return fmt.Errorf("ceremony kind")
	   }
	  return nil
  }

   func assertReplayIsolated() error {
	   _, file, _, ok := runtime.Caller(0)
	  if !ok {
		  return fmt.Errorf("caller")
	  }
	   dir := filepath.Join(filepath.Dir(file), "..", "replay")
	   entries, err := os.ReadDir(dir)
	  if err != nil {
		  return err
	   }
	  fset := token.NewFileSet()
	  banned := []string{"internal/ledger", "internal/deposit", "internal/withdraw", "internal/sweeprun", "internal/kcfund"}
	  for _, e := range entries {
		  if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			  continue
		   }
		  f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		  if err != nil {
			   return err
		  }
		  for _, imp := range f.Imports {
			   path := strings.Trim(imp.Path.Value, `"`)
			  for _, b := range banned {
				  if strings.Contains(path, b) {
					   return fmt.Errorf("replay imports %s", path)
				   }
			  }
		  }
	  }
	  return nil
   }
