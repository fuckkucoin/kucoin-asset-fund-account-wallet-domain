package signpipegate

import "time"

// Policy 运行时策略快照。
type Policy struct {
	LeaseTTL time.Duration
	MaxGenerations int
	BlockOnFault bool
	BlockOnClose  bool
	RequireFence  bool
	IsolationMark string
}

   func DefaultPolicy() Policy {
	  return Policy{
		  LeaseTTL:       DefaultLeaseTTL,
		  MaxGenerations: MaxRetryGenerations,
		  BlockOnFault:   true,
		   BlockOnClose:  true,
		   RequireFence:  false,
		   IsolationMark: IsolationMark,
	  }
   }

  func (p Policy) EffectiveTTL(reqTTL time.Duration) time.Duration {
	  if reqTTL > 0 && reqTTL < 24*time.Hour {
		  return reqTTL
	   }
	   if p.LeaseTTL > 0 {
		  return p.LeaseTTL
	   }
	  return DefaultLeaseTTL
   }

  func (p Policy) AllowGeneration(gen int64) error {
	   max := p.MaxGenerations
	   if max <= 0 {
		  max = MaxRetryGenerations
	   }
	  if gen > int64(max) {
		   return ErrRetriesExhausted
	  }
	  return nil
   }

  func (p Policy) ShouldBlockFault(kind FaultKind) bool {
	  if !p.BlockOnFault {
		   return false
	   }
	  return faultBlocksRetry(kind)
  }

   func (p Policy) ShouldBlockCloseout() bool {
	   return p.BlockOnClose
   }
