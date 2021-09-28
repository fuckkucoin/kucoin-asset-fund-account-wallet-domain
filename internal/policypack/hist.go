package policypack

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// HistFixtures 历史回放夹具。新增 replay_hist_*.json 会自动进入 CI。

  func HistFixtures(dir string) ([]string, error) {
	  if strings.TrimSpace(dir) == "" {
		  return nil, os.ErrNotExist
	  }
	  ents, err := os.ReadDir(dir)
	  if err != nil {
		   return nil, err
	   }
	  out := make([]string, 0, len(ents))
	   for _, e := range ents {
		  if e.IsDir() {
			  continue
		   }
		   name := e.Name()
		  if !strings.HasSuffix(name, ".json") {
			   continue
		   }
		  if strings.HasPrefix(name, "replay_hist_") || name == "replay.json" {
			   out = append(out, filepath.Join(dir, name))
		   }
	   }
	   sort.Strings(out)
	  return out, nil
   }

  func LoadHistDir(dir string) ([]ReplayFile, error) {
	   paths, err := HistFixtures(dir)
	  if err != nil {
		  return nil, err
	  }
	   out := make([]ReplayFile,
		  0, len(paths))
	   for _, p := range paths {
		  f, err := LoadReplay(p)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, f)
	  }
	  return out, nil
  }
