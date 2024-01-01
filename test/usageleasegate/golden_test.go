package usageleasegate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/usageleasegate"
)

type goldenCase struct {
	Name		string			`json:"name"`
	Description	string			`json:"description"`
	Setup		map[string]any		`json:"setup"`
	Actions		[]map[string]any	`json:"actions"`
	Expected	map[string]any		`json:"expected"`
}

func TestGoldenDriver(t *testing.T) {
	root := testdataRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "case_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 4 {
		t.Fatalf("need >=4 goldens, got %d", len(files))
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var gc goldenCase
			if err := json.Unmarshal(b, &gc); err != nil {
				t.Fatal(err)
			}
			runGolden(t, gc)
		})
	}
}

func runGolden(t *testing.T, gc goldenCase) {
	t.Helper()
	eng := testEng(t)
	setup := gc.Setup
	user, _ := setup["user_id"].(string)
	asset, _ := setup["asset"].(string)
	dest, _ := setup["dest"].(string)
	kind := usage.Kind(str(setup["kind"], "withdraw"))
	cap := str(setup["daily_cap"], "100")
	ttl := str(setup["ttl"], "2m")
	leases := map[string]usageleasegate.SnapshotWithLeaseResp{}
	amts := map[string]string{}
	committed := 0
	failed := 0
	for _, act := range gc.Actions {
		typ, _ := act["type"].(string)
		switch typ {
		case "snapshot":
			amt := str(act["amount"], str(setup["amount"], "1"))
			resp, err := eng.SnapshotWithLease(context.Background(), usageleasegate.SnapshotWithLeaseReq{
				Kind:	kind, UserID: user, Asset: asset, Dest: dest,
				Amount:		amt,
				DailyCap:	cap, TTL: ttl, Reserve: true,
			})
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if resp.LeaseID == "" {
				t.Fatal("missing usage_lease_id")
			}
			id, _ := act["id"].(string)
			leases[id] = resp
			amts[id] = amt
			if ex, ok := act["expect"].(map[string]any); ok {
				if v, _ := ex["reserved"].(bool); v && !resp.Reserved {
					t.Fatalf("want reserved %+v", resp)
				}
			}
		case "foreign_add":
			amt := str(act["amount"], "1")
			if err := eng.Usage().Add(kind, user, asset, dest, amt); err != nil {
				t.Fatal(err)
			}
		case "record":
			id, _ := act["id"].(string)
			ls := leases[id]
			amt := amts[id]
			if a := str(act["amount"], ""); a != "" {
				amt = a
			}
			rec, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
				LeaseID:	ls.LeaseID, Kind: kind, UserID: user, Asset: asset, Dest: dest, Amount: amt,
			})
			ex, _ := act["expect"].(map[string]any)
			wantOK, _ := ex["committed"].(bool)
			if wantOK {
				if err != nil || !rec.Committed {
					t.Fatalf("want commit: %v %+v", err, rec)
				}
				committed++
			} else {
				if err == nil || rec.Committed {
					t.Fatalf("want fail %+v", rec)
				}
				failed++
				if v, _ := ex["has_proof"].(bool); v && rec.DriftProof == nil {
					t.Fatal("missing proof")
				}
			}
		case "record_concurrent":
			ids, _ := act["ids"].([]any)
			var wg sync.WaitGroup
			var mu sync.Mutex
			for _, raw := range ids {
				id, _ := raw.(string)
				ls := leases[id]
				amt := amts[id]
				if amt == "" {
					amt = "80"
				}
				wg.Add(1)
				go func(ls usageleasegate.SnapshotWithLeaseResp, amt string) {
					defer wg.Done()
					rec, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
						LeaseID:	ls.LeaseID, Kind: kind, UserID: user, Asset: asset, Dest: dest, Amount: amt,
					})
					mu.Lock()
					defer mu.Unlock()
					if err == nil && rec.Committed {
						committed++
					} else {
						failed++
						if rec.DriftProof == nil && err == nil {
							t.Error("fail without proof")
						}
					}
				}(ls, amt)
			}
			wg.Wait()
			if committed != 1 || failed != 1 {
				t.Fatalf("concurrent committed=%d failed=%d", committed, failed)
			}
			if eng.Usage().Len() != 1 {
				t.Fatalf("occupancy must be 1 got %d", eng.Usage().Len())
			}
		case "record_bare":
			rec, err := eng.RecordAssert(context.Background(), usageleasegate.RecordAssertReq{
				Kind:	kind, UserID: user, Asset: asset, Dest: dest, Amount: str(setup["amount"], "1"),
			})
			if err == nil || rec.Committed {
				t.Fatalf("bare must fail %+v", rec)
			}
			if rec.DriftProof == nil {
				t.Fatal("missing lease proof")
			}
			if eng.Usage().Len() != 0 {
				t.Fatal("bare must not count occupancy")
			}
		case "sleep_ms":
			ms := 5.0
			if v, ok := act["ms"].(float64); ok {
				ms = v
			}
			time.Sleep(time.Duration(ms) * time.Millisecond)
		case "expire_release":
			n,
				err := eng.ExpireRelease(context.Background(), 50)
			if err != nil {
				t.Fatal(err)
			}
			if ex, ok := act["expect"].(map[string]any); ok {
				if v, ok := ex["released_gte"].(float64); ok && float64(n) < v {
					t.Fatalf("released %d", n)
				}
			}
		default:
			t.Fatalf("unknown action %s", typ)
		}
	}
	if v, ok := gc.Expected["occupancy_positive"].(bool); ok && v && eng.Usage().Len() < 1 {
		t.Fatal("expected occupancy")
	}
	if v, ok := gc.Expected["occupancy_len"].(float64); ok && eng.Usage().Len() != int(v) {
		t.Fatalf("occupancy %d", eng.Usage().Len())
	}
}

func (s usageleasegate.SnapshotWithLeaseResp,) KindAmount(setup, act map[string]any) string {
	if v := str(act["amount"], ""); v != "" {
		return v
	}
	return amountOf(setup, s)
}

func amountOf(setup map[string]any, s usageleasegate.SnapshotWithLeaseResp,) string {
	if s.LeaseID != "" {
		// amount lives on lease; tests pass same amount as snapshot
	}
	if v := str(setup["amount"], ""); v != "" {
		return v
	}
	if v := str(setup["amount_a"], ""); v != "" && s.TodaySum != "" {
		return v
	}
	return "80"
}

func str(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func testdataRoot(t *testing.T) string {
	t.Helper()
	cands := []string{
		"testdata/usageleasegate",
		filepath.Join("..", "..", "testdata", "usageleasegate"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	t.Fatal("testdata/usageleasegate not found")
	return ""
}
