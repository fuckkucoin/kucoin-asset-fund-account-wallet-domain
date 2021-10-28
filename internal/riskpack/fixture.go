package riskpack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FixtureFile struct {
	Name string     `json:"name,omitempty"`
	Caps []Cap      `json:"caps,omitempty"`
	Rows []ReplayIn `json:"rows"`
}

  func (e *Engine) ApplyCaps(caps []Cap) error {
	  if e == nil {
		  return ErrUnavailable
	  }
	   for _, c := range caps {
		   if err := e.Exposure().SetCap(c); err != nil {
			   return err
		  }
	   }
	  return nil
  }

   func LoadFixture(path string) (FixtureFile, error) {
	   raw, err := os.ReadFile(path)
	   if err != nil {
		  return FixtureFile{}, err
	  }
	   var f FixtureFile
	   if err := json.Unmarshal(raw, &f); err != nil {
		  return FixtureFile{}, err
	   }
	  if len(f.Rows) == 0 {
		   return FixtureFile{}, fmt.Errorf("risk fixture empty")
	  }
	   if f.Name == "" {
		  f.Name = filepath.Base(path)
	   }
	  return f, nil
  }

  func ListFixtures(dir string) ([]string, error) {
	  ents, err := os.ReadDir(dir)
	  if err != nil {
		   return nil, err
	   }
	   out := make([]string, 0, len(ents))
	  for _, e := range ents {
		  if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			  continue
		  }
		   if strings.HasPrefix(e.Name(), "replay_") {
			  out = append(out, filepath.Join(dir, e.Name()))
		   }
	  }
	  sort.Strings(out)
	  return out, nil
   }
