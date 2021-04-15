package cvquotelease_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/cvquotelease"
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
	eng, marks := testEng(t)
	setup := gc.Setup
	from, _ := setup["from"].(string)
	to, _ := setup["to"].(string)
	rate, _ := setup["initial_rate"].(string)
	if rate == "" {
		rate, _ = setup["rate"].(string)
	}
	if from != "" && to != "" && rate != "" {
		marks.Put(from, to, rate)
		_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: from, To: to, Rate: rate})
	}
	var last cvquotelease.QuoteWithLeaseResp
	for _, act := range gc.Actions {
		typ, _ := act["type"].(string)
		switch typ {
		case "mark_put":
			fr, _ := act["from"].(string)
			td, _ := act["to"].(string)
			rt, _ := act["rate"].(string)
			if fr == "" {
				fr = from
			}
			if td == "" {
				td = to
			}
			_, err := eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: fr, To: td, Rate: rt})
			if err != nil {
				t.Fatalf("mark_put: %v", err)
			}
		case "quote":
			uid, _ := setup["user_id"].(string)
			if v, ok := act["user_id"].(string); ok && v != "" {
				uid = v
			}
			ttl, _ := act["ttl"].(string)
			if ttl == "" {
				ttl, _ = setup["ttl"].(string)
			}
			amt, _ := setup["amount"].(string)
			book, _ := setup["book"].(string)
			q, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
				UserID:	uid, From: from, To: to, Amount: amt, Book: book, TTL: ttl,
			})
			if err != nil {
				t.Fatalf("quote: %v", err)
			}
			last = q
		case "wait":
			d, _ := act["duration"].(string)
			if d != "" {
				if dur, err := time.ParseDuration(d); err == nil {
					time.Sleep(dur)
				}
			}
		case "scan_expired":
			n, err := eng.ScanExpiredQuotes(context.Background(), 50)
			if err != nil || n < 1 {
				t.Fatalf("scan n=%d err=%v", n, err)
			}
		case "execute":
			uid, _ := setup["user_id"].(string)
			if v, ok := act["user_id"].(string); ok && v != "" {
				uid = v
			}
			leaseID := last.LeaseID
			if omit, _ := act["omit_lease"].(bool); omit {
				leaseID = ""
			}
			ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
				UserID:	uid, QuoteID: last.QuoteID, LeaseID: leaseID,
			})
			expect, _ := act["expect"].(map[string]any)
			if filled, _ := expect["filled"].(bool); filled {
				if err != nil || !ex.Filled {
					t.Fatalf("want fill %+v %v", ex, err)
				}
			} else if expect != nil {
				if err == nil && ex.Filled {
					t.Fatalf("want reject %+v", ex)
				}
				if hp, _ := expect["has_proof"].(bool); hp && ex.DriftProof == nil {
					t.Fatal("missing proof")
				}
			}
		case "dual_execute":
			var filled int
			var order string
			for i := 0; i < 2; i++ {
				ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
					UserID:		last.UserID,
					QuoteID:	last.QuoteID, LeaseID: last.LeaseID,
				})
				if err == nil && ex.Filled {
					if !ex.Reused {
						filled++
						order = ex.OrderID
					} else if order == "" {
						order = ex.OrderID
						filled++
					}
				}
			}
			// 第二笔 reuse 也算同一成交
			lease, _ := eng.GetLease(context.Background(), last.LeaseID)
			if lease.State != cvquotelease.LeaseConsumed || !lease.Used {
				t.Fatalf("dual consume %+v filled=%d", lease, filled)
			}
		case "place_quoted":
			ex, err := eng.PlaceQuoted(context.Background(), cvquotelease.PlaceQuotedReq{
				UserID:	setup["user_id"].(string), From: from, To: to,
				Amount:	setup["amount"].(string), Rate: rate, Book: "funding",
			})
			if err == nil || ex.Filled {
				t.Fatalf("bare must fail %+v", ex)
			}
			if ex.DriftProof == nil {
				t.Fatal("missing lease proof")
			}
		}
	}
}
