package batchhopgate_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "batchhopgate")
}

func TestGolden_CaseJSONLoad(t *testing.T) {
	dir := testdataDir(t)
	files := []string{
		"case_01_partial_fail_residual.json",
		"case_02_concurrent_resume_mutex.json",
		"case_03_no_token_reject.json",
		"case_04_acctpol_isolation.json",
	}
	for _, name := range files {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("json %s: %v", name, err)
		}
		if body["scenario"] == nil || body["user_id"] == nil {
			t.Errorf("%s missing scenario/user_id", name)
		}
		if body["proof_summary"] == nil {
			t.Errorf("%s missing proof_summary", name)
		}
	}
}

func TestGolden_ProofDigestCSV(t *testing.T) {
	path := filepath.Join(testdataDir(t), "golden_proof_digest.csv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 6 {
		t.Fatalf("rows=%d want >=6 (header+5)", len(rows))
	}
	if rows[0][0] != "proof_id" {
		t.Errorf("header %v", rows[0])
	}
	for i, row := range rows[1:] {
		if len(row) < 7 || row[4] == "" {
			t.Errorf("row %d incomplete: %v", i+1, row)
		}
	}
}

func TestGolden_FixtureSQLMarkers(t *testing.T) {
	path := filepath.Join(testdataDir(t), "fixture_lifecycle.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	need := []string{
		"portfolio_batch_residuals",
		"portfolio_resume_leases",
		"portfolio_hop_proofs",
		"u-batch-01",
		"u-batch-04",
	}
	s := string(raw)
	for _, n := range need {
		if !containsStr(s, n) {
			t.Errorf("fixture missing %s", n)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
