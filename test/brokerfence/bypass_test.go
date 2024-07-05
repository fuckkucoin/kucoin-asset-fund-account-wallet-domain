package brokerfence_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/brokerfence"
)

  func TestBypassDetector_Detect(t *testing.T) {
	   tests := []struct {
		  name    string
		   leaseID string
		  subID   string
		   masterID string
		   asset    string
		   amount   string
		  kind     brokerfence.BypassKind
		   wantErr  bool
	   }{
		   {
			   name:    "正常检测 credit_master 旁路",
			   leaseID: "lease-001",
			   subID:   "sub-001",
			   masterID: "master-001",
			  asset:    "USDT",
			   amount: "200",
			  kind: brokerfence.BypassKindCreditMaster,
			  wantErr: false,
		  },
		   {
			  name:    "正常检测 to_sub 旁路",
			   leaseID: "lease-002",
			   subID:    "sub-002",
			  masterID: "master-002",
			   asset:    "BTC",
			   amount: "0.5",
			  kind: brokerfence.BypassKindToSub,
			  wantErr: false,
		   },
		  {
			   name:    "leaseID 空时返回错误",
			  leaseID: "",
			   subID:  "sub-003",
			  asset:  "USDT",
			   amount: "100",
			   kind: brokerfence.BypassKindToMaster,
			  wantErr: true,
		   },
	  }
	   for _, tc := range tests {
		   t.Run(tc.name, func(t *testing.T) {
			   store := brokerfence.NewMemBypassStore()
			   d := brokerfence.NewBypassDetector(store)
			  bc, err := d.Detect(tc.leaseID, tc.subID, tc.masterID, tc.asset, tc.amount, tc.kind)
			   if tc.wantErr {
				   if err == nil {
					   t.Error("期望错误，实际 nil")
				  }
				  return
			   }
			  if err != nil {
				  t.Errorf("意外错误: %v", err)
				  return
			   }
			   if bc.State != brokerfence.BypassStateOpen {
				   t.Errorf("初始状态期望 open，实际 %s", bc.State)
			  }
			   if bc.HangingAmt != tc.amount {
				   t.Errorf("挂账金额期望 %s，实际 %s", tc.amount, bc.HangingAmt)
			   }
		  })
	   }
   }

  func TestBypassDetector_DualApproval(t *testing.T) {
	  store := brokerfence.NewMemBypassStore()
	  d := brokerfence.NewBypassDetector(store)
	  bc, err := d.Detect("lease-A", "sub-A", "master-A", "USDT", "300", brokerfence.BypassKindCreditMaster)
	   if err != nil {
		   t.Fatal(err)
	  }
	   caseID := bc.ID
	   tests := []struct {
		  name string
		   step int
		   approver string
		  wantState brokerfence.BypassState
		  wantErr   error
	   }{
		   {
			   name:      "第一审 step=1 成功",
			  step:      1,
			   approver:  "reviewer-alice",
			   wantState: brokerfence.BypassStateProven,
			  wantErr: nil,
		  },
		   {
			   name: "同人第二审拒绝",
			  step:      2,
			  approver:  "reviewer-alice", // 同人
			   wantState: brokerfence.BypassStateProven,
			  wantErr:   brokerfence.ErrSameReviewer,
		  },
		  {
			  name: "第二审 step=2 不同人成功",
			   step:      2,
			  approver:  "reviewer-bob",
			  wantState: brokerfence.BypassStateClosed,
			  wantErr: nil,
		  },
		  {
			  name:      "已关闭案件再次操作幂等",
			   step:      2,
			   approver:  "reviewer-carol",
			   wantState: brokerfence.BypassStateClosed,
			   wantErr: nil,
		  },
	  }
	  for _, tc := range tests {
		   t.Run(tc.name, func(t *testing.T) {
			   result, err := d.Approve(brokerfence.BypassConfirmReq{
				  CaseID: caseID,
				   Approver: tc.approver,
				  Step:     tc.step,
			   })
			  if tc.wantErr != nil {
				   if err != tc.wantErr {
					  t.Errorf("期望错误 %v，实际 %v", tc.wantErr, err)
				   }
				  return
			  }
			   if err != nil {
				  t.Errorf("意外错误: %v", err)
				   return
			   }
			   if result.State != tc.wantState {
				   t.Errorf("状态期望 %s，实际 %s", tc.wantState, result.State)
			  }
		   })
	   }
	   // 关闭后 hanging_amount 必须为 0。
	   final, _ := d.Get(caseID)
	  if final.HangingAmt != "0" {
		   t.Errorf("关闭后 hanging_amount 期望 0，实际 %s", final.HangingAmt)
	   }
  }

   func TestBypassDetector_HasOpenCases(t *testing.T) {
	   store := brokerfence.NewMemBypassStore()
	  d := brokerfence.NewBypassDetector(store)
	   subID := "sub-has-open"
	  // 无案件时 HasOpenCases=false。
	  has, err := d.HasOpenCases(subID)
	  if err != nil || has {
		  t.Errorf("无案件时期望 false，实际 has=%v err=%v", has, err)
	   }
	  // 创建案件后为 true。
	  bc, _ := d.Detect("lease-B", subID, "master-B", "USDT", "50", brokerfence.BypassKindToSub)
	   has, err = d.HasOpenCases(subID)
	  if err != nil || !has {
		  t.Errorf("有 open 案件时期望 true，实际 has=%v", has)
	   }
	   // 双人关闭后为 false。
	   _, _ = d.Approve(brokerfence.BypassConfirmReq{CaseID: bc.ID, Approver: "rev-1", Step: 1})
	   _, _ = d.Approve(brokerfence.BypassConfirmReq{CaseID: bc.ID, Approver: "rev-2", Step: 2})
	   has, _ = d.HasOpenCases(subID)
	  if has {
		   t.Error("全部关闭后期望 false，实际 true")
	   }
   }

// ──────────────────────────────────────────────
// FSM 状态机合法性
// ──────────────────────────────────────────────

   func TestFSM_LeaseTransitions(t *testing.T) {
	  now := time.Now().UTC()
	  validCases := []struct {
		  from brokerfence.LeaseState
		  to   brokerfence.LeaseState
	  }{
		   {brokerfence.LeaseStateOpen, brokerfence.LeaseStateActive},
		   {brokerfence.LeaseStateOpen, brokerfence.LeaseStateSealed},
		  {brokerfence.LeaseStateActive, brokerfence.LeaseStateLifting},
		   {brokerfence.LeaseStateActive,
			   brokerfence.LeaseStateSealed},
		   {brokerfence.LeaseStateLifting, brokerfence.LeaseStateSealed},
	   }
	  for _, c := range validCases {
		  lease := &brokerfence.FenceLease{State: c.from}
		   if err := brokerfence.TransitLease(lease, c.to, now); err != nil {
			  t.Errorf("合法转换 %s→%s 失败: %v", c.from, c.to, err)
		   }
	  }
	   // 非法转换。
	   invalidCases := []struct {
		   from brokerfence.LeaseState
		   to brokerfence.LeaseState
	   }{
		  {brokerfence.LeaseStateSealed, brokerfence.LeaseStateActive},
		   {brokerfence.LeaseStateLifting, brokerfence.LeaseStateOpen},
		   {brokerfence.LeaseStateActive, brokerfence.LeaseStateOpen},
	  }
	   for _, c := range invalidCases {
		   lease := &brokerfence.FenceLease{State: c.from}
		   if err := brokerfence.TransitLease(lease, c.to, now); err == nil {
			   t.Errorf("非法转换 %s→%s 期望错误，实际 nil", c.from, c.to)
		   }
	  }
   }
