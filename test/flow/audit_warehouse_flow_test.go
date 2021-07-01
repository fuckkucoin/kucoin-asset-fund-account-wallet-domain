package flow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain"
	"account-assets.kucoin.net/funding/internal/auditchain/fixture"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func TestAuditWarehouseDrills(t *testing.T) {
	  ingest.SetMode(ingest.ModeOn)
	  t.Cleanup(func() { ingest.SetMode(ingest.ModeOff) })
	  if err := fixture.RunAll(); err != nil {
		  t.Fatal(err)
	  }
   }

  func TestAuditWaterLexicographic(t *testing.T) {
	   base := time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC)
	   later := base.Add(time.Nanosecond)
	   if !(auditchain.Water(base, "k") < auditchain.Water(later, "k")) {
		  t.Fatalf("nano water regressed %s vs %s", auditchain.Water(base, "k"), auditchain.Water(later, "k"))
	  }
   }

  func TestAuditIngestCursorMonotonic(t *testing.T) {
	  ingest.SetMode(ingest.ModeOn)
	   t.Cleanup(func() { ingest.SetMode(ingest.ModeOff) })
	  ctx := context.Background()
	  wh := auditchain.NewWarehouse(auditchain.NewMem(), auditchain.DefaultConfig())
	  ingest.Install(wh)
	  base := time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC)
	   first := ingest.CreditFact("u", "USDT", "1", "c-a", "ada", base)
	  second := ingest.CreditFact("u", "USDT", "1", "c-b", "ada", base.Add(time.Nanosecond))
	  if err := ingest.Push(ctx, first); err != nil {
		   t.Fatal(err)
	  }
	  if err := ingest.Push(ctx, second); err != nil {
		   t.Fatal(err)
	   }
	   late := ingest.CreditFact("u", "USDT", "1", "c-old", "ada", base)
	  if err := ingest.Push(ctx, late); !errors.Is(err, auditchain.ErrCursor) {
		  t.Fatalf("late cursor %v", err)
	   }
	  n, err := wh.EntryCount(ctx)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if n != 3 {
		  t.Fatalf("late row dropped %d", n)
	   }
  }

   func TestAuditFreezeFactMovesOnce(t *testing.T) {
	   at := time.Date(2026, 4, 2, 1, 0, 0, 0, time.UTC)
	  held := ingest.FreezeFact("u", "USDT", "40", "wd-1", "", "held", "ada", at)
	   if held.Kind != auditchain.KindWdFreeze || held.Amount != "40" {
		  t.Fatalf("held %+v", held)
	   }
	   screen := ingest.FreezeFact("u", "USDT", "40", "wd-1", "held", "screening", "ada", at)
	  if screen.Kind != auditchain.KindWdFreeze || screen.Amount != "0" {
		  t.Fatalf("screen %+v", screen)
	  }
	   cancel := ingest.FreezeFact("u", "USDT", "40", "wd-1", "screening", "cancelled", "ada", at)
	  if cancel.Kind != auditchain.KindWdUnfreeze || cancel.Amount != "40" {
		  t.Fatalf("cancel %+v", cancel)
	   }
	   after := ingest.FreezeFact("u", "USDT", "40", "wd-1", "broadcast", "failed", "ada", at)
	  if after.Kind != auditchain.KindWdBroadcast || after.Amount != "0" {
		   t.Fatalf("post-broadcast fail %+v", after)
	   }
  }

   func TestAuditCheckpointSameInstant(t *testing.T) {
	  ctx := context.Background()
	   frozen := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	  wh := auditchain.NewWarehouse(auditchain.NewMem(), auditchain.Config{
		   SegCap: 256, HotDays: 3, LagLimit: 2 * time.Minute,
		  Clock: func() time.Time { return frozen },
	   })
	  row, err := wh.Append(ctx, auditchain.Envelope{
		  Source: "deposit.credit", IdemKey: "same", Kind: auditchain.KindDepositCredit,
		  UserID: "u", Asset: "USDT", Amount: "2", RefID: "same", Actor: "ada",
		   Body: "{}", OccurredAt: frozen,
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	   if _, err := wh.Tip(ctx, row.SegmentID, "ada"); err != nil {
		   t.Fatal(err)
	  }
	   if _, err := wh.Seal(ctx, row.SegmentID, "ada"); err != nil {
		   t.Fatal(err)
	  }
	  rep, err := wh.Inspect(ctx)
	  if err != nil {
		   t.Fatal(err)
	  }
	   if rep.Failed {
		   t.Fatalf("same-instant chain frozen: %+v", rep)
	  }
	  if err := wh.ExportAllowed(ctx, row.SegmentID); err != nil {
		  t.Fatal(err)
	   }
   }

   func TestAuditIdleCursorNotLag(t *testing.T) {
	  ctx := context.Background()
	   wh := auditchain.NewWarehouse(auditchain.NewMem(), auditchain.DefaultConfig())
	   at := time.Now().UTC()
	  if _, err := wh.Append(ctx, auditchain.Envelope{
		   Source: "deposit.credit", IdemKey: "idle", Kind: auditchain.KindDepositCredit,
		   UserID: "u", Asset: "USDT", Amount: "1", RefID: "idle", Actor: "ada",
		   Body: "{}", OccurredAt: at,
	   }); err != nil {
		  t.Fatal(err)
	  }
	  if _, err := wh.AdvanceCursor(ctx, "deposit.credit", auditchain.Water(at, "idle"), "idle", at); err != nil {
		   t.Fatal(err)
	   }
	   rep, err := wh.Inspect(ctx)
	  if err != nil {
		  t.Fatal(err)
	  }
	  for _, name := range rep.Alerts {
		  if name == auditchain.AlertLag {
			   t.Fatal("idle cursor raised lag")
		   }
	  }
   }

   func TestAuditSQLRefusesRewrite(t *testing.T) {
	  st := auditchain.NewSQL(nil)
	   if err := st.CorruptBody(context.Background(), "seg", 1, "x"); err == nil || !errors.Is(err, auditchain.ErrFrozen) {
		   t.Fatalf("corrupt %v", err)
	   }
	  if err := st.RewriteRoot(context.Background(), "seg", "root"); err == nil || !errors.Is(err, auditchain.ErrFrozen) {
		  t.Fatalf("root %v", err)
	  }
   }

   func TestAuditMigrationVersionUnique(t *testing.T) {
	  dir := config.FindRepoFile("migrations")
	  files, err := persist.ListMigrationFiles(dir)
	  if err != nil {
		   t.Fatal(err)
	  }
	  var saw43, saw56 bool
	   for _, f := range files {
		   switch f.Version {
		   case 43:
			   saw43 = true
			  if !strings.Contains(f.Name, "recon_deposit_diff") {
				  t.Fatalf("0043 is %s", f.Name)
			   }
		   case 56:
			  saw56 = true
			   if !strings.Contains(f.Name, "audit_warehouse") {
				   t.Fatalf("0056 is %s", f.Name)
			  }
		  }
	  }
	  if !saw43 || !saw56 {
		   t.Fatalf("versions 43=%v 56=%v", saw43, saw56)
	   }
   }
