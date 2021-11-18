package screenleasegate_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/screenleasegate"
)

func TestBindStoreRefuseProductionMem(t *testing.T) {
	rt := &app.Runtime{Mem: true, Cfg: &config.Config{Profile: "production", Auth: config.AuthConfig{Kind: "iam"}, Storage: "mem"}}
	if _, err := screenleasegate.BindStore(rt); !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("%v", err)
	}
}

func TestBindStoreDevMem(t *testing.T) {
	rt := &app.Runtime{Mem: true, Cfg: &config.Config{Profile: "mem", Auth: config.AuthConfig{Kind: "mem"}, Storage: "mem"}}
	st, err := screenleasegate.BindStore(rt)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*screenleasegate.MemStore); !ok {
		t.Fatalf("%T", st)
	}
}
