package svcreg_test

import (
	"encoding/json"
	"testing"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protowire"
	"account-assets.kucoin.net/funding/internal/svcreg"
)

func TestParseEtcdTargetPlain(t *testing.T) {
	target, tlsOn, err := svcreg.ParseEtcdTarget("http://etcd-0.kucoin.net:2379")
	if err != nil {
		t.Fatal(err)
	}
	if tlsOn {
		t.Fatal("http endpoint must be plaintext grpc")
	}
	if target != "etcd-0.kucoin.net:2379" {
		t.Fatalf("target %s", target)
	}
	target, tlsOn, err = svcreg.ParseEtcdTarget("https://etcd-0.kucoin.net:2379")
	if err != nil || !tlsOn || target != "etcd-0.kucoin.net:2379" {
		t.Fatalf("tls target=%s tls=%v err=%v", target, tlsOn, err)
	}
}

func TestAliasNamesLedger(t *testing.T) {
	names := svcreg.AliasNames("ledger-service")
	if len(names) < 2 || names[0] != "ledger-service" || names[1] != "ledger" {
		t.Fatalf("%v", names)
	}
	m := svcreg.NewMem()
	if err := m.Register(nil, svcreg.Instance{Name: "ledger", Host: "ledger.kucoin.net", Addr: "http://ledger.kucoin.net:18480"}); err != nil {
		t.Fatal(err)
	}
	inst, err := m.Pick("ledger-service")
	if err != nil || inst.Name != "ledger" {
		t.Fatalf("pick %+v %v", inst, err)
	}
}

func TestDecodeRangeInstance(t *testing.T) {
	body, _ := json.Marshal(svcreg.Instance{Name: "ledger", Addr: "http://ledger.kucoin.net:18480"})
	kv := svcreg.AppendBytesField(nil, 5, body)
	msg := svcreg.AppendBytesField(nil,
		2, kv)
	list := svcreg.DecodeInstances(msg)
	if len(list) != 1 || list[0].Name != "ledger" {
		t.Fatalf("%+v", list)
	}
	id := svcreg.DecodeVarintField(svcreg.AppendVarintField(nil, 2, 42), 2)
	if id != 42 {
		t.Fatal(id)
	}
	_ = protowire.VarintType
}

func TestOpenEtcdBuildsPlainDial(t *testing.T) {
	et, err := svcreg.OpenEtcd([]string{"http://etcd-0.kucoin.net:2379"}, "/aa/svc", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if len(et.dials) != 1 || et.dials[0].tls || et.dials[0].target != "etcd-0.kucoin.net:2379" {
		t.Fatalf("%+v", et.dials)
	}
}
