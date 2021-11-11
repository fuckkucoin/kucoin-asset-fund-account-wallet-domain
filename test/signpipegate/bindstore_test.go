package signpipegate_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/signpipegate"
)

func TestBindStoreRefuseProductionMem(t *testing.T) {
	rt := &app.Runtime{Mem: true, Cfg: &config.Config{Profile: "production", Storage: "mem"}}
	if _, err := signpipegate.BindStore(rt); !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("%v", err)
	}
}

func TestBindStoreDevMem(t *testing.T) {
	st, err := signpipegate.BindStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*signpipegate.MemStore); !ok {
		t.Fatalf("%T", st)
	}
}
