package keyrotatefence_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/opsaudit"
	"account-assets.kucoin.net/funding/internal/keyrotatefence"
)

func TestFenceTable(t *testing.T) {
	ctx := context.Background()
	fixed := time.Date(2021, 11, 16, 8, 0, 0, 0, time.UTC)
	type row struct {
		id	string
		run	func(t *testing.T)
	}
	cases := []row{
		{"E-S01-主成功封窗", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil).WithClock(func() time.Time { return fixed })
			w, err := eng.Arm(ctx, keyrotatefence.ArmReq{
				TicketID:	"rot-e-001", IdemKey: "arm-1",
				OldKeyID:	"hsm-slot-a", NewKeyID: "hsm-slot-b",
				OldFingerprint:	"fp-old", OpenedBy: "ops-a",
				AuthKind:	"mem", Profile: "mem", KeySource: "kms",
			})
			if err != nil || w.Phase != keyrotatefence.PhaseArmed {
				t.Fatalf("arm: %v %+v", err, w)
			}
			w, err = eng.Cut(ctx, keyrotatefence.CutReq{TicketID: "rot-e-001", Actor: "ops-b"})
			if err != nil || w.Phase != keyrotatefence.PhaseCut {
				t.Fatalf("cut: %v %+v", err, w)
			}
			w, err = eng.Seal(ctx, keyrotatefence.SealReq{TicketID: "rot-e-001", NewFingerprint: "fp-new", Actor: "ops-c"})
			if err != nil || w.Phase != keyrotatefence.PhaseSealed {
				t.Fatalf("seal: %v %+v", err, w)
			}
			if err := eng.AssertUse(ctx, keyrotatefence.UseReq{TicketID: "rot-e-001", KeyID: "hsm-slot-b", Source: "kms", AuthKind: "mem", Profile: "mem"}); err != nil {
				t.Fatalf("new key: %v", err)
			}
		}},
		{"E-S02-切旧后旧钥失败封闭", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil).WithClock(func() time.Time { return fixed })
			mustArm(t, eng)
			if _, err := eng.Cut(ctx, keyrotatefence.CutReq{TicketID: "rot-e-001", Actor: "ops-b"}); err != nil {
				t.Fatal(err)
			}
			if err := eng.AssertUse(ctx, keyrotatefence.UseReq{TicketID: "rot-e-001", KeyID: "hsm-slot-a", Source: "kms"}); !errors.Is(err, keyrotatefence.ErrOldKeyRetired) {
				t.Fatalf("old key after cut: %v", err)
			}
		}},
		{"E-S03-Arm幂等与冲突", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			mustArm(t, eng)
			w, err := eng.Arm(ctx, keyrotatefence.ArmReq{
				TicketID:	"rot-e-001", IdemKey: "arm-1",
				OldKeyID:	"hsm-slot-a", NewKeyID: "hsm-slot-b",
				OldFingerprint:	"fp-old", OpenedBy: "ops-a",
				KeySource:	"kms",
			})
			if err != nil || w.Phase != keyrotatefence.PhaseArmed {
				t.Fatalf("reenter: %v", err)
			}
			if _, err := eng.Arm(ctx, keyrotatefence.ArmReq{
				TicketID:	"rot-e-001", IdemKey: "arm-1",
				OldKeyID:	"hsm-slot-a", NewKeyID: "hsm-slot-z",
				OldFingerprint:	"fp-old", OpenedBy: "ops-a",
			}); !errors.Is(err, keyrotatefence.ErrIdemConflict) {
				t.Fatalf("conflict: %v", err)
			}
		}},
		{"E-S04-Cut重入", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			mustArm(t, eng)
			if _, err := eng.Cut(ctx, keyrotatefence.CutReq{TicketID: "rot-e-001", Actor: "ops-b"}); err != nil {
				t.Fatal(err)
			}
			w, err := eng.Cut(ctx, keyrotatefence.CutReq{TicketID: "rot-e-001", Actor: "ops-b"})
			if err != nil || w.Phase != keyrotatefence.PhaseCut {
				t.Fatalf("cut reenter: %v %+v", err, w)
			}
		}},
		{"E-S05-重启后旧钥仍拒", func(t *testing.T) {
			src := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			mustArm(t, src)
			if _, err := src.Cut(ctx, keyrotatefence.CutReq{TicketID: "rot-e-001", Actor: "ops-b"}); err != nil {
				t.Fatal(err)
			}
			raw, err := src.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dst := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			if err := dst.Restore(ctx, raw); err != nil {
				t.Fatal(err)
			}
			got, err := dst.Get(ctx, "rot-e-001")
			if err != nil || got.Phase != keyrotatefence.PhaseCut {
				t.Fatalf("restore phase: %v %+v", err, got)
			}
			if err := dst.AssertUse(ctx, keyrotatefence.UseReq{TicketID: "rot-e-001", KeyID: "hsm-slot-a"}); !errors.Is(err, keyrotatefence.ErrOldKeyRetired) {
				t.Fatalf("after restart: %v", err)
			}
		}},
		{"E-S06-local-hmac锁死", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			_, err := eng.Arm(ctx, keyrotatefence.ArmReq{
				TicketID:	"rot-prod-hmac", OldKeyID: "k1", NewKeyID: "k2",
				OpenedBy:	"ops", AuthKind: "iam", Profile: "production",
				KeySource:	"local-hmac",
			})
			if !errors.Is(err, keyrotatefence.ErrLocalHMAC) {
				t.Fatalf("local-hmac: %v", err)
			}
			if err := eng.AssertUse(ctx, keyrotatefence.UseReq{Source: "gate-local-hmac", AuthKind: "iam", Profile: "prod"}); !errors.Is(err, keyrotatefence.ErrLocalHMAC) {
				t.Fatalf("assert local-hmac: %v", err)
			}
		}},
		{"E-S07-仓内私钥锁死", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			_, err := eng.Arm(ctx, keyrotatefence.ArmReq{
				TicketID:	"rot-prod-pem", OldKeyID: "k1", NewKeyID: "k2",
				OpenedBy:	"ops", AuthKind: "iam", Profile: "production",
				KeySource:	"kms", NewKeyPath: "configs/kms/kek.pem",
			})
			if !errors.Is(err, keyrotatefence.ErrDemoKey) {
				t.Fatalf("demo pem: %v", err)
			}
		}},
		{"E-S08-空来源锁死", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			_, err := eng.Arm(ctx, keyrotatefence.ArmReq{
				TicketID:	"rot-prod-empty", OldKeyID: "k1", NewKeyID: "k2",
				OpenedBy:	"ops", AuthKind: "iam", Profile: "prod",
			})
			if !errors.Is(err, keyrotatefence.ErrProdFallback) && !errors.Is(err, keyrotatefence.ErrLocalHMAC) {
				t.Fatalf("empty source: %v", err)
			}
		}},
		{"E-S09-审计链随围栏齐备", func(t *testing.T) {
			chain := opsaudit.NewChain(opsaudit.NewMemStore()).WithClock(func() time.Time { return fixed })
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), chain)
			mustArm(t, eng)
			if _, err := eng.Cut(ctx, keyrotatefence.CutReq{TicketID: "rot-e-001", Actor: "ops-b"}); err != nil {
				t.Fatal(err)
			}
			if _, err := eng.Seal(ctx, keyrotatefence.SealReq{TicketID: "rot-e-001", NewFingerprint: "fp-new", Actor: "ops-c"}); err != nil {
				t.Fatal(err)
			}
			if err := chain.Verify(ctx); err != nil {
				t.Fatal(err)
			}
			comp, err := chain.CheckRotate(ctx, "rot-e-001")
			if err != nil || !comp.OK {
				t.Fatalf("completeness: %v %+v", err, comp)
			}
		}},
		{"E-S10-blocked后禁止封窗", func(t *testing.T) {
			eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
			w, err := eng.Arm(ctx, keyrotatefence.ArmReq{
				TicketID:	"rot-block", OldKeyID: "k1", NewKeyID: "k2",
				OpenedBy:	"ops", AuthKind: "mem", Profile: "mem", KeySource: "kms",
			})
			if err != nil {
				t.Fatal(err)
			}
			w.NewKeyID = ""
			if err := eng.store.Save(ctx, w); err != nil {
				t.Fatal(err)
			}
			_, err = eng.Cut(ctx, keyrotatefence.CutReq{TicketID: "rot-block", Actor: "ops-b"})
			if !errors.Is(err, keyrotatefence.ErrNewKeyMissing) {
				t.Fatalf("cut empty new: %v", err)
			}
			if _, err := eng.Seal(ctx, keyrotatefence.SealReq{TicketID: "rot-block", NewFingerprint: "x", Actor: "ops-c"}); !errors.Is(err, keyrotatefence.ErrWindowBlocked) {
				t.Fatalf("seal blocked: %v", err)
			}
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, c.run)
	}
}

