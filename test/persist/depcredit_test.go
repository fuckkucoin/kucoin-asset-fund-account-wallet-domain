package persist_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/persist"
)

func TestHT12DepositIndexesInSchema(t *testing.T) {
	t.Parallel()
	have := persist.IndexesFromDDL(persist.SchemaSQL + persist.SchemaPostgres)
	if err := persist.ProbeDepositCreditIndexes(have); err != nil {
		t.Fatalf("H-T12 schema: %v have=%v", err, have)
	}
	_, file, _, _ := runtime.Caller(0)
	mig := filepath.Join(filepath.Dir(file), "..", "..", "migrations", "0147_aa_deposits_credit_cols.sql")
	raw, err := os.ReadFile(mig)
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.ProbeDepositCreditIndexes(persist.IndexesFromDDL(string(raw))); err != nil {
		t.Fatal(err)
	}
	cols := persist.ColumnsFromDDL(persist.SchemaSQL)
	if err := persist.ProbeDepositCreditColumns(cols); err != nil {
		t.Fatalf("cols %v %v", cols, err)
	}
	if !persist.NaturalKeyFromDDL(persist.SchemaSQL) || !persist.NaturalKeyFromDDL(persist.SchemaPostgres) {
		t.Fatal("natural key missing")
	}
	if strings.Contains(strings.ToLower(persist.SchemaSQL), "primary key (chain, tx_hash)") &&
		!strings.Contains(strings.ToLower(persist.SchemaSQL), "output_index") {
		t.Fatal("old key")
	}
}

func TestProbeDepositCreditMissing(t *testing.T) {
	t.Parallel()
	if err := persist.ProbeDepositCreditColumns([]string{"chain"}); !errors.Is(err, persist.ErrDepositCols) {
		t.Fatalf("%v", err)
	}
	if err := persist.ProbeDepositCreditIndexes([]string{"idx_aa_dep_xfer"}); !errors.Is(err, persist.ErrDepositCols) {
		t.Fatalf("%v", err)
	}
}
