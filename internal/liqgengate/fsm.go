package liqgengate

import "fmt"

// DescribePhase 书面说明相位约束。
   func DescribePhase(p LiqPhase) string {
	  switch p {
	   case PhaseOpen:
		  return "借款已开立，尚未进入强平冻结闸"
	  case PhaseFreezing:
		   return "已推进 liq_gen 并占互斥锁，Hold 已记，等待 freeze hook"
	   case PhaseFrozen:
		   return "freeze hook 成功，可持匹配世代进入释放闸"
	  case PhaseReleasing:
		   return "释放闸已通过，等待关借收口"
	  case PhaseClosed:
		   return "关借完成，世代只读，禁止再释"
	   case PhaseHalfOpen:
		   return "freeze hook 失败半截，禁止按已强平释放"
	   default:
		   return "未知相位，fail-closed"
	   }
   }

// AssertReleaseFence 关借/释放前置：必须有世代且相位允许。
  func AssertReleaseFence(g *LiqGeneration, submitted uint64, hasGen bool) error {
	   if g == nil {
		  return ErrMissingGen
	   }
	   if !hasGen {
		  return ErrMissingGen
	  }
	  if submitted != g.Gen {
		  return ErrStaleGen
	   }
	   if g.Phase == PhaseHalfOpen {
		  return ErrHalfOpen
	  }
	  if !g.Phase.Releasable() && g.Phase != PhaseReleasing {
		  return ErrUnexpectedPhase
	  }
	  return nil
   }

// MutexKind 互斥种类说明。
   func MutexKind(kind string) string {
	  switch kind {
	  case "freeze":
		  return "FreezeWithGen 占用，Release/Close 必须失败"
	   case "release":
		  return "ReleaseWithGen 占用，Freeze 必须失败"
	  case "close":
		   return "CloseWithGen 占用，Freeze/Release 必须失败"
	  default:
		  return fmt.Sprintf("未知互斥种类 %s", kind)
	   }
   }
