package usageleasegate_test

import (
	"testing"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/usageleasegate"
)

func nopLog() zerolog.Logger	{ return zerolog.Nop() }

func testEng(t *testing.T) *usageleasegate.Engine {
	t.Helper()
	us := usage.New()
	return usageleasegate.New(usageleasegate.Config{Store: usageleasegate.NewMemStore(), Usage: us, AutoResv: true})
}

func snapReq(user, asset, dest, amt, cap, ttl string) usageleasegate.SnapshotWithLeaseReq {
	return usageleasegate.SnapshotWithLeaseReq{
		Kind:	usage.KindWithdraw, UserID: user, Asset: asset, Dest: dest,
		Amount:	amt, DailyCap: cap, TTL: ttl, Reserve: true,
	}
}
