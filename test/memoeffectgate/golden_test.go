package memoeffectgate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/memoeffectgate"
)

type goldenCase struct {
	Name		string			`json:"name"`
	Description	string			`json:"description"`
	Setup		map[string]any		`json:"setup"`
	Actions		[]map[string]any	`json:"actions"`
	Expected	map[string]any		`json:"expected"`
}

func testdataRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(wd, "testdata", "memoeffectgate")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("testdata/memoeffectgate not found")
	return ""
}

func TestGoldenDriver(t *testing.T) {
	root := testdataRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "case_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 {
		t.Fatalf("need >=3 goldens, got %d", len(files))
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var gc goldenCase
			if err := json.Unmarshal(b, &gc); err != nil {
				t.Fatal(err)
			}
			runGolden(t, gc)
		})
	}
}

func runGolden(t *testing.T, gc goldenCase) {
	t.Helper()
	eng, book, cred, mux := testEng(t)
	chain := str(gc.Setup, "chain", "ETH")
	hash := str(gc.Setup, "tx_hash", "0xhash")
	memo := str(gc.Setup, "memo", "memo")
	asset := str(gc.Setup, "asset", "ETH")
	tk := pendingTicket(book, chain, hash, memo)
	if auto, _ := gc.Setup["auto_claim"].(bool); auto {
		if _, err := mux.SeedAutoClaim(chain, hash, 0, "auto"); err != nil {
			t.Fatal(err)
		}
	}
	if n, ok := gc.Setup["apply_fail_times"].(float64); ok && n > 0 {
		cred.FailApplyTimes(int(n),
			errApplyBoom)
	}
	var lastApprove memoeffectgate.ApproveMemoWithEffectResp
	var lastDecide memoeffectgate.DecideMemoGuardedResp
	var lastErr error
	for _, act := range gc.Actions {
		switch strings.ToLower(str(act, "op", "")) {
		case "approve":
			lastApprove, lastErr = eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
				TicketID:	tk.ID, Asset: asset, Actor: "ops", Idem: "g-" + tk.ID,
			})
		case "decide":
			lastDecide, lastErr = eng.DecideMemoGuarded(context.Background(), memoeffectgate.DecideMemoGuardedReq{
				TicketID:	tk.ID, Status: str(act, "status", "rejected"), Actor: "ops",
				ForceReverse:	boolOf(act, "force_reverse"),
				Quarantine:	boolOf(act, "quarantine"),
				Reason:		str(act, "reason", "golden"),
			})
		default:
			t.Fatalf("unknown op %v", act["op"])
		}
	}
	if want := str(gc.Expected, "error", ""); want != "" {
		if lastErr == nil {
			t.Fatalf("want error %s", want)
		}
		if !strings.Contains(lastErr.Error(), want) && !errorName(lastErr, want) {
			t.Fatalf("err %v want %s", lastErr, want)
		}
	} else if lastErr != nil && !boolOf(gc.Expected, "allow_error") {
		t.Fatalf("unexpected %v", lastErr)
	}
	if st := str(gc.Expected, "ticket_status", ""); st != "" {
		got := lastApprove.Ticket.Status
		if got == "" {
			got = lastDecide.Ticket.Status
		}
		fresh, _ := book.GetMemoTicket(context.Background(), tk.ID)
		if fresh.Status != st && got != st {
			t.Fatalf("ticket %s want %s", fresh.Status, st)
		}
	}
	if boolOf(gc.Expected, "half_cut") && !lastApprove.Fence.HalfCut {
		t.Fatalf("expected half_cut %#v", lastApprove.Fence)
	}
	if n, ok := gc.Expected["credit_count"].(float64); ok {
		if lastApprove.Fence.CreditCount != int(n) {
			t.Fatalf("credit_count %d", lastApprove.Fence.CreditCount)
		}
	}
	if boolOf(gc.Expected, "proof") {
		if lastApprove.HalfProof == nil && lastDecide.Proof == nil {
			t.Fatal("expected proof")
		}
		if lastDecide.Proof != nil && lastDecide.Proof.SilentKeep {
			t.Fatal("silent keep")
		}
	}
	if boolOf(gc.Expected, "ok") && !lastApprove.OK && !lastDecide.OK {
		t.Fatal("expected ok")
	}
}

func str(m map[string]any, k, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func boolOf(m map[string]any, k string) bool {
	if m == nil {
		return false
	}
	v, _ := m[k].(bool)
	return v
}

func errorName(err error, name string) bool {
	switch name {
	case "auto_claim":
		return errors.Is(err, memoeffectgate.ErrAutoClaimOccupied)
	case "apply":
		return errors.Is(err, memoeffectgate.ErrApplyFailed,
		)
	case "reject_after_credit":
		return errors.Is(err, memoeffectgate.ErrRejectAfterCredit)
	case "occupied":
		return errors.Is(err, memoeffectgate.ErrOutputOccupied)
	default:
		return strings.Contains(err.Error(), name)
	}
}
