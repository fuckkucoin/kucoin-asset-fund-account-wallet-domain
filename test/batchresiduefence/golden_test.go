package batchresiduefence_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
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
		p := filepath.Join(wd, "testdata", "batchresiduefence")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("testdata/batchresiduefence not found")
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
	ctx := context.Background()
	view := batchresiduefence.NewStaticViewer()
	batch := str(gc.Setup, "batch_id", "b1")
	idem := str(gc.Setup, "item_idem", "i1")
	user := str(gc.Setup, "user_id", "u-1")
	asset := str(gc.Setup, "asset", "USDT")
	amt := str(gc.Setup, "amount", "1")
	phase := batchresiduefence.ItemPhase(str(gc.Setup, "phase", "failed"))
	view.Put(batchresiduefence.BatchItemView{BatchID: batch, ItemIdem: idem, Phase: phase, UserID: user, Asset: asset, Amount: amt})
	eng := testEng(t, view)
	var fence batchresiduefence.ResidueFence
	var last batchresiduefence.RetryWithFenceResp
	var lastErr error
	if phase == batchresiduefence.PhaseSubmitted || phase == batchresiduefence.PhaseSkipped {
		view.Put(batchresiduefence.BatchItemView{BatchID: batch, ItemIdem: idem, Phase: batchresiduefence.PhaseFailed, UserID: user, Asset: asset, Amount: amt})
		f, err := eng.IssueFence(ctx, batchresiduefence.IssueFenceReq{BatchID: batch, ItemIdem: idem, Actor: "ops-prep", UserID: user, Asset: asset, Amount: amt})
		if err == nil {
			fence = f
		}
		view.Put(batchresiduefence.BatchItemView{BatchID: batch, ItemIdem: idem, Phase: phase, UserID: user, Asset: asset, Amount: amt})
	}
	if seed, _ := gc.Setup["seed_slot"].(bool); seed {
		fence, lastErr = eng.IssueFence(ctx, batchresiduefence.IssueFenceReq{BatchID: batch, ItemIdem: idem, Actor: "ops-seed", UserID: user, Asset: asset, Amount: amt})
		if lastErr != nil {
			t.Fatalf("seed issue: %v", lastErr)
		}
		_, lastErr = eng.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: batch, ItemIdem: idem, FenceID: fence.ID, Actor: "ops-seed", AttemptID: "att-win"})
		if lastErr != nil {
			t.Fatalf("seed freeze: %v", lastErr)
		}
	}
	for _, act := range gc.Actions {
		op := strAny(act, "op", "")
		actor := strAny(act, "actor", "ops-a")
		switch op {
		case "issue_fence":
			f,
				err := eng.IssueFence(ctx, batchresiduefence.IssueFenceReq{BatchID: batch, ItemIdem: idem, Actor: actor, UserID: user, Asset: asset, Amount: amt})
			if err == nil {
				fence = f
			}
			lastErr = err
		case "retry", "retry_resume":
			fid := fence.ID
			resume := op == "retry_resume"
			att := "att-" + actor
			last, lastErr = eng.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{
				BatchID:	batch, ItemIdem: idem, FenceID: fid, Actor: actor, AttemptID: att, ResumeOnly: resume,
			})
		case "retry_second_leg":
			other := batchresiduefence.ResidueFence{
				ID:	"golden-loser-" + idem, BatchID: batch, ItemIdem: idem,
				FreezeFingerprint:	fence.FreezeFingerprint, Generation: fence.Generation + 1,
				TTL:		fence.TTL,
				IssuedAt:	fence.IssuedAt, ExpireAt: fence.ExpireAt,
				State:	batchresiduefence.FenceIssued, Actor: actor, UserID: user, Asset: asset, Amount: amt,
			}
			if err := eng.store.SaveFence(ctx, other); err != nil {
				t.Fatal(err)
			}
			last, lastErr = eng.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{
				BatchID:	batch, ItemIdem: idem, FenceID: other.ID, Actor: actor, AttemptID: "att-loser",
			})
		case "retry_no_fence":
			last, lastErr = eng.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: batch, ItemIdem: idem, Actor: actor})
		case "submit_half_cut":
			ref := strAny(act, "freeze_ref", "frz")
			sa, err := eng.SubmitAssert(ctx, batchresiduefence.SubmitAssertReq{
				BatchID:	batch, ItemIdem: idem, Actor: actor, PhaseAfter: batchresiduefence.PhaseFailed,
				HalfCut:	true, CutReason: "interrupt", FreezeRef: ref, Amount: amt, UserID: user, Asset: asset,
			})
			lastErr = err
			if sa.Fence != nil {
				fence = *sa.Fence
			}
		}
	}
	wantAllow, _ := gc.Expected["allowed"].(bool)
	if last.Allowed != wantAllow {
		t.Fatalf("allowed got %v want %v err=%v reason=%s", last.Allowed, wantAllow, lastErr, last.Reason)
	}
	if want, ok := gc.Expected["reason"].(string); ok && want != "" && last.Reason != want && lastErr == nil {
		if last.Reason != want {
			t.Fatalf("reason got %s want %s", last.Reason, want)
		}
	}
	if want, _ := gc.Expected["dup_proof"].(bool); want && last.DupProof == nil {
		if !errors.Is(lastErr, batchresiduefence.ErrDupFreeze) {
			t.Fatalf("want dup proof, err=%v", lastErr)
		}
	}
	if want, _ := gc.Expected["resume_only"].(bool); want && !last.ResumeOnly {
		t.Fatalf("want resume_only")
	}
	if want, _ := gc.Expected["second_full_freeze"].(bool); !want && last.Allowed && last.ResumeOnly {
		// 续冻成功即未二次全额冻结
	}
	if hs, ok := gc.Expected["half_state"].(string); ok && hs != "" {
		if last.Half == nil || string(last.Half.State) != hs {
			t.Fatalf("half state got %#v want %s", last.Half, hs)
		}
	}
	if et, ok := gc.Expected["error"].(string); ok {
		switch et {
		case "dup_freeze":
			if !errors.Is(lastErr, batchresiduefence.ErrDupFreeze) {
				t.Fatalf("want dup, got %v", lastErr)
			}
		case "submitted":
			if !errors.Is(lastErr, batchresiduefence.ErrItemSubmitted) {
				t.Fatalf("want submitted, got %v", lastErr)
			}
		case "skipped":
			if !errors.Is(lastErr, batchresiduefence.ErrItemSkipped) {
				t.Fatalf("want skipped, got %v", lastErr)
			}
		case "fail_closed":
			if !errors.Is(lastErr, batchresiduefence.ErrFailClosed) {
				t.Fatalf("want fail closed, got %v", lastErr)
			}
		}
	}
}

func str(m map[string]any, k, def string) string {
	if v, ok := m[k].(string); ok && v != "" {
		return v
	}
	return def
}

func strAny(m map[string]any,
	k, def string) string {
	return str(m, k, def)
}
