package fundgraph_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/events"
	"account-assets.kucoin.net/funding/internal/fundgraph"
)

func TestAdaptersCoverSideFacts(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	spine := mustSpine(t, fundgraph.SpineParams{Ref: "side-1", UserID: "u", Amount: "4"})
	if _, err := svc.Ingest(ctx, spine); err != nil {
		t.Fatal(err)
	}
	parts := []fundgraph.IngestBatch{}
	plan, err := fundgraph.AdaptSweepPlan(fundgraph.SweepPlanFact{PlanID: "plan-1", Asset: "USDT", Amount: "4", Chain: "ETH", FromWallet: "hot", ToWallet: "warm", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := fundgraph.AdaptSweepProof(fundgraph.SweepProofFact{PlanID: "plan-1", ProofID: "proof-1", Asset: "USDT", Amount: "4", CreditRef: "side-1", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	recon, err := fundgraph.AdaptReconTicket(fundgraph.ReconTicketFact{TicketID: "rt-1", Ref: "side-1", UserID: "u", Asset: "USDT", Amount: "4", TicketKind: "diff", ParentKind: fundgraph.NodeDepositObserve, At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	protect, err := fundgraph.AdaptAddrProtect(fundgraph.AddrProtectFact{TicketID: "ap-1", Ref: "side-1", UserID: "u", Chain: "eth", Address: "0xabc", Reason: "dust", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := fundgraph.AdaptApprovalTicket(fundgraph.ApprovalTicketFact{TicketID: "az-1", Ref: "side-1", UserID: "u", Asset: "USDT", Amount: "4", Level: "l2", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	release, err := fundgraph.AdaptReleaseRef(fundgraph.ReleaseRefFact{ReleaseID: "rel-1", TicketID: "az-1", Ref: "side-1", Amount: "4", Asset: "USDT", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := fundgraph.AdaptHotReserve(fundgraph.HotReserveFact{TicketID: "hr-1", Ref: "side-1", Asset: "USDT", Amount: "4", WalletID: "hot-1", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	envref, err := fundgraph.AdaptEnvelopeRef(fundgraph.EnvelopeRefFact{Ref: "side-1", EnvelopeID: "env-side", ParentKind: fundgraph.NodeWithdrawBroadcast, At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	roll, err := fundgraph.AdaptDepositRollback(fundgraph.DepositRollbackFact{Ref: "side-1", UserID: "u", Asset: "USDT", Amount: "4", RollbackID: "rb-1", Reason: "early", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	parts = append(parts, plan, proof, recon, protect, approval, release, reserve, envref, roll)
	for _, p := range parts {
		if _, err := svc.Ingest(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	edges, err := svc.Edges(ctx, fundgraph.EdgeFilter{Ref: "side-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[fundgraph.EdgeType]int{}
	for _, e := range edges {
		got[e.EdgeType]++
	}
	for _, typ := range []fundgraph.EdgeType{fundgraph.EdgeObserveCredit, fundgraph.EdgeCreditTransfer, fundgraph.EdgeTransferFreeze, fundgraph.EdgeFreezeBroadcast, fundgraph.EdgeObserveRecon, fundgraph.EdgeAddrpoolProtect, fundgraph.EdgeFreezeApproval, fundgraph.EdgeApprovalRelease, fundgraph.EdgeHotReserveHold, fundgraph.EdgeEnvelopeRef, fundgraph.EdgeDepositRollback,
	} {
		if got[typ] == 0 {
			t.Fatalf("missing %s in %v", typ, got)
		}
	}
	proofEdges, err := svc.Edges(ctx, fundgraph.EdgeFilter{Ref: "plan-1"})
	if err != nil {
		t.Fatal(err)
	}
	sawProof := false
	for _, e := range proofEdges {
		if e.EdgeType == fundgraph.EdgeSweepPlanProof {
			sawProof = true
		}
	}
	if !sawProof {
		t.Fatal("sweep proof edge missing")
	}
	res, err := svc.Lineage(ctx, fundgraph.LineageQuery{StartRef: "side-1", Direction: fundgraph.DirReverse, MaxHops: 8})
	if err != nil {
		t.Fatal(err)
	}
	if res.Paths[0].Nodes[len(res.Paths[0].Nodes)-1].Kind != fundgraph.NodeDepositObserve {
		t.Fatalf("root %s", res.Paths[0].Nodes[len(res.Paths[0].Nodes)-1].Kind)
	}
}

func TestBusEnvelopeRoundTrip(t *testing.T) {
	payload, _ := json.Marshal(fundgraph.DepositObserveFact{
		Ref:	"bus-1", UserID: "u", Asset: "usdt", Amount: "8", Chain: "eth", TxHash: "0x1", ObservedAt: fundgraph.FixtureClock,
	})
	env := fundgraph.BusEnvelope{Kind: events.KindDepositObserved, Payload: payload, EnvelopeID: "env-bus", At: fundgraph.FixtureClock}
	batch, err := fundgraph.AdaptBus(env)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Nodes[0].Asset != "USDT" || batch.Nodes[0].Attrs["envelope_id"] != "env-bus" {
		t.Fatalf("node %+v", batch.Nodes[0])
	}
	if events.TopicForKind(events.KindFundGraphIncident) != events.TopicFundGraphIncident {
		t.Fatal("topic")
	}
	if events.TopicForKind(events.KindTransferHop) != "transfer.hop" {
		t.Fatal("hop topic")
	}
	_, err = fundgraph.AdaptBus(fundgraph.BusEnvelope{Kind: "not-a-kind", Payload: payload})
	if !errors.Is(err, fundgraph.ErrSkip) {
		t.Fatalf("skip %v", err)
	}
	_, err = fundgraph.AdaptDepositObserve(fundgraph.DepositObserveFact{Ref: "x", Amount: "-1", UserID: "u", Asset: "USDT", Chain: "ETH", TxHash: "0x"})
	if !errors.Is(err, fundgraph.ErrInvalid) {
		t.Fatalf("neg %v", err)
	}
}

func TestSecondHopAndConfirm(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	spine := mustSpine(t, fundgraph.SpineParams{Ref: "hop2", UserID: "u", Amount: "1"})
	if _, err := svc.Ingest(ctx, spine); err != nil {
		t.Fatal(err)
	}
	hop, err := fundgraph.AdaptTransferHop(fundgraph.TransferHopFact{Ref: "hop2", Hop: 1, UserID: "u", Asset: "USDT", Amount: "1", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	if hop.Edges[0].EdgeType != fundgraph.EdgeTransferHop {
		t.Fatalf("type %s", hop.Edges[0].EdgeType)
	}
	conf, err := fundgraph.AdaptWithdrawConfirm(fundgraph.WithdrawConfirmFact{Ref: "hop2", UserID: "u", Asset: "USDT", Amount: "1", Chain: "ETH", TxHash: "0xspine", At: fundgraph.FixtureClock})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ingest(ctx, fundgraph.MergeBatches(hop, conf)); err != nil {
		t.Fatal(err)
	}
	ins, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Broken != 0 || ins.Cycles != 0 {
		t.Fatalf("inspect %+v", ins)
	}
	res, err := svc.Lineage(ctx, fundgraph.LineageQuery{StartRef: "hop2", Direction: fundgraph.DirReverse, TargetKind: fundgraph.NodeDepositObserve})
	if err != nil {
		t.Fatal(err)
	}
	if res.Paths[0].Nodes[0].Kind != fundgraph.NodeWithdrawConfirm {
		t.Fatalf("start %s", res.Paths[0].Nodes[0].Kind)
	}
	if !strings.Contains(string(res.Paths[0].Nodes[len(res.Paths[0].Nodes)-1].Kind), "observe") {
		t.Fatal(res.Paths[0].Nodes[len(res.Paths[0].Nodes)-1].Kind)
	}
}

func TestJSONLReplay(t *testing.T) {
	raw := `{"kind":"deposit_observed","envelope_id":"e1","payload":{"ref":"j1","user_id":"u","asset":"USDT","amount":"1","chain":"ETH","tx_hash":"0xj","observed_at":"2026-03-21T08:00:00Z"}}
{"kind":"unknown-kind","payload":{"ref":"j1"}}
{"kind":"deposit_credited","payload":{"ref":"j1","user_id":"u","asset":"USDT","amount":"1","credit_ref":"c1"}}
`
	envs, err := fundgraph.LoadEnvelopesJSONL(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 3 {
		t.Fatalf("envs %d", len(envs))
	}
	svc := testSvc(t)
	w := &fundgraph.Worker{Svc: svc, Sources: []fundgraph.Source{fundgraph.NewEnvelopeSource("replay", "bus", envs)}}
	rep, err := w.Once(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conflicts != 0 {
		t.Fatalf("report %+v", rep)
	}
	nodes, err := svc.Nodes(context.Background(), fundgraph.NodeFilter{Ref: "j1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes %d", len(nodes))
	}
	cur, ok, err := svc.GetCursor(context.Background(), "replay", "bus")
	if err != nil || !ok || cur.Token != "3" {
		t.Fatalf("cursor %+v ok=%v err=%v", cur, ok, err)
	}
	rep2, err := w.Once(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Ingested != 0 || rep2.Conflicts != 0 {
		t.Fatalf("second %+v", rep2)
	}
}
