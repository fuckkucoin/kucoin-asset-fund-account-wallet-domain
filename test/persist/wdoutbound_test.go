package persist_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"account-assets.kucoin.net/funding/internal/persist"
)

func TestWDOutboundDDLProbe(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	mig := filepath.Join(filepath.Dir(file), "..", "..", "migrations", "0222_aa_wd_outbound_saga.sql")
	raw, err := os.ReadFile(mig)
	if err != nil {
		t.Fatal(err)
	}
	ddl := string(raw)
	if !persist.WDOutboundNaturalKeyFromDDL(ddl) {
		t.Fatal("natural key transfer_id")
	}
	cols := persist.WDOutboundColumnsFromDDL(ddl)
	if err := persist.ProbeWDOutboundColumns(cols); err != nil {
		t.Fatal(err)
	}
	idx := persist.WDOutboundIndexesFromDDL(ddl)
	if err := persist.ProbeWDOutboundIndexes(idx); err != nil {
		t.Fatal(err)
	}
	if err := persist.ProbeWDOutboundColumns([]string{"transfer_id"}); err == nil {
		t.Fatal("want missing cols")
	}
}
