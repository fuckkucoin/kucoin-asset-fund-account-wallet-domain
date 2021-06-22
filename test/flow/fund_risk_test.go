package flow

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/stmt"
	"account-assets.kucoin.net/funding/internal/travelvasp"
)

  func TestFundingRiskEdgesLocalFakes(t *testing.T) {
	  allow := destallow.New()
	   addr := "0x3f8e2a1c9b7d4e60518293a4b5c6d7e8f9012345"
	  if err := allow.Require("user-10001", "ETH", addr); !errors.Is(err, destallow.ErrMissing) {
		   t.Fatalf("missing %v", err)
	  }
	   if _, err := allow.Add("user-10001", "ETH", addr, "book"); err != nil {
		  t.Fatal(err)
	  }
	  if err := allow.Require("user-10001", "ETH", addr); err != nil {
		  t.Fatal(err)
	   }
	   book := recon.New()
	  tk, err := book.Open("ops-recon", "user-10001", "wallet", "USDT", "hot vs chain drift")
	  if err != nil {
		   t.Fatal(err)
	   }
	  as, err := book.Assign(tk.ID, "ops-recon-2", "ops-desk", tk.Fence.Epoch, tk.Fence.Token)
	  if err != nil || as.Assignee != "ops-desk" {
		  t.Fatalf("assign %v %+v", err, as)
	  }
	   if _, err := book.Resolve(as.ID, "ops-recon-3", as.Fence.Epoch, as.Fence.Token); err != nil {
		  t.Fatal(err)
	  }
	   if book.OpenCount("user-10001") != 0 {
		  t.Fatal("open")
	   }
	   tr := travelvasp.New()
	  env, err := travelvasp.Shape("USDT", "2",
		   travelvasp.Party{Name: "origin", Account: "user-10001", VASP: "KC01"},
		   travelvasp.Party{Name: "peer", Account: "ext-alice", VASP: "PEER"},
	  )
	   if err != nil {
		  t.Fatal(err)
	   }
	  if _, err := tr.OpenOut("PEER", "ops-tr", "flow-tr-1", env); err != nil {
		  t.Fatal(err)
	  }
	   if err := tr.VerifyChain(); err != nil {
		   t.Fatal(err)
	   }
	   if n := len(tr.Gaps()); n != 0 {
		   t.Fatalf("gaps %d", n)
	  }
	   binder := kyt.NewBinder()
	   if _, err := binder.Bind("flow-m1", kyt.Allow, "clean"); err != nil {
		   t.Fatal(err)
	  }
	   if err := binder.MustAllow("flow-m1"); err != nil {
		   t.Fatal(err)
	   }
	   if err := binder.VerifyChain(); err != nil {
		  t.Fatal(err)
	   }
	   views := []ledger.BookView{{Book: "wallet", Asset: "USDT", Available: "1", Frozen: "0"}}
	   shot := recon.Shot{UserID: "user-10001", Positions: []recon.ShotPos{{Book: "funding", Asset: "USDT", Available: "2", Frozen: "0", Total: "2"}}}
	  rep, err := recon.CompareSources("user-10001", views, shot, stmt.Report{})
	  if err != nil || rep.OK || len(recon.Exceptions(rep)) == 0 {
		  t.Fatalf("missing leg %v %+v", err, rep)
	  }
   }
