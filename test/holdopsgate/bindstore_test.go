package holdopsgate_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
)

func TestBindStoreRefuseProductionMem(t *testing.T) {
	rt := &app.Runtime{Mem: true, Cfg: &config.Config{Profile: "prod", Auth: config.AuthConfig{Kind: "iam"}, Storage: "mem"}}
	if _, err := holdopsgate.BindStore(rt); !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("%v", err)
	}
}

func TestBindStoreDevMem(t *testing.T) {
	st, err := holdopsgate.BindStore(&app.Runtime{Mem: true, Cfg: &config.Config{Profile: "dev"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*holdopsgate.MemStore); !ok {
		t.Fatalf("%T", st)
	}
}
