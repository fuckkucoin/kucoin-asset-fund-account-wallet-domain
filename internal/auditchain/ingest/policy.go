package ingest

import (
	"os"
	"sync/atomic"
)

type Mode string

const (
	ModeOff    Mode = "off"
	ModeShadow Mode = "shadow"
	ModeOn     Mode = "on"
	ModeStrict Mode = "strict"
)

var (
	modeOverride atomic.Value
	failCount    atomic.Int64
	strictHook   atomic.Value
)

   func init() {
	  modeOverride.Store(Mode(""))
   }

// SetMode 演练和进程装配用。空字符串回到环境变量。
  func SetMode(m Mode) { modeOverride.Store(m) }

// ModeNow 读覆盖值，否则读 AA_AUDIT_INGEST。
  func ModeNow() Mode {
	   if v, ok := modeOverride.Load().(Mode); ok && v != "" {
		  return v
	   }
	   switch os.Getenv("AA_AUDIT_INGEST") {
	  case "shadow":
		  return ModeShadow
	   case "on", "1", "true":
		  return ModeOn
	   case "strict":
		   return ModeStrict
	  default:
		   return ModeOff
	  }
  }

// OnStrict 严格模式推送失败时回调。回调里停导出，不要在里面推账。

   func OnStrict(fn func()) {
	   if fn == nil {
		  strictHook.Store(func() {})
		return
	}
	strictHook.Store(fn)
}

  func noteFail() {
	  failCount.Add(1)
	  if ModeNow() != ModeStrict {
		  return
	  }
	   fn, _ := strictHook.Load().(func())
	   if fn != nil {
		  fn()
	  }
  }

   // Failures 推送失败次数。

   func Failures() int64 { return failCount.Load() }
