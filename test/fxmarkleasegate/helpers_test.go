package fxmarkleasegate_test

import (
	"testing"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/fxmarkleasegate"
)

func nopLog() zerolog.Logger	{ return zerolog.Nop() }

func testReader() *fxmarkleasegate.StaticReader {
	return &fxmarkleasegate.StaticReader{
		USDMap:	map[string]string{"ETH": "3000", "USDT": "1", "USDC": "1", "BTC": "40000"},
		Gens:	map[string]int64{},
	}
}

func testEng(t *testing.T) (*fxmarkleasegate.Engine, *fxmarkleasegate.StaticReader,) {
	t.Helper()
	rd := testReader()
	e := fxmarkleasegate.New(fxmarkleasegate.Config{Store: fxmarkleasegate.NewMemStore(), Reader: rd, TTL: 2 * time.Minute, ThresholdBPS: 10})
	return e, rd
}
