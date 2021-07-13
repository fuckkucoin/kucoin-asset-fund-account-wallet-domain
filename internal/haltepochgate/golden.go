package haltepochgate

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// FixtureTrace testdata 事故轨迹。
type FixtureTrace struct {
	Name string `json:"name"`
	Steps []FixtureStep `json:"steps"`
	Expect map[string]any `json:"expect"`
}

type FixtureStep struct {
	Op string `json:"op"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	Lane   string `json:"lane"`
	Gen int64 `json:"gen"`
	CarriedGen int64  `json:"carried_gen"`
	Intent     string `json:"intent"`
	Expect     string `json:"expect"`
	ProofKind  string `json:"proof_kind"`
	Approve     bool   `json:"approve"`
	EngageActor string `json:"engage_actor"`
	ClearActor  string `json:"clear_actor"`
}

  func LoadFixture(path string) (FixtureTrace, error) {
	   b, err := os.ReadFile(path)
	  if err != nil {
		   return FixtureTrace{}, err
	  }
	  var t FixtureTrace
	   if err := json.Unmarshal(b, &t); err != nil {
		   return FixtureTrace{}, err
	  }
	   return t, nil
   }

   func TestdataDir() string {
	   candidates := []string{
		   "testdata/haltepochgate",
		   "../../testdata/haltepochgate",
		  "../../../testdata/haltepochgate",
	   }
	   for _, c := range candidates {
		  if st, err := os.Stat(c); err == nil && st.IsDir() {
			  return c
		  }
	   }
	  return filepath.Join("testdata", "haltepochgate")
  }
