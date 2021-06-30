package fundgraph_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/fundgraph"
)

func TestPruneMarksWithoutDelete(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	old := mustSpine(t, fundgraph.SpineParams{Ref: "old-1", UserID: "u", Amount: "1"})
	for i := range old.Nodes {
		old.Nodes[i].OccurredAt = fundgraph.FixtureClock.Add(-200 * 24 * time.Hour)
	}
	for i := range old.Edges {
		old.Edges[i].OccurredAt = fundgraph.FixtureClock.Add(-200 * 24 * time.Hour)
	}
	svc.SetClock(func() time.Time { return fundgraph.FixtureClock })
	if _, err := svc.Ingest(ctx, old); err != nil {
		t.Fatal(err)
	}
	before, err := svc.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PlanPrune(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Mark) != before.Edges {
		t.Fatalf("mark %d edges %d", len(plan.Mark), before.Edges)
	}
	for _, item := range plan.Mark {
		if item.Tier != fundgraph.TierArchive {
			t.Fatalf("tier %s", item.Tier)
		}
	}
	res, err := svc.ApplyPrune(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.Marked != before.Edges || res.Deleted != 0 {
		t.Fatalf("apply %+v", res)
	}
	after, err := svc.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Edges != before.Edges {
		t.Fatalf("deleted silently %d -> %d", before.Edges, after.Edges)
	}
	plan.Mode = "delete"
	plan.Delete = []string{plan.Mark[0].EdgeID}
	plan.Confirm = fundgraph.ConfirmToken(plan.Delete)
	if _, err := svc.ApplyPrune(ctx, plan); !errors.Is(err, fundgraph.ErrDeleteDisabled) {
		t.Fatalf("delete %v", err)
	}
	cfg := fundgraph.DefaultConfig()
	cfg.AllowDelete = true
	svc.cfg = cfg
	plan.Confirm = "nope"
	if _, err := svc.ApplyPrune(ctx, plan); !errors.Is(err, fundgraph.ErrConfirm) {
		t.Fatalf("confirm %v", err)
	}
	plan.Confirm = fundgraph.ConfirmToken(plan.Delete)
	res, err = svc.ApplyPrune(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Fatalf("deleted %d", res.Deleted)
	}
}

func TestPruneDeleteRefusedWritesNothing(t *testing.T) {
	svc := testSvc(t)
	ctx := context.Background()
	batch := mustSpine(t, fundgraph.SpineParams{Ref: "prune-deny", Amount: "1"})
	if _, err := svc.Ingest(ctx, batch); err != nil {
		t.Fatal(err)
	}
	id := batch.Edges[0].ID
	plan := fundgraph.PrunePlan{
		Mode:		"delete",
		Mark:		[]fundgraph.PruneItem{{EdgeID: id, Tier: fundgraph.TierCold}},
		Delete:		[]string{id},
		Confirm:	fundgraph.ConfirmToken([]string{id}),
	}
	if _, err := svc.ApplyPrune(ctx, plan); !errors.Is(err, fundgraph.ErrDeleteDisabled) {
		t.Fatalf("delete %v", err)
	}
	edges, err := svc.Edges(ctx, fundgraph.EdgeFilter{Ref: "prune-deny"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edges {
		if e.ID == id && e.Attrs["tier"] != "" {
			t.Fatalf("tier written on refused delete %+v", e.Attrs)
		}
	}
}

func TestHTTPLineageAndConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := testSvc(t)
	batch := mustSpine(t, fundgraph.SpineParams{Ref: "http-1", UserID: "u", Amount: "6"})
	if _, err := svc.Ingest(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	r := fundgraph.Router(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "fund_graph_store") {
		t.Fatalf("health %d %s", w.Code, w.Body.String())
	}
	body := `{"start_ref":"http-1","direction":"reverse"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/fund-graph/lineage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "deposit_observe") {
		t.Fatalf("lineage %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/fund-graph/lineage?start_ref=http-1&direction=forward", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "withdraw_broadcast") {
		t.Fatalf("get lineage %d %s", w.Code, w.Body.String())
	}
	batch.Edges[0].Amount = "100"
	raw, _ := json.Marshal(batch)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/fund-graph/ingest", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatalf("conflict %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/fund-graph/inspect", strings.NewReader(`{}`))
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("inspect %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/fund-graph/catalog", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "observe_to_credit") {
		t.Fatalf("catalog %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/fund-graph/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"edges":4`) {
		t.Fatalf("metrics %s", w.Body.String())
	}
}

func TestCatalogFrozenAndSchema(t *testing.T) {
	if len(fundgraph.EdgeCatalog()) != fundgraph.EdgeTypeCount {
		t.Fatalf("edges %d", len(fundgraph.EdgeCatalog()))
	}
	if len(fundgraph.NodeCatalog()) != fundgraph.NodeKindCount {
		t.Fatalf("nodes %d", len(fundgraph.NodeCatalog()))
	}
	seen := map[fundgraph.EdgeType]bool{}
	for _, spec := range fundgraph.EdgeCatalog() {
		if seen[spec.Type] {
			t.Fatalf("dup %s", spec.Type)
		}
		seen[spec.Type] = true
		if !fundgraph.KnownEdgeType(spec.Type) {
			t.Fatal(spec.Type)
		}
		if spec.From == "" || spec.To == "" || spec.Title == "" {
			t.Fatalf("spec %+v", spec)
		}
	}
	for _, kind := range fundgraph.SpineStages {
		if !fundgraph.KnownNodeKind(kind) {
			t.Fatal(kind)
		}
		if link,
			ok := fundgraph.SpineLink(kind); ok && !fundgraph.SpineEdge(link) {
			t.Fatalf("link %s", link)
		}
	}
	if len(fundgraph.Statements()) != 5 {
		t.Fatalf("statements %d", len(fundgraph.Statements()))
	}
	if _, err := os.Stat("../../migrations/0044_fund_graph.sql"); !os.IsNotExist(err) {
		t.Fatal("fund graph migration must not reuse 0044")
	}
	mig, err := os.ReadFile("../../migrations/0057_fund_graph.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(mig) != fundgraph.SchemaSQL {
		t.Fatal("migration drifted from embedded schema")
	}
	for _, st := range fundgraph.Statements() {
		if !strings.Contains(st, "fund_graph_") {
			t.Fatal(st)
		}
	}
	if _, err := fundgraph.NormAmount("1.50"); err != nil || mustAmt("1.50") != "1.5" {
		t.Fatalf("amt %s %v", mustAmt("1.50"), err)
	}
}

func mustAmt(s string) string {
	a, err := fundgraph.NormAmount(s)
	if err != nil {
		return err.Error()
	}
	return a
}

func TestWorkerMemorySourceConflictStopsCursor(t *testing.T) {
	svc := testSvc(t)
	good := mustSpine(t, fundgraph.SpineParams{Ref: "w-1", Amount: "1"})
	bad := mustSpine(t, fundgraph.SpineParams{Ref: "w-1", Amount: "1"})
	bad.Edges[0].Amount = "5"
	src := fundgraph.NewMemorySource("mem", "spine", []fundgraph.IngestBatch{good, bad})
	w := &fundgraph.Worker{Svc: svc, Sources: []fundgraph.Source{src}}
	rep, err := w.Once(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conflicts != 1 {
		t.Fatalf("first %+v", rep)
	}
	cur, ok, err := svc.GetCursor(context.Background(), "mem", "spine")
	if err != nil || !ok || cur.Token != "1" {
		t.Fatalf("cursor %+v %v", cur, err)
	}
	rep, err = w.Once(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conflicts != 1 {
		t.Fatalf("second %+v", rep)
	}
	cur, _, _ = svc.GetCursor(context.Background(), "mem", "spine")
	if cur.Token != "1" {
		t.Fatalf("cursor advanced on conflict %s", cur.Token)
	}
	edges, err := svc.Edges(context.Background(), fundgraph.EdgeFilter{Ref: "w-1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edges {
		if e.Amount == "5" {
			t.Fatal("bad amount landed")
		}
	}
}
