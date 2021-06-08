package fundgraph_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fundgraph"
)

func testSvc(t *testing.T) *fundgraph.Service {
	t.Helper()
	svc := fundgraph.New(fundgraph.NewMemStore(), fundgraph.DefaultConfig())
	svc.SetClock(func() time.Time { return fundgraph.FixtureClock.Add(30 * time.Second) })
	return svc
}

func mustSpine(t *testing.T, p fundgraph.SpineParams,) fundgraph.IngestBatch {
	t.Helper()
	b, err := fundgraph.FixtureSpine(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReverseLineageReachesObserve(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := mustSpine(t, fundgraph.SpineParams{Ref: "flow-1", UserID: "alice", Asset: "USDT", Amount: "12.5", EnvelopeID: "env-7"})
	res, err := svc.Ingest(ctx, batch)
	if err != nil {
		t.Fatal(err)
	}
	if res.InsertedNodes != 5 || res.InsertedEdges != 4 {
		t.Fatalf("ingest %+v", res)
	}
	got, err := svc.Lineage(ctx, fundgraph.LineageQuery{StartRef: "flow-1", Direction: fundgraph.DirReverse})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Paths) != 1 {
		t.Fatalf("lineage complete=%v gaps=%v paths=%d", got.Complete, got.Gaps, len(got.Paths))
	}
	path := got.Paths[0]
	if len(path.Nodes) != 5 || len(path.Edges) != 4 {
		t.Fatalf("path nodes=%d edges=%d", len(path.Nodes), len(path.Edges))
	}
	if path.Nodes[0].Kind != fundgraph.NodeWithdrawBroadcast {
		t.Fatalf("anchor %s", path.Nodes[0].Kind)
	}
	if path.Nodes[len(path.Nodes)-1].Kind != fundgraph.NodeDepositObserve {
		t.Fatalf("root %s", path.Nodes[len(path.Nodes)-1].Kind)
	}
	want := []fundgraph.EdgeType{fundgraph.EdgeFreezeBroadcast, fundgraph.EdgeTransferFreeze, fundgraph.EdgeCreditTransfer, fundgraph.EdgeObserveCredit}
	if len(path.Edges) != len(want) {
		t.Fatalf("edges %d", len(path.Edges))
	}
	for i, typ := range want {
		if path.Edges[i].EdgeType != typ {
			t.Fatalf("edge %d got %s want %s", i, path.Edges[i].EdgeType, typ)
		}
		if path.Edges[i].EnvelopeID != "env-7" && typ != fundgraph.EdgeTransferFreeze && typ != fundgraph.EdgeCreditTransfer && typ != fundgraph.EdgeFreezeBroadcast && typ != fundgraph.EdgeObserveCredit {
			t.Fatalf("envelope %s", path.Edges[i].EnvelopeID)
		}
	}
	for _, e := range path.Edges {
		if e.EnvelopeID != "env-7" {
			t.Fatalf("envelope attr %s on %s", e.EnvelopeID, e.EdgeType)
		}
	}
	ins, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Broken != 0 || ins.Cycles != 0 || ins.MultiParent != 0 || ins.Lag != 0 {
		t.Fatalf("inspect %+v", ins)
	}
}

func TestBrokenMiddleOpensIncidentAndGap(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := fundgraph.WithoutMiddle(mustSpine(t, fundgraph.SpineParams{Ref: "gap-1", UserID: "bob", Amount: "3"}))
	if _, err := svc.Ingest(ctx, batch); err != nil {
		t.Fatal(err)
	}
	ins, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Broken == 0 || len(ins.Opened) == 0 {
		t.Fatalf("expected broken incident %+v", ins)
	}
	found := false
	for _, inc := range ins.Opened {
		if inc.Kind == fundgraph.IncidentBroken && inc.Ref == "gap-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("opened %+v", ins.Opened)
	}
	again, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Opened) != 0 {
		t.Fatalf("duplicate incidents %d", len(again.Opened))
	}
	got, err := svc.Lineage(ctx, fundgraph.LineageQuery{StartRef: "gap-1", Direction: fundgraph.DirReverse})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete {
		t.Fatal("broken lineage marked complete")
	}
	if !hasGap(got.Gaps, fundgraph.GapMissingEdge) {
		t.Fatalf("gaps %+v", got.Gaps)
	}
	for _, n := range got.Reached {
		if n.Kind == fundgraph.NodeDepositObserve {
			t.Fatal("observe should not be reached")
		}
	}
	if len(got.Paths) == 0 || len(got.Paths[0].Nodes) == 0 {
		t.Fatal("expected a partial path")
	}
	if got.Paths[0].Nodes[0].Kind != fundgraph.NodeWithdrawBroadcast {
		t.Fatalf("path start %s", got.Paths[0].Nodes[0].Kind)
	}
}