func mustArm(t *testing.T, eng *keyrotatefence.Engine,) {
	t.Helper()
	_, err := eng.Arm(context.Background(), keyrotatefence.ArmReq{
		TicketID:	"rot-e-001", IdemKey: "arm-1",
		OldKeyID:	"hsm-slot-a", NewKeyID: "hsm-slot-b",
		OldFingerprint:	"fp-old", OpenedBy: "ops-a",
		AuthKind:	"mem", Profile: "mem", KeySource: "kms",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGoldenCutOld(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "keyrotatefence", "cut_old_failclosed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Ticket		string	`json:"ticket"`
		OldKeyID	string	`json:"old_key_id"`
		Phase		string	`json:"phase"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	eng := keyrotatefence.NewEngine(keyrotatefence.NewMemStore(), nil)
	mustArm(t, eng)
	w, err := eng.Cut(context.Background(), keyrotatefence.CutReq{TicketID: g.Ticket, Actor: "ops-b"})
	if err != nil {
		t.Fatal(err)
	}
	if string(w.Phase) != g.Phase {
		t.Fatalf("phase %s want %s", w.Phase, g.Phase)
	}
	if err := eng.AssertUse(context.Background(), keyrotatefence.UseReq{TicketID: g.Ticket, KeyID: g.OldKeyID}); !errors.Is(err, keyrotatefence.ErrOldKeyRetired) {
		t.Fatalf("golden old use: %v", err)
	}
}
