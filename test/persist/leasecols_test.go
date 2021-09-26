package persist_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/persist"
)

func TestProbeLeaseColumns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name	string
		have	[]string
		need	[]string
		ok	bool
	}{
		{name: "A-T10-screen-ok", have: persist.ScreenLeaseCols, need: persist.ScreenLeaseCols, ok: true},
		{name: "A-T10-holdops-ok", have: persist.HoldOpsLeaseCols, need: persist.HoldOpsLeaseCols, ok: true},
		{name: "A-T10-signpipe-ok", have: persist.SignPipeLeaseCols, need: persist.SignPipeLeaseCols, ok: true},
		{name: "A-T10-missing-expires", have: []string{"id", "actor"}, need: persist.HoldOpsLeaseCols, ok: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := persist.ProbeLeaseColumns(tc.have, tc.need)
			if tc.ok && err != nil {
				t.Fatal(err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected missing column")
			}
		})
	}
}
