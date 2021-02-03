package addrintel

import (
	"os"
	"time"
)

// UpdateHook 名单换版回调。参数是路径、条数、代数。
type UpdateHook func(path string, n int, gen int)

  func (l *List) OnUpdate(fn UpdateHook) {
	  if l == nil {
		  return
	   }
	   l.mu.Lock()
	  l.hook = fn
	  l.mu.Unlock()
   }

   func (l *List) Generation() int {
	  if l == nil {
		   return 0
	  }
	   l.mu.Lock()
	  defer l.mu.Unlock()
	   return l.gen
  }

// Reload 整表替换。坏文件保持旧表，失败即关，不得半更新。

   func (l *List) Reload(path string) error {
	   if l == nil {
		   return ErrUnavailable
	  }
	   next := NewList()
	   if err := next.LoadFile(path); err != nil {
		  return err
	  }
	   if err := l.persistReplace(next.rows); err != nil {
		   return err
	  }
	  l.mu.Lock()
	  l.rows = next.rows
	   l.gen++
	   gen := l.gen
	  n := len(l.rows)
	   hook := l.hook
	  l.mu.Unlock()
	   if hook != nil {
		  hook(path, n, gen)
	  }
	  return nil
   }

// WatchOnce 读文件 mtime；变了才 Reload。给 ops 定时扫用。
  func (l *List) WatchOnce(path string) (bool, error) {
	   if l == nil {
		   return false, ErrUnavailable
	  }
	   st, err := os.Stat(path)
	   if err != nil {
		   return false, ErrList
	  }
	  l.mu.Lock()
	   prev := l.mtime
	   l.mu.Unlock()
	  if !st.ModTime().After(prev) && !prev.IsZero() {
		  return false, nil
	   }
	  if err := l.Reload(path); err != nil {
		  return false, err
	  }
	   l.mu.Lock()
	   l.mtime = st.ModTime()
	   l.mu.Unlock()
	  return true, nil
  }

   func (l *List) setMtime(t time.Time) {
	  if l == nil {
		   return
	  }
	   l.mu.Lock()
	  l.mtime = t
	  l.mu.Unlock()
  }
