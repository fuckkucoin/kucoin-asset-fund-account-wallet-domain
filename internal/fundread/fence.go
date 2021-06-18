package fundread

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/fundmat"
)

// RequireCaughtUp 按围栏检查水位。未指定流时，每条流都要追平自己的日志头。
  func RequireCaughtUp(wms []fundmat.MatWatermark, fence fundmat.ReadFence, now time.Time) error {
	   if fence.Expired(now) {
		   return ErrFenceExpired
	   }
	   if fence.Stream != "" {
		  wm,
			   ok := findWM(wms, fence.Stream)
		   if !ok {
			   return fmt.Errorf("%w: stream %s 无水位", ErrNotCaughtUp, fence.Stream)
		   }
		  return checkOne(wm, fence)
	   }
	  for _, wm := range wms {
		   if wm.ContiguousSeq < wm.HeadSeq {
			   return fmt.Errorf("%w: stream %s contiguous %d head %d", ErrNotCaughtUp, wm.Stream, wm.ContiguousSeq, wm.HeadSeq)
		   }
	  }
	   return nil
   }

   func checkOne(wm fundmat.MatWatermark, fence fundmat.ReadFence) error {
	  if fence.Epoch > 0 && wm.Epoch < fence.Epoch {
		  return fmt.Errorf("%w: epoch %d < %d", ErrNotCaughtUp, wm.Epoch, fence.Epoch)
	  }
	  need := fence.MinSeq
	   if need < 0 {
		  need = 0
	  }
	  if need > wm.HeadSeq {
		   return fmt.Errorf("%w: fence %d head %d", ErrNotCaughtUp, need, wm.HeadSeq)
	  }
	  if wm.ContiguousSeq < need {
		   return fmt.Errorf("%w: contiguous %d fence %d", ErrNotCaughtUp, wm.ContiguousSeq, need)
	  }
	  if need == 0 && wm.ContiguousSeq < wm.HeadSeq {
		  return fmt.Errorf("%w: contiguous %d head %d", ErrNotCaughtUp, wm.ContiguousSeq, wm.HeadSeq)
	  }
	  return nil
   }

   func findWM(wms []fundmat.MatWatermark, stream fundmat.Stream) (fundmat.MatWatermark, bool) {
	  for _, wm := range wms {
		   if wm.Stream == stream {
			  return wm, true
		  }
	  }
	   return fundmat.MatWatermark{}, false
  }
