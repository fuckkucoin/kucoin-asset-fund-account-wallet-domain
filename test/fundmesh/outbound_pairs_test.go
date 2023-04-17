package fundmesh_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"account-assets.kucoin.net/funding/internal/fundmesh"
)

type pairRow struct {
	Name string `json:"name"`
	Key    string `json:"key"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain string `json:"chain"`
	Dest  string `json:"dest"`
}

  func loadPairs(t *testing.T) []pairRow {
	   t.Helper()
	  p := filepath.Join("testdata", "fundmesh", "outbound_pairs.json")
	  if _, err := os.Stat(p); err != nil {
		  p = filepath.Join("..", "..", "testdata", "fundmesh", p[len("testdata/"):])
		  p = filepath.Join("..", "..", "testdata", "fundmesh", "outbound_pairs.json")
	  }
	   b, err := os.ReadFile(p)
	   if err != nil {
		   t.Fatal(err)
	  }
	   var wrap struct {
		   Pairs []pairRow `json:"pairs"`
	  }
	   if err := json.Unmarshal(b, &wrap); err != nil {
		  t.Fatal(err)
	  }
	  if len(wrap.Pairs) < 10 {
		  t.Fatalf("pairs %d", len(wrap.Pairs))
	   }
	   return wrap.Pairs
   }

  func TestBO26PairReplayEachOnce(t *testing.T) {
	   pairs := loadPairs(t)
	   eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	  for i, p := range pairs {
		  req := outboundReq(p.Key, p.Asset, p.Amount, p.Chain, p.Dest)
		  first,
			   err := eng.SubmitWithdraw(ctx, req)
		  if err != nil {
			  t.Fatalf("%s %v", p.Name, err)
		  }
		  second, err := eng.SubmitWithdraw(ctx, req)
		   if err != nil {
			  t.Fatalf("%s replay %v", p.Name, err)
		  }
		   if first.Code != fundmesh.CodeOutboundOK || second.Code != fundmesh.CodeOutboundReplay {
			  t.Fatalf("%s codes %s %s", p.Name, first.Code, second.Code)
		   }
		  if first.WithdrawID != second.WithdrawID || first.Phase != second.Phase {
			  t.Fatalf("%s terminal", p.Name)
		   }
		   if debitCalls(eng) != i+1 {
			  t.Fatalf("%s debit %d want %d", p.Name, debitCalls(eng), i+1)
		   }
		  if n, _ := eng.Journals().CountOK(ctx, first.IdempotencyKey); n != 1 {
			   t.Fatalf("%s journal %d", p.Name, n)
		   }
	  }
   }

   func TestBO27PairConflictDoesNotStealDebit(t *testing.T) {
	   pairs := loadPairs(t)
	   eng := fundmesh.NewMemEngine()
	  ctx := context.Background()
	  p := pairs[0]
	  if _, err := eng.SubmitWithdraw(ctx, outboundReq(p.Key, p.Asset, p.Amount, p.Chain, p.Dest)); err != nil {
		  t.Fatal(err)
	   }
	   _, err := eng.SubmitWithdraw(ctx, outboundReq(p.Key, p.Asset, "999", p.Chain, p.Dest))
	  if err == nil {
		   t.Fatal("want conflict")
	  }
	   if debitCalls(eng) != 1 {
		  t.Fatalf("debit %d", debitCalls(eng))
	  }
  }

  func TestBO28MemRepoConflictDifferentWD(t *testing.T) {
	   repo := fundmesh.NewMemOutboundRepo()
	   rec := &fundmesh.OutboundRecord{
		   RecordID: "or-1", WithdrawID: "wd-1", IdempotencyKey: "fmout.v1.c.client-key-alpha01",
		   Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		   State: fundmesh.OutboundAccepted,
	  }
	   if err := repo.Put(context.Background(), rec); err != nil {
		   t.Fatal(err)
	   }
	  other := *rec
	   other.RecordID = "or-2"
	   other.WithdrawID = "wd-2"
	   other.Fingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	  if err := repo.Put(context.Background(), &other); err == nil {
		   t.Fatal("want conflict")
	   }
   }

   func TestBO29GetOutboundLookup(t *testing.T) {
	   eng := fundmesh.NewMemEngine()
	   ctx := context.Background()
	   res, err := eng.SubmitWithdraw(ctx, outboundReq("client-key-alpha01", "USDT", "1", "ETH", "0xabc"))
	   if err != nil {
		  t.Fatal(err)
	  }
	   rec, err := eng.GetOutbound(ctx, res.IdempotencyKey, "")
	   if err != nil || rec.WithdrawID != res.WithdrawID {
		   t.Fatalf("%v %+v", err, rec)
	  }
	   rec2, err := eng.GetOutbound(ctx, "", res.WithdrawID)
	  if err != nil || rec2.IdempotencyKey != rec.IdempotencyKey {
		   t.Fatal(err)
	   }
   }

   func TestBO30RejectSecondDebitHelper(t *testing.T) {
	  if err := fundmesh.RejectSecondDebit(nil); err != nil {
		   t.Fatal(err)
	  }
	  rec := &fundmesh.OutboundRecord{DebitCount: 1, State: fundmesh.OutboundAccepted, Phase: fundmesh.WithdrawFrozen}
	   if err := fundmesh.RejectSecondDebit(rec); err == nil {
		  t.Fatal("want forbid")
	  }
   }
