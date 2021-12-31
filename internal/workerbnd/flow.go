package workerbnd

import "fmt"

// FakeFlow 进程内假流水：观测→占用→入账→对账顺序可跑。
type FakeFlow struct {
	Steps []string
}

  func (f *FakeFlow) Run(segs SegmentSet) error {
	   if f == nil {
		   return ErrSegment
	  }
	  if !segs.Complete() {
		   return fmt.Errorf("%w: %v", ErrSegment, segs.Missing())
	   }
	  order := []string{SegObserve, SegOccupancy, SegCredit, SegRecon}
	  for _, s := range order {
		  f.Steps = append(f.Steps, s)
	  }
	   return nil
   }
