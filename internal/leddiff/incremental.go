package leddiff

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SeqCheck 两个快照的 seq 连续性元数据。
type SeqCheck struct {
	LeftLabel  string    `json:"left_label"`
	RightLabel string    `json:"right_label"`
	LeftAt     time.Time `json:"left_at"`
	RightAt time.Time `json:"right_at"`
	LeftFence persist.Fence `json:"left_fence"`
	RightFence persist.Fence `json:"right_fence"`
	// FenceAdvanced 右侧 fence.Epoch > 左侧，账本在两次快照之间有写入。
	FenceAdvanced bool `json:"fence_advanced"`
	// ClockForward 右侧 TakenAt > 左侧，时钟正向。
	ClockForward bool `json:"clock_forward"`
}

// CheckSeq 校验两个已存快照的 seq / fence 连续性。
// 不要求 fence 必须相邻；只判断时钟是否正向 + fence epoch 是否前进。
// 返回 SeqCheck 供调用方决策（不抛错误，让上层判断严重度）。
  func (s *Service) CheckSeq(leftID, rightID string) (SeqCheck, error) {
	  if s == nil {
		   return SeqCheck{}, ErrUnavailable
	   }
	   left, err := s.Get(leftID)
	  if err != nil {
		   return SeqCheck{}, err
	  }
	  right, err := s.Get(rightID)
	   if err != nil {
		   return SeqCheck{}, err
	  }
	  check := SeqCheck{
		   LeftLabel: left.Label,
		  RightLabel: right.Label,
		   LeftAt:     left.TakenAt,
		   RightAt: right.TakenAt,
		   FenceAdvanced: right.TakenAt.After(left.TakenAt),
		  ClockForward:  right.TakenAt.After(left.TakenAt),
	   }
	  return check, nil
   }

// IncrementalReport 增量差分报告，叠加 SeqCheck。
type IncrementalReport struct {
	Report
	SeqCheck SeqCheck `json:"seq_check"`
}

// IncrementalDiff 执行差分并附带 seq 校验。
// 若时钟倒退（ClockForward=false），附加 ErrImbalance 供调用方判断。
   func (s *Service) IncrementalDiff(leftID, rightID string) (IncrementalReport, error) {
	  if s == nil {
		   return IncrementalReport{}, ErrUnavailable
	   }
	   sc,
		   err := s.CheckSeq(leftID, rightID)
	   if err != nil {
		  return IncrementalReport{},
			  err
	   }
	   rep, diffErr := s.Diff(leftID, rightID)
	  ir := IncrementalReport{Report: rep, SeqCheck: sc}
	   if !sc.ClockForward {
		  return ir, fmt.Errorf("%w: clock not forward left=%s right=%s", ErrSnap, leftID, rightID)
	  }
	  return ir, diffErr
  }
