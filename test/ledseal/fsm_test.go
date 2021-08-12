package ledseal_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/ledseal"
)

// TestPlanFSM_Transitions 验证 planFSM 所有合法与非法转换。
func TestPlanFSM_Transitions(t *testing.T) {
	type tc struct {
		name		string
		fromState	ledseal.PlanState
		event		string
		expectOK	bool
		expectTo	ledseal.PlanState
	}
	cases := []tc{
		// 合法转换
		{name: "active_pause", fromState: ledseal.PlanActive, event: "pause", expectOK: true, expectTo: ledseal.PlanPaused},
		{name: "paused_activate", fromState: ledseal.PlanPaused, event: "activate", expectOK: true, expectTo: ledseal.PlanActive},
		// 非法转换（同状态重复操作）
		{name: "active_activate", fromState: ledseal.PlanActive, event: "activate", expectOK: false},
		{name: "paused_pause", fromState: ledseal.PlanPaused, event: "pause", expectOK: false},
		// 无效事件
		{name: "active_unknown", fromState: ledseal.PlanActive, event: "unknown", expectOK: false},
		{name: "paused_unknown", fromState: ledseal.PlanPaused, event: "unknown", expectOK: false},
	}
	fsm := ledseal.PlanFSM()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			to, err := fsm.Apply(tc.fromState, tc.event)
			if tc.expectOK {
				if err != nil {
					t.Errorf("want ok, got error: %v", err)
				}
				if to != tc.expectTo {
					t.Errorf("want to=%s, got %s", tc.expectTo, to)
				}
			} else {
				if err == nil {
					t.Errorf("want error, got ok (to=%s)", to)
				}
			}
		})
	}
}

// TestBatchFSM_Transitions 验证 batchFSM 所有合法与非法转换。

func TestBatchFSM_Transitions(t *testing.T) {
	type tc struct {
		name		string
		fromState	ledseal.BatchState
		event		string
		expectOK	bool
		expectTo	ledseal.BatchState
	}
	cases := []tc{
		// 合法转换
		{name: "running_seal", fromState: ledseal.BatchRunning, event: "seal", expectOK: true, expectTo: ledseal.BatchSealed},
		{name: "running_fail", fromState: ledseal.BatchRunning, event: "fail", expectOK: true, expectTo: ledseal.BatchFailed},
		// 终止状态不可再转换
		{name: "sealed_seal", fromState: ledseal.BatchSealed, event: "seal", expectOK: false},
		{name: "sealed_fail", fromState: ledseal.BatchSealed, event: "fail", expectOK: false},
		{name: "failed_seal", fromState: ledseal.BatchFailed, event: "seal", expectOK: false},
		{name: "failed_fail", fromState: ledseal.BatchFailed, event: "fail", expectOK: false},
		// 无效事件
		{name: "running_unknown", fromState: ledseal.BatchRunning, event: "unknown", expectOK: false},
	}
	fsm := ledseal.BatchFSM()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			to,
				err := fsm.Apply(tc.fromState, tc.event)
			if tc.expectOK {
				if err != nil {
					t.Errorf("want ok, got error: %v", err)
				}
				if to != tc.expectTo {
					t.Errorf("want to=%s, got %s", tc.expectTo, to)
				}
			} else {
				if err == nil {
					t.Errorf("want error, got ok (to=%s)", to)
				}
			}
		})
	}
}

// TestCanXxx 验证辅助函数。

func TestCanXxx_Functions(t *testing.T) {
	cases := []struct {
		name	string
		fn	func() bool
		want	bool
	}{
		{"CanSealBatch_running", func() bool { return ledseal.CanSealBatch(ledseal.BatchRunning) }, true},
		{"CanSealBatch_sealed", func() bool { return ledseal.CanSealBatch(ledseal.BatchSealed) }, false},
		{"CanSealBatch_failed", func() bool { return ledseal.CanSealBatch(ledseal.BatchFailed) }, false},
		{"CanFailBatch_running", func() bool { return ledseal.CanFailBatch(ledseal.BatchRunning) }, true},
		{"CanFailBatch_sealed", func() bool { return ledseal.CanFailBatch(ledseal.BatchSealed) }, false},
		{"CanActivatePlan_paused", func() bool { return ledseal.CanActivatePlan(ledseal.PlanPaused) }, true},
		{"CanActivatePlan_active", func() bool { return ledseal.CanActivatePlan(ledseal.PlanActive) }, false},
		{"CanPausePlan_active", func() bool { return ledseal.CanPausePlan(ledseal.PlanActive) }, true},
		{"CanPausePlan_paused", func() bool { return ledseal.CanPausePlan(ledseal.PlanPaused) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn()
			if got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}
