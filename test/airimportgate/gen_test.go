package airimportgate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/airimportgate"
)

func TestParseImportGen_Expiry(t *testing.T) {
	base := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name		string
		gen		string
		now		time.Time
		ttl		time.Duration
		wantGenErr	error
		wantExpiry	error
	}{
		{
			name:		"刚好有效（10s 内）",
			gen:		airimportgate.MakeImportGen("00000001-0000-0000-0000-000000000001", base.Add(-10*time.Second)),
			now:		base,
			ttl:		airimportgate.DefaultGenTTL,
			wantGenErr:	nil,
			wantExpiry:	nil,
		},
		{
			name:		"恰好到期（300s）",
			gen:		airimportgate.MakeImportGen("00000001-0000-0000-0000-000000000002", base.Add(-300*time.Second)),
			now:		base,
			ttl:		airimportgate.DefaultGenTTL,
			wantGenErr:	nil,
			wantExpiry:	nil,	// 等于不超过，不算过期
		},
		{
			name:		"已过期（301s）",
			gen:		airimportgate.MakeImportGen("00000001-0000-0000-0000-000000000003", base.Add(-301*time.Second)),
			now:		base,
			ttl:		airimportgate.DefaultGenTTL,
			wantGenErr:	nil,
			wantExpiry:	airimportgate.ErrImportGenExpired,
		},
		{
			name:		"自定义 TTL 60s 已过期",
			gen:		airimportgate.MakeImportGen("00000001-0000-0000-0000-000000000004", base.Add(-61*time.Second)),
			now:		base,
			ttl:		60 * time.Second,
			wantGenErr:	nil,
			wantExpiry:	airimportgate.ErrImportGenExpired,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := airimportgate.ParseImportGen(tc.gen)
			if err != tc.wantGenErr {
				t.Fatalf("ParseImportGen want %v, got %v", tc.wantGenErr, err)
			}
			if tc.wantGenErr != nil {
				return
			}
			err = airimportgate.CheckGenExpiry(parsed, tc.ttl, tc.now)
			if err != tc.wantExpiry {
				t.Errorf("CheckGenExpiry want %v, got %v", tc.wantExpiry, err)
			}
		})
	}
}
