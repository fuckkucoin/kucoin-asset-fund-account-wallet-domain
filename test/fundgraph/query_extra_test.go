package fundgraph_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fundgraph"
)

func TestHopLimitAndPagination(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := fundgraph.FixtureHopChain(10, "long")
	if _, err := svc.Ingest(ctx, batch); err != nil {
		t.Fatal(err)
	}
	start := batch.Nodes[0].ID
	limited, err := svc.Lineage(ctx, fundgraph.LineageQuery{StartNodeID: start, Direction: fundgraph.DirForward, MaxHops: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Truncated || limited.HopsUsed > 2 {
		t.Fatalf("limit hops=%d trunc=%v reached=%d", limited.HopsUsed, limited.Truncated, len(limited.Reached))
	}
	if !hasGap(limited.Gaps, fundgraph.GapHopLimit) {
		t.Fatalf("gaps %+v", limited.Gaps)
	}
	seen := map[string]bool{}
	var token string
	pages := 0
	for {
		pages++
		if pages > 20 {
			t.Fatal("page loop")
		}
		res, err := svc.Lineage(ctx, fundgraph.LineageQuery{
			StartNodeID:	start,
			Direction:	fundgraph.DirForward,
			MaxHops:	16,
			Limit:		3,
			PageToken:	token,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Reached) == 0 && token != "" {
			break
		}
		for _, n := range res.Reached {
			if seen[n.ID] {
				t.Fatalf("dup %s", n.ID)
			}
			seen[n.ID] = true
		}
		if res.NextPageToken == "" {
			break
		}
		if res.NextPageToken == token {
			t.Fatal("token stuck")
		}
		token = res.NextPageToken
	}
	if len(seen) != 10 {
		t.Fatalf("paged %d", len(seen))
	}
}

func TestMissingAnchorDoesNotPanic(t *testing.T) {
	svc := testSvc(t)
	res, err := svc.Lineage(context.Background(), fundgraph.LineageQuery{StartNodeID: "missing", Direction: fundgraph.DirReverse})
	if err != nil {
		t.Fatal(err)
	}
	if !hasGap(res.Gaps, fundgraph.GapStartMissing) {
		t.Fatalf("gaps %+v", res.Gaps)
	}
	if res.Complete {
		t.Fatal("complete")
	}
	res, err = svc.Lineage(context.Background(), fundgraph.LineageQuery{Direction: fundgraph.DirForward})
	if err != nil {
		t.Fatal(err)
	}
	if !hasGap(res.Gaps, fundgraph.GapNoAnchor) {
		t.Fatalf("gaps %+v", res.Gaps)
	}
}

func TestShortestResponsibility(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	if _, err := svc.Ingest(ctx, mustSpine(t, fundgraph.SpineParams{Ref: "short-1", UserID: "sam"})); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Lineage(ctx, fundgraph.LineageQuery{
		StartRef:	"short-1",
		Direction:	fundgraph.DirShortest,
		TargetKind:	fundgraph.NodeDepositObserve,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Complete || len(res.Paths) != 1 {
		t.Fatalf("short %+v gaps=%v", res.Complete, res.Gaps)
	}
	path := res.Paths[0]
	if path.Nodes[0].Kind != fundgraph.NodeWithdrawBroadcast || path.Nodes[len(path.Nodes)-1].Kind != fundgraph.NodeDepositObserve {
		t.Fatalf("path %s -> %s", path.Nodes[0].Kind, path.Nodes[len(path.Nodes)-1].Kind)
	}
	if path.Hops != 4 {
		t.Fatalf("hops %d", path.Hops)
	}
}

func TestUserAssetWindow(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	if _, err := svc.Ingest(ctx, mustSpine(t, fundgraph.SpineParams{Ref: "win-1", UserID: "win", Asset: "btc", Amount: "2"})); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Lineage(ctx, fundgraph.LineageQuery{
		UserID:		"win",
		Asset:		"btc",
		From:		fundgraph.FixtureClock.Add(-time.Minute),
		To:		fundgraph.FixtureClock.Add(time.Minute),
		Direction:	fundgraph.DirReverse,
	})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[fundgraph.NodeKind]bool{}
	var token string
	for {
		page, err := svc.Lineage(ctx, fundgraph.LineageQuery{
			UserID:	"win", Asset: "BTC", Direction: fundgraph.DirForward, Limit: 2, PageToken: token,
			From:	fundgraph.FixtureClock.Add(-time.Minute), To: fundgraph.FixtureClock.Add(time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range page.Reached {
			kinds[n.Kind] = true
		}
		if page.NextPageToken == "" {
			break
		}
		token = page.NextPageToken
	}
	if !kinds[fundgraph.NodeDepositObserve] || !kinds[fundgraph.NodeWithdrawBroadcast] {
		t.Fatalf("kinds %v reverseComplete=%v", kinds, res.Complete)
	}
}

func TestDanglingQueryAndIncident(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := fundgraph.FixtureDangling("d1")
	if _, err := svc.Ingest(ctx, batch); err != nil {
		t.Fatal(err)
	}
	ins, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Dangling == 0 {
		t.Fatalf("dangling %+v", ins)
	}
	res, err := svc.Lineage(ctx, fundgraph.LineageQuery{StartNodeID: batch.Nodes[0].ID, Direction: fundgraph.DirForward})
	if err != nil {
		t.Fatal(err)
	}
	if !hasGap(res.Gaps, fundgraph.GapDangling) {
		t.Fatalf("gaps %+v", res.Gaps)
	}
}

func TestMultiParentAndLag(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	if _, err := svc.Ingest(ctx, fundgraph.FixtureMultiParent("mp")); err != nil {
		t.Fatal(err)
	}
	if err := svc.PutCursor(ctx, fundgraph.Cursor{
		Source:		"deposit",
		Stream:		"observe",
		Token:		"7",
		UpdatedAt:	fundgraph.FixtureClock.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	ins, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ins.MultiParent == 0 || ins.Lag == 0 {
		t.Fatalf("inspect %+v opened=%+v", ins, ins.Opened)
	}
	list, err := svc.Incidents(ctx, fundgraph.IncidentFilter{Kind: fundgraph.IncidentMultiParent})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("no multi parent ticket")
	}
	if err := svc.AckIncident(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, inc := range again.Opened {
		if inc.ID == list[0].ID {
			t.Fatal("acked incident reopened")
		}
	}
	if err := svc.CloseIncident(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	re, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reopened := false
	for _, inc := range re.Opened {
		if inc.Kind == fundgraph.IncidentMultiParent {
			reopened = true
		}
	}
	if !reopened {
		t.Fatal("closed incident should reopen while the graph is still wrong")
	}
}

func TestPageTokenMismatch(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	if _, err := svc.Ingest(ctx, fundgraph.FixtureHopChain(4, "pg")); err != nil {
		t.Fatal(err)
	}
	start := fundgraph.NodeID(fundgraph.NodeTransferHop, "pg#0")
	res, err := svc.Lineage(ctx, fundgraph.LineageQuery{StartNodeID: start, Direction: fundgraph.DirForward, Limit: 1, MaxHops: 8})
	if err != nil {
		t.Fatal(err)
	}
	if res.NextPageToken == "" {
		t.Fatal("expected token")
	}
	_,
		err = svc.Lineage(ctx, fundgraph.LineageQuery{StartNodeID: start, Direction: fundgraph.DirReverse, Limit: 1, MaxHops: 8, PageToken: res.NextPageToken})
	if !errors.Is(err, fundgraph.ErrPage) {
		t.Fatalf("err %v", err)
	}
}
