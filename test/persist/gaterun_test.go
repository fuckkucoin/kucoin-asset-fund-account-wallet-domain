package persist_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

func TestRefuseMemRegisterTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name		string
		prod		bool
		storage		string
		mem		bool
		wantRefuse	bool
	}{
		{name: "A-T01-prod-mem", prod: true, storage: "mem", mem: true, wantRefuse: true},
		{name: "A-T01-prod-bound-mem", prod: true, storage: "sql", mem: true, wantRefuse: true},
		{name: "A-T02-dev-mem", prod: false, storage: "mem", mem: true, wantRefuse: false},
		{name: "prod-sql", prod: true, storage: "sql", mem: false, wantRefuse: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := persist.RefuseMemRegister(tc.prod, tc.storage, tc.mem)
			if tc.wantRefuse && !errors.Is(err, persist.ErrMemRegister) {
				t.Fatalf("want refuse got %v", err)
			}
			if !tc.wantRefuse && err != nil {
				t.Fatalf("dev mem allowed: %v", err)
			}
		})
	}
}

func TestTableRegisterSQLSemantics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2025, 11, 18, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	t.Run("A-T03-register-get", func(t *testing.T) {
		reg := persist.NewTableRegister().WithClock(clock)
		got, err := reg.Register(ctx, "k1", "h1", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		back, err := reg.Get(ctx, "k1")
		if err != nil || back.Holder != "h1" || !back.ExpiresAt.Equal(got.ExpiresAt) {
			t.Fatalf("%+v %v", back, err)
		}
	})
	t.Run("A-T04-conflict", func(t *testing.T) {
		reg := persist.NewTableRegister().WithClock(clock)
		if _, err := reg.Register(ctx, "k1", "h1", time.Minute); err != nil {
			t.Fatal(err)
		}
		if _, err := reg.Register(ctx, "k1", "h2", time.Minute); !errors.Is(err, persist.ErrConflict) {
			t.Fatalf("want conflict got %v", err)
		}
	})
	t.Run("A-T05-renew", func(t *testing.T) {
		reg := persist.NewTableRegister().WithClock(clock)
		first, err := reg.Register(ctx, "k1", "h1", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		later := now.Add(30 * time.Second)
		reg.WithClock(func() time.Time { return later })
		got, err := reg.Renew(ctx, "k1", "h1", time.Minute)
		if err != nil || !got.ExpiresAt.After(first.ExpiresAt) {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("A-T06-release", func(t *testing.T) {
		reg := persist.NewTableRegister().WithClock(clock)
		if _, err := reg.Register(ctx, "k1", "h1", time.Minute); err != nil {
			t.Fatal(err)
		}
		if err := reg.Release(ctx, "k1", "h1"); err != nil {
			t.Fatal(err)
		}
		if _, err := reg.Get(ctx, "k1"); !errors.Is(err, persist.ErrNotFound) {
			t.Fatalf("want empty got %v", err)
		}
	})
	t.Run("A-T07-restart", func(t *testing.T) {
		shared := map[string]persist.Lease{}
		a := persist.OpenTableRegister(shared).WithClock(clock)
		want, err := a.Register(ctx, "k1", "h1", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		b := persist.OpenTableRegister(shared).WithClock(clock)
		got, err := b.Get(ctx, "k1")
		if err != nil || got.Holder != want.Holder || !got.ExpiresAt.Equal(want.ExpiresAt) {
			t.Fatalf("reload %+v %v", got, err)
		}
	})
	t.Run("A-T08-expired-restart", func(t *testing.T) {
		shared := map[string]persist.Lease{}
		a := persist.OpenTableRegister(shared).WithClock(clock)
		if _, err := a.Register(ctx, "k1", "h1", time.Second); err != nil {
			t.Fatal(err)
		}
		b := persist.OpenTableRegister(shared).WithClock(func() time.Time { return now.Add(2 * time.Second) })
		if _, err := b.Get(ctx, "k1"); !errors.Is(err, persist.ErrNotFound) {
			t.Fatalf("expired must hide: %v", err)
		}
	})
	t.Run("A-T09-list-active", func(t *testing.T) {
		reg := persist.NewTableRegister().WithClock(clock)
		if _, err := reg.Register(ctx, "live", "h1", time.Minute); err != nil {
			t.Fatal(err)
		}
		if _, err := reg.Register(ctx, "dead", "h2", time.Second); err != nil {
			t.Fatal(err)
		}
		reg.WithClock(func() time.Time { return now.Add(2 * time.Second) })
		act := reg.ListActive(ctx)
		if len(act) != 1 || act[0].Name != "live" {
			t.Fatalf("%+v", act)
		}
	})
}

func TestKVRegisterRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2025, 11, 18, 13, 0, 0, 0, time.UTC)
	kv := persist.NewMemKV()
	a := persist.NewLeases(kv).WithClock(func() time.Time { return now })
	want, err := a.Register(ctx, "gate/k", "holder-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	b := persist.NewLeases(kv).WithClock(func() time.Time { return now })
	got, err := b.Get(ctx, "gate/k")
	if err != nil || got.Holder != want.Holder {
		t.Fatalf("%+v %v", got, err)
	}
}
// temporary guard removed after the production check
