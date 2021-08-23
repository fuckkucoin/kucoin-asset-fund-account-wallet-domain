package lending_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/lending/daycut"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/lending"
)

func TestBindDayCutProductionDefault(t *testing.T) {
	st := ledger.NewMemStore()
	led := ledger.NewService(st, nil, ledger.MemLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{DisableDriftBlock: true})
	svc := lending.New(led, nil)
	stack := svc.BindInterest(utabook.New(), nil)
	eng := stack.BindDayCut(daycut.NewMemStore(),
		daycut.Config{})
	if eng == nil {
		t.Fatal("bind")
	}
	if !eng.Cfg.ProductionLocked() {
		t.Fatalf("default cfg %q not production locked", eng.Cfg.Profile)
	}
	if err := eng.Cfg.RefuseRollback(); err == nil {
		t.Fatal("prod must refuse rollback")
	}
}
