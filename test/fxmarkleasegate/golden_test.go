package fxmarkleasegate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fxmarkleasegate"
)

type goldenCase struct {
	Name		string			`json:"name"`
	Description	string			`json:"description"`
	Setup		map[string]any		`json:"setup"`
	Actions		[]map[string]any	`json:"actions"`
	Expected	map[string]any		`json:"expected"`
}

func testdataRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(wd, "testdata", "fxmarkleasegate")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("testdata/fxmarkleasegate not found")
	return ""
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
	rd := &fxmarkleasegate.StaticReader{USDMap: map[string]string{}, Gens: map[string]int64{}}
	if usd, ok := gc.Setup["usd"].(map[string]any); ok {
		for k, v := range usd {
			rd.USDMap[strings.ToUpper(k)] = strAny(v)
		}
	}
	if live, _ := gc.Setup["live"].(bool); live {
		rd.Live = true
	}
	ttl := str(gc.Setup, "ttl", "2m")
	d, err := time.ParseDuration(ttl)
	if err != nil {
		d = 2 * time.Minute
	}
	eng := fxmarkleasegate.New(fxmarkleasegate.Config{Store: fxmarkleasegate.NewMemStore(),
		Reader:	rd, TTL: d, ThresholdBPS: 10})
	leases := map[string]fxmarkleasegate.IssueMarkLeaseResp{}
	from := str(gc.Setup, "from", "ETH")
	to := str(gc.Setup, "to", "USDT")
	purpose := str(gc.Setup, "purpose", "convert")
	var lastAssert fxmarkleasegate.AssertBeforePostResp
	var lastErr error
	for _, act := range gc.Actions {
		typ, _ := act["type"].(string)
		switch typ {
		case "issue":
			prefer, _ := act["prefer_live"].(bool)
			if !prefer {
				prefer, _ = gc.Setup["prefer_live"].(bool)
			}
			resp, err := eng.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{
				From:	from, To: to, Purpose: purpose, TTL: ttl, PreferLive: prefer,
			})
			expect := str(act, "expect_error", "")
			if expect != "" {
				if err == nil {
					t.Fatalf("issue expected error %s", expect)
				}
				lastErr = err
				continue
			}
			if err != nil {
				t.Fatalf("issue: %v", err)
			}
			id, _ := act["id"].(string)
			leases[id] = resp
		case "put":
			rd.PutUSD(str(act, "asset", from), str(act, "usd", "1"))
			_, _ = eng.OnPutChange(context.Background(), str(act, "asset", from), str(act, "usd", "1"))
		case "live_flip":
			rd.SetUSDNoBump(str(act, "asset", from), str(act, "usd", "1"))
		case "sleep_ms":
			ms := 5
			if n, ok := act["ms"].(float64); ok {
				ms = int(n)
			}
			time.Sleep(time.Duration(ms) * time.Millisecond)
		case "assert":
			id, _ := act["id"].(string)
			l := leases[id]
			consume, _ := act["consume"].(bool)
			lastAssert, lastErr = eng.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{
				LeaseID:	l.LeaseID, From: from, To: to, Purpose: purpose, Consume: consume,
			})
			expect := str(act, "expect_error", "")
			if expect != "" && lastErr == nil {
				t.Fatalf("assert expected error containing %s", expect)
			}
			if expect == "" && lastErr != nil {
				t.Fatalf("assert: %v", lastErr)
			}
		default:
			t.Fatalf("unknown action %s", typ)
		}
	}
	exp := gc.Expected
	if posted, ok := exp["posted"].(bool); ok && lastAssert.Posted != posted {
		t.Fatalf("posted=%v want %v err=%v", lastAssert.Posted, posted, lastErr)
	}
	if kind, ok := exp["proof_kind"].(string); ok && kind != "" {
		if lastAssert.DriftProof == nil || string(lastAssert.DriftProof.Kind) != kind {
			got := ""
			if lastAssert.DriftProof != nil {
				got = string(lastAssert.DriftProof.Kind)
			}
			t.Fatalf("proof kind %s want %s err=%v", got, kind, lastErr)
		}
		if !lastAssert.DriftProof.Blocked {
			t.Fatal("proof must be blocked")
		}
	}
	if st, ok := exp["state"].(string); ok && st != "" && lastAssert.State != fxmarkleasegate.LeaseState(st) {
		if lastAssert.LeaseID != "" {
			got, _ := eng.GetLease(context.Background(), lastAssert.LeaseID)
			if string(got.State) != st && string(lastAssert.State) != st {
				t.Fatalf("state %s/%s want %s", lastAssert.State, got.State, st)
			}
		}
	}
	if fc, ok := exp["fail_closed"].(bool); ok && fc {
		if lastErr == nil && lastAssert.Posted {
			t.Fatal("fail-closed expected")
		}
	}
	if issued, ok := exp["issued"].(bool); ok && !issued && lastErr == nil && len(leases) > 0 {
		t.Fatal("issue should have failed")
	}
}

func str(m map[string]any, k, def string) string {
	if m == nil {
		return def
	}
	return strAnyDef(m[k], def)
}

func strAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func strAnyDef(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}
