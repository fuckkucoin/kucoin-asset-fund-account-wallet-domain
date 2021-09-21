package opsaudit_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/opsaudit"
)

func TestChainTable(t *testing.T) {
	ctx := context.Background()
	fixed := time.Date(2021, 11, 16, 9, 0, 0, 0, time.UTC)
	type row struct {
		id	string
		run	func(t *testing.T)
	}
	cases := []row{
		{"E-A01-主成功验环", func(t *testing.T) {
			c := opsaudit.NewChain(opsaudit.NewMemStore()).WithClock(func() time.Time { return fixed })
			appendRotate(t, c, "rot-e-001")
			if err := c.Verify(ctx); err != nil {
				t.Fatal(err)
			}
			comp, err := c.CheckRotate(ctx, "rot-e-001")
			if err != nil || !comp.OK {
				t.Fatalf("complete: %v %+v", err, comp)
			}
		}},
		{"E-A02-幂等重入同哈希", func(t *testing.T) {
			c := opsaudit.NewChain(opsaudit.NewMemStore()).WithClock(func() time.Time { return fixed })
			a, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "ops-a", TicketID: "t1", IdemKey: "ops-e-reenter-1", Body: "halt"})
			if err != nil {
				t.Fatal(err)
			}
			b, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "ops-a", TicketID: "t1", IdemKey: "ops-e-reenter-1", Body: "halt"})
			if err != nil {
				t.Fatal(err)
			}
			if a.Hash != b.Hash || a.Seq != 1 || b.Seq != 1 {
				t.Fatalf("idem hash/seq a=%+v b=%+v", a, b)
			}
			if _, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "ops-a", TicketID: "t1", IdemKey: "ops-e-reenter-1", Body: "other"}); !errors.Is(err, opsaudit.ErrIdemConflict) {
				t.Fatalf("conflict: %v", err)
			}
		}},
		{"E-A03-缺seal失败封闭", func(t *testing.T) {
			c := opsaudit.NewChain(opsaudit.NewMemStore())
			if _, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindKeyRotateArm, Actor: "a", TicketID: "t2", Body: "armed"}); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindKeyRotateCut, Actor: "b", TicketID: "t2", Body: "cut"}); err != nil {
				t.Fatal(err)
			}
			if _, err := c.CheckRotate(ctx, "t2"); !errors.Is(err, opsaudit.ErrIncomplete) {
				t.Fatalf("incomplete: %v", err)
			}
		}},
		{"E-A04-序号缺口失败封闭", func(t *testing.T) {
			st := opsaudit.NewMemStore()
			c := opsaudit.NewChain(st)
			if _, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "a", Body: "one"}); err != nil {
				t.Fatal(err)
			}
			rows, _ := st.List(ctx)
			rows[0].Seq = 3
			raw, _ := json.Marshal(rows)
			if err := st.Restore(ctx, raw); err != nil {
				t.Fatal(err)
			}
			if err := c.Verify(ctx); !errors.Is(err, opsaudit.ErrGap) {
				t.Fatalf("gap: %v", err)
			}
		}},
		{"E-A05-哈希篡改失败封闭", func(t *testing.T) {
			st := opsaudit.NewMemStore()
			c := opsaudit.NewChain(st)
			if _, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "a", Body: "one"}); err != nil {
				t.Fatal(err)
			}
			rows, _ := st.List(ctx)
			rows[0].Hash = "deadbeef"
			raw, _ := json.Marshal(rows)
			_ = st.Restore(ctx, raw)
			if err := c.Verify(ctx); !errors.Is(err, opsaudit.ErrChainBroken) {
				t.Fatalf("tamper: %v", err)
			}
		}},
		{"E-A06-重启后幂等保持", func(t *testing.T) {
			src := opsaudit.NewChain(opsaudit.NewMemStore()).WithClock(func() time.Time { return fixed })
			first, err := src.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "ops-a", TicketID: "t1", IdemKey: "ops-e-reenter-1", Body: "halt"})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := src.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dst := opsaudit.NewChain(opsaudit.NewMemStore())
			if err := dst.Restore(ctx, raw); err != nil {
				t.Fatal(err)
			}
			if err := dst.Verify(ctx); err != nil {
				t.Fatal(err)
			}
			again, err := dst.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "ops-a", TicketID: "t1", IdemKey: "ops-e-reenter-1", Body: "halt"})
			if err != nil {
				t.Fatal(err)
			}
			if again.Hash != first.Hash || again.Seq != 1 {
				t.Fatalf("reenter after restore: %+v", again)
			}
		}},
		{"E-A07-缺字段拒绝", func(t *testing.T) {
			c := opsaudit.NewChain(opsaudit.NewMemStore())
			if _, err := c.Append(ctx, opsaudit.AppendReq{Actor: "a", Body: "x"}); !errors.Is(err, opsaudit.ErrKindRequired) {
				t.Fatalf("kind: %v", err)
			}
			if _, err := c.Append(ctx, opsaudit.AppendReq{Kind: opsaudit.KindOpsSensitive, Actor: "a"}); !errors.Is(err, opsaudit.ErrBodyRequired) {
				t.Fatalf("body: %v", err)
			}
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, c.run)
	}
}

func appendRotate(t *testing.T, c *opsaudit.Chain, ticket string) {
	t.Helper()
	ctx := context.Background()
	for _, rec := range []opsaudit.AppendReq{
		{Kind: opsaudit.KindKeyRotateArm, Actor: "ops-a", TicketID: ticket, IdemKey: ticket + "-arm", Body: "armed"},
		{Kind: opsaudit.KindKeyRotateCut, Actor: "ops-b", TicketID: ticket, IdemKey: ticket + "-cut", Body: "cut"},
		{Kind: opsaudit.KindKeyRotateSeal, Actor: "ops-c", TicketID: ticket, IdemKey: ticket + "-seal", Body: "sealed"},
	} {
		if _, err := c.Append(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGoldenChainOK(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "opsaudit", "chain_ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Ticket	string		`json:"ticket"`
		Kinds	[]string	`json:"kinds"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	c := opsaudit.NewChain(opsaudit.NewMemStore())
	appendRotate(t, c, g.Ticket)
	if err := c.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(g.Kinds) {
		t.Fatalf("len %d want %d", len(rows), len(g.Kinds))
	}
	for i, k := range g.Kinds {
		if rows[i].Kind != k {
			t.Fatalf("kind[%d]=%s want %s", i, rows[i].Kind, k)
		}
	}
}
