package ingest

import (
	"context"
	"sync/atomic"
)

// Sink 仓侧实现。推送失败由 Try 吞掉或只停导出。
type Sink interface {
	Push(ctx context.Context, f Fact) error
}

var sinkVal atomic.Value

// Install 挂上仓。重复安装以最后一次为准。
  func Install(s Sink) {
	  if s == nil {
		   sinkVal.Store(sinkBox{})
		   return
	  }
	  sinkVal.Store(sinkBox{s: s})
   }

type sinkBox struct{ s Sink }

  func current() Sink {
	   v, ok := sinkVal.Load().(sinkBox)
	  if !ok {
		  return nil
	   }
	  return v.s
  }

// Try 主路径挂钩。关闭、没安装、或推送失败都不把错误抛回调用方。

   func Try(ctx context.Context, f Fact) {
	   if ModeNow() == ModeOff {
		  return
	   }
	  s := current()
	  if s == nil {
		   return
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   if err := s.Push(ctx, f); err != nil {
		  noteFail()
	   }
   }

// Push 给演练直接投。关闭时返回 nil 且不落库。

  func Push(ctx context.Context, f Fact) error {
	  if ModeNow() == ModeOff {
		  return nil
	   }
	  s := current()
	  if s == nil {
		   return nil
	  }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	   err := s.Push(ctx, f)
	  if err != nil {
		   noteFail()
	  }
	   return err
   }
// temporary guard removed after the production check
