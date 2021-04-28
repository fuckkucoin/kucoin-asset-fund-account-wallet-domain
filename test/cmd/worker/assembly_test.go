package main_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/workerbnd"
)

  func stubBind(rt *app.Runtime) {
	   kcfund.BindFundsRecon(rt)
  }

  func TestFT01WorkerAssemblyCallsBindFundsRecon(t *testing.T) {
	  rt := &app.Runtime{Cfg: &config.Config{}}
	   called := false
	   asm, err := workerbnd.Assemble(rt, workerbnd.Options{
		   Bind: func(r *app.Runtime) {
			  called = true
			   stubBind(r)
		   },
		  SQLBound: true,
		   Segments: workerbnd.ProdSegments(),
	  })
	  if err != nil {
		  t.Fatal(err)
	   }
	  if !called || !asm.Manifest.BindFundsRecon {
		   t.Fatalf("BindFundsRecon not recorded: called=%v manifest=%+v", called, asm.Manifest)
	   }
	   if rt.FundsRecon == nil {
		  t.Fatal("FundsRecon hook missing")
	  }
   }

  func TestFT02BindDepsMissingAssemblyFails(t *testing.T) {
	   rt := &app.Runtime{Cfg: &config.Config{}}
	  _, err := workerbnd.Assemble(rt, workerbnd.Options{
		   Bind: nil,
		  SQLBound: true,
		   Segments: workerbnd.ProdSegments(),
	   })
	  if !errors.Is(err, workerbnd.ErrBindRecon) {
		   t.Fatalf("want ErrBindRecon got %v", err)
	  }
	  _, err = workerbnd.Assemble(nil, workerbnd.Options{
		   Bind: stubBind,
		  SQLBound: true,
		  Segments: workerbnd.ProdSegments(),
	   })
	  if !errors.Is(err, workerbnd.ErrRuntime) {
		  t.Fatalf("want ErrRuntime got %v", err)
	   }
	  rt2 := &app.Runtime{Cfg: &config.Config{}}
	  _, err = workerbnd.Assemble(rt2, workerbnd.Options{
		   Bind:     func(*app.Runtime) {},
		  SQLBound: true,
		  Segments: workerbnd.ProdSegments(),
	  })
	  if !errors.Is(err, workerbnd.ErrBindRecon) {
		   t.Fatalf("want bind no-op fail %v", err)
	   }
   }

   func TestFT10ReconTickFakeClockInterval(t *testing.T) {
	   clk := workerbnd.NewFakeClock(time.Unix(1_700_000_000, 0).UTC())
	  n := 0
	  loop := workerbnd.NewReconLoop(time.Second,
		  clk, func(context.Context) { n++ })
	   ctx := context.Background()
	   if !loop.Tick(ctx) || n != 1 {
		   t.Fatalf("first tick n=%d", n)
	   }
	   if loop.Tick(ctx) {
		  t.Fatal("same instant should not fire")
	   }
	   clk.Advance(500 * time.Millisecond)
	  if loop.Tick(ctx) {
		  t.Fatal("half interval should not fire")
	   }
	   clk.Advance(500 * time.Millisecond)
	   if !loop.Tick(ctx) || n != 2 {
		   t.Fatalf("full interval n=%d count=%d", n, loop.Count())
	   }
	   clk.Advance(2 * time.Second)
	   if !loop.Tick(ctx) || n != 3 || loop.Count() != 3 {
		  t.Fatalf("want 3 ticks got n=%d count=%d", n, loop.Count())
	   }
  }

   func TestFT11ProdConfigWithoutBindSQLFails(t *testing.T) {
	   rt := &app.Runtime{Cfg: &config.Config{Profile: "production"}}
	  _, err := workerbnd.Assemble(rt, workerbnd.Options{
		  Bind: stubBind,
		   SQLBound: false,
		  RequireSQL: true,
		   Profile:  "production",
		  Segments: workerbnd.ProdSegments(),
	  })
	   if !errors.Is(err, workerbnd.ErrBindSQL) {
		  t.Fatalf("want ErrBindSQL got %v", err)
	  }
  }

  func TestFT12MissingOccupancyAttachFails(t *testing.T) {
	   rt := &app.Runtime{Cfg: &config.Config{}}
	   _, err := workerbnd.Assemble(rt, workerbnd.Options{
		   Bind:     stubBind,
		   SQLBound: true,
		  Segments: workerbnd.SegmentSet{Observe: true, Credit: true, Recon: true},
	  })
	  if !errors.Is(err, workerbnd.ErrSegment) {
		  t.Fatalf("want ErrSegment got %v", err)
	  }
  }

   func TestFT13AllFourSegmentsAssemblyOKAndFakeFlow(t *testing.T) {
	  rt := &app.Runtime{Cfg: &config.Config{}}
	  asm, err := workerbnd.Assemble(rt, workerbnd.Options{
		   Bind: stubBind,
		   SQLBound: true,
		  Segments: workerbnd.ProdSegments(),
	   })
	   if err != nil {
		  t.Fatal(err)
	   }
	   if !asm.Manifest.Observe || !asm.Manifest.Occupancy || !asm.Manifest.Credit || !asm.Manifest.Recon || !asm.Manifest.BindSQL {
		  t.Fatalf("incomplete manifest %+v", asm.Manifest)
	  }
	   flow := &workerbnd.FakeFlow{}
	  if err := flow.Run(workerbnd.ProdSegments()); err != nil {
		  t.Fatal(err)
	  }
	  want := []string{workerbnd.SegObserve,
		  workerbnd.SegOccupancy, workerbnd.SegCredit, workerbnd.SegRecon}
	  if len(flow.Steps) != 4 {
		  t.Fatalf("steps %v", flow.Steps)
	   }
	   for i, s := range want {
		  if flow.Steps[i] != s {
			  t.Fatalf("step %d want %s got %s", i, s, flow.Steps[i])
		   }
	   }
  }
