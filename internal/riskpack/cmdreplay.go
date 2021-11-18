package riskpack

import (
	"fmt"
	"path/filepath"
)

// ReplayDir 加载目录内 replay_*.json，任一套件 drift 则返回错误。
   func ReplayDir(dir string) ([]CompareReport, error) {
	  paths, err := ListFixtures(dir)
	  if err != nil {
		   return nil, err
	  }
	   if len(paths) == 0 {
		  return nil, fmt.Errorf("no risk fixtures in %s", dir)
	   }
	  out := make([]CompareReport, 0, len(paths))
	   var first error
	  for _, p := range paths {
		   f, err := LoadFixture(p)
		   if err != nil {
			   return out, err
		   }
		  e := NewEngine()
		  if err := e.ApplyCaps(f.Caps); err != nil {
			  return out, fmt.Errorf("%s: %w", filepath.Base(p), err)
		   }
		   rep, err := e.CompareReplay(f.Rows)
		   if err != nil {
			  return out, fmt.Errorf("%s: %w", filepath.Base(p), err)
		  }
		  out = append(out, rep)
		  if err := rep.Fail(); err != nil && first == nil {
			   first = fmt.Errorf("%s: %w", filepath.Base(p), err)
		   }
	  }
	   return out, first
  }