func TestCycleIsBounded(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := fundgraph.FixtureCycle("loop")
	if _, err := svc.Ingest(ctx, batch); err != nil {
		t.Fatal(err)
	}
	ins, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Cycles == 0 {
		t.Fatalf("cycle not opened %+v", ins)
	}
	start := batch.Nodes[0].ID
	done := make(chan struct{})
	var got fundgraph.LineageResult
	var qerr error
	go func() {
		got, qerr = svc.Lineage(ctx, fundgraph.LineageQuery{StartNodeID: start, Direction: fundgraph.DirForward, MaxHops: 100000})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lineage did not return")
	}
	if qerr != nil {
		t.Fatal(qerr)
	}
	if got.HopsUsed > 64 {
		t.Fatalf("hops %d", got.HopsUsed)
	}
	if !got.Clamped {
		t.Fatal("expected clamp")
	}
	if !hasGap(got.Gaps, fundgraph.GapCycleCut) {
		t.Fatalf("gaps %+v", got.Gaps)
	}
	if got.Complete {
		t.Fatal("cycle path marked complete")
	}
}

func TestIdempotentIngestAndConflict(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := mustSpine(t, fundgraph.SpineParams{Ref: "idem-1", Amount: "1.0"})
	first, err := svc.Ingest(ctx, batch)
	if err != nil {
		t.Fatal(err)
	}
	second,
		err := svc.Ingest(ctx, batch)
	if err != nil {
		t.Fatal(err)
	}
	if second.InsertedEdges != 0 || second.ReplayEdges != first.InsertedEdges {
		t.Fatalf("replay %+v", second)
	}
	m, err := svc.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.Edges != 4 || m.Nodes != 5 {
		t.Fatalf("metrics %+v", m)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.Ingest(ctx, batch)
		}()
	}
	wg.Wait()
	m, err = svc.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.Edges != 4 {
		t.Fatalf("parallel doubled edges %d", m.Edges)
	}
	mut := batch
	mut.Edges = append([]fundgraph.FundEdge{}, batch.Edges...)
	mut.Edges[0].Amount = "99"
	_, err = svc.Ingest(ctx, mut)
	if !errors.Is(err, fundgraph.ErrConflict) {
		t.Fatalf("conflict err %v", err)
	}
	audits, err := svc.Audits(ctx, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	hit := false
	for _, a := range audits {
		if a.Action == fundgraph.ActionConflict && strings.Contains(a.Detail, "amount") {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("audits %+v", audits)
	}
	edges, err := svc.Edges(ctx, fundgraph.EdgeFilter{Ref: "idem-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 4 {
		t.Fatalf("edges %d", len(edges))
	}
	for _, e := range edges {
		if e.Amount == "99" {
			t.Fatal("conflict overwrote amount")
		}
	}
	m, err = svc.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.Conflicts < 1 {
		t.Fatalf("conflicts %d", m.Conflicts)
	}
}

func TestReplayOmitsOccurredAt(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := mustSpine(t, fundgraph.SpineParams{Ref: "omit-at", Amount: "2"})
	if _, err := svc.Ingest(ctx, batch); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.Node(ctx, batch.Nodes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range batch.Nodes {
		batch.Nodes[i].OccurredAt = time.Time{}
	}
	for i := range batch.Edges {
		batch.Edges[i].OccurredAt = time.Time{}
	}
	res, err := svc.Ingest(ctx, batch)
	if err != nil {
		t.Fatalf("omitted time conflicted: %v", err)
	}
	if res.InsertedEdges != 0 || res.ReplayEdges == 0 {
		t.Fatalf("replay %+v", res)
	}
	again, err := svc.Node(ctx, stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !again.OccurredAt.Equal(stored.OccurredAt) {
		t.Fatalf("time rewritten %s -> %s", stored.OccurredAt, again.OccurredAt)
	}
}

func TestEdgeIDCollisionFailsClosed(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	first := mustSpine(t, fundgraph.SpineParams{Ref: "id-a", Amount: "1"})
	if _, err := svc.Ingest(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := mustSpine(t, fundgraph.SpineParams{Ref: "id-b", Amount: "3"})
	second.Edges[0].ID = first.Edges[0].ID
	_, err := svc.Ingest(ctx, second)
	if !errors.Is(err, fundgraph.ErrConflict) {
		t.Fatalf("id collision %v", err)
	}
	edges, err := svc.Edges(ctx, fundgraph.EdgeFilter{Ref: "id-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 4 {
		t.Fatalf("edges %d", len(edges))
	}
	for _, e := range edges {
		if e.ID == first.Edges[0].ID && e.Amount != first.Edges[0].Amount {
			t.Fatalf("id overwritten %+v", e)
		}
	}
	if _, err := svc.Node(ctx, second.Nodes[0].ID); !errors.Is(err, fundgraph.ErrNotFound) {
		t.Fatalf("rejected batch still landed: %v", err)
	}
}

func TestWritePathDoesNotUseExternalAppend(t *testing.T) {
	if fundgraph.WritePath != "fund_graph_store" {
		t.Fatal(fundgraph.WritePath)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"auditchain",
		"internal/deposit\"",
		"internal/withdraw\"",
		"internal/ledger\"",
		"internal/addrpool\"",
		"internal/hotinv\"",
		"internal/approvestore\"",
		"internal/sweeppol",
		"internal/sweeprun",
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		for _, bad := range forbidden {
			if strings.Contains(text, bad) {
				t.Fatalf("%s contains %s", name, bad)
			}
		}
	}
}

func hasGap(gaps []fundgraph.GapMarker, code string) bool {
	for _, g := range gaps {
		if g.Code == code {
			return true
		}
	}
	return false
}
