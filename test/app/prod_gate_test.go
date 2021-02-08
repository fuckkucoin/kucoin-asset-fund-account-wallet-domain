package app_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/app"
)

func TestRefuseGateMemStorage(t *testing.T) {
	err := app.RefuseGateMemStorage(&config.Config{Storage: "mem"})
	if !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("%v", err)
	}
	if err := app.RefuseGateMemStorage(&config.Config{Storage: "sql"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATE_STORAGE", "mem")
	if err := app.RefuseGateMemStorage(&config.Config{Storage: "sql"}); !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("GATE_STORAGE=mem must refuse, got %v", err)
	}
}

func TestLoadedProductionConfigClearsCallGates(t *testing.T) {
	t.Setenv("CONVERT_FILL_REQUIRE_IDENT", "")
	t.Setenv("CONVERT_FILL_HMAC_SECRET", "")
	t.Setenv("CONVERT_FILL_MTLS_CNS", "")
	t.Setenv("GATE_STORAGE", "")
	t.Setenv("KEY_SOURCE", "")
	t.Setenv("RSA_PRIVATE_KEY_PATH", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RefuseProduction(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestRefuseProductionRejectsMem(t *testing.T) {
	t.Setenv("CONVERT_FILL_REQUIRE_IDENT", "1")
	t.Setenv("CONVERT_FILL_HMAC_SECRET", "unit-fill-hmac")
	t.Setenv("GATE_STORAGE", "mem")
	cfg := prodGateFixture()
	cfg.Storage = "sql"
	if err := app.RefuseProduction(cfg); !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("want ErrMemRegister got %v", err)
	}
	t.Setenv("GATE_STORAGE", "")
	cfg.Storage = "mem"
	if err := app.RefuseProduction(cfg); !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("want ErrMemRegister got %v", err)
	}
}

func TestRefuseMemDevAllows(t *testing.T) {
	if err := app.RefuseMem(&config.Config{Profile: "mem", Auth: config.AuthConfig{Kind: "mem"}}); err != nil {
		t.Fatal(err)
	}
	if err := app.RefuseGateMemStorage(&config.Config{Profile: "dev", Storage: "mem"}); !errors.Is(err, persist.ErrMemRegister) {
		t.Fatalf("helper always treats caller as production, got %v", err)
	}
	if err := persist.RefuseMemRegister(false, "mem", true); err != nil {
		t.Fatal(err)
	}
}

func prodGateFixture() *config.Config {
	return &config.Config{
		MySQLDSN:	"acct@tcp(mysql.kucoin.net)/funding",
		RedisAddr:	"redis.kucoin.net:6379",
		KafkaOptional:	true,
		Profile:	"prod",
		Storage:	"sql",
		Auth:		config.AuthConfig{Kind: "iam", Enabled: true, Issuer: "https://iam.kucoin.net", JWKSURL: "https://iam.kucoin.net/.well-known/jwks.json"},
		KYT:		config.KYTConfig{Mode: "vendor", VendorURL: "https://kyt.kucoin.net"},
		KMS:		config.KMSConfig{Kind: "cloud-kms", Endpoint: "https://kms.kucoin.net"},
		Transfer:	config.TransferConfig{ConfirmDepth: 12},
		Etcd:		config.EtcdConfig{Endpoints: []string{"etcd.kucoin.net:2379"}},
		KeySource:	"env",
	}
}
