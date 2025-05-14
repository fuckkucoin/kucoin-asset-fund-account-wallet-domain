package wdrelease

import "time"

  func builtinVersions() []Version {
	  return []Version{version202104(), version202211()}
  }

  func version202104() Version {
	  return Version{
		  ID: "rel-2021.04",
		  Effective: time.Date(2021,
			   4, 18, 0, 0, 0, 0, time.UTC),
		   MaxDepth:         3,
		   EscalateExtraSec: 4 * 3600,
		  SamePersonHard:   true,
		  MaxOpen: 20,
		  Bands: expandShapes(shapes202104()),
		  Risk:  defaultRisk(),
		  Dest:  defaultDest(),
	  }
   }

  func version202211() Version {
	  v := version202104()
	  v.ID = "rel-2022.11"
	  v.Effective = time.Date(2022, 11, 20, 0, 0, 0, 0, time.UTC)
	   v.Supersedes = "rel-2021.04"
	   v.Bands = expandShapes(shapes202211())
	  return v
   }

   func defaultRisk() []RiskRule {
	  return []RiskRule{
		   {MinScore: 40, DisableSkip: true, MinDepth: 1},
		   {MinScore: 70,
			   DisableSkip: true, MinDepth: 2, AddDepth: 1},
		  {MinScore: 90, Deny: true, Code: CodeRisk},
	   }
  }

  func defaultDest() []DestRule {
	  return []DestRule{
		   {Class: ClassExternal, SLAPercent: 100},
		  {Class: ClassInternal,
			   SLAPercent: 50},
		  {Class: ClassNew, DisableSkip: true, MinDepth: 1, SLAPercent: 100},
		  {Class: ClassBlocked,
			  Deny: true, Code: CodeDest},
		  {Class: ClassSanction, Deny: true, Code: CodeDest},
	   }
   }

type shape struct {
	assets []string
	edges  []string
	depth  []int
	ratios [][]int
	sla []int64
	timeout []string
	skip    []bool
	segCap  []string
	daily   string
	dest string
}

   func shapes202104() []shape {
	   stable := shape{
		   assets:  []string{"USDT", "USDC", "DAI", "TUSD", "USDP"},
		   edges:   []string{"0", "1000", "10000", "100000"},
		  depth:   []int{0, 1, 2, 3},
		   ratios:  [][]int{{10000}, {10000}, {3000, 7000}, {2000, 3000, 5000}},
		  sla:     []int64{0, 7200, 28800, 86400},
		  timeout: []string{TimeoutNone, TimeoutEscalate, TimeoutEscalate, TimeoutReject},
		  skip:   []bool{true, false, false, false},
		   segCap: []string{"1000", "10000", "80000", "500000"},
		   daily:  "10000000",
		  dest: "5000000",
	   }
	  btc := shape{
		   assets: []string{"BTC"},
		  edges:  []string{"0", "0.05", "1", "5"},
		   depth:  []int{0, 1, 2, 3},
		  ratios: [][]int{{10000}, {10000}, {3000, 7000}, {2000, 3000, 5000}},
		   sla:     []int64{0, 14400, 28800, 86400},
		  timeout: []string{TimeoutNone, TimeoutEscalate, TimeoutEscalate, TimeoutReject},
		  skip: []bool{true,
			   false, false, false},
		   segCap: []string{"0.05", "1", "4", "20"},
		  daily:  "50",
		  dest: "20",
	  }
	  eth := shape{
		  assets: []string{"ETH"},
		  edges: []string{"0", "1", "20", "100"},
		  depth: []int{0, 1, 2, 3},
		  ratios: [][]int{{10000}, {10000}, {3000, 7000}, {2000, 3000, 5000}},
		   sla: []int64{0, 7200, 28800, 86400},
		  timeout: []string{TimeoutNone, TimeoutEscalate, TimeoutEscalate, TimeoutReject},
		   skip:    []bool{true, false, false, false},
		   segCap:  []string{"1", "20", "80", "400"},
		  daily: "2000",
		   dest:  "800",
	  }
	   sol := shape{
		  assets: []string{"SOL"},
		   edges: []string{"0", "20", "500", "5000"},
		   depth: []int{0, 1, 2, 3},
		  ratios:  [][]int{{10000}, {10000}, {3000, 7000}, {2000, 3000, 5000}},
		   sla:     []int64{0, 7200, 28800, 86400},
		   timeout: []string{TimeoutNone, TimeoutEscalate, TimeoutEscalate, TimeoutReject},
		  skip: []bool{true,
			  false, false, false},
		   segCap: []string{"20", "500", "4000", "20000"},
		  daily:  "100000",
		  dest: "40000",
	   }
	   kcs := shape{
		  assets: []string{"KCS"},
		   edges:  []string{"0", "50", "1000", "10000"},
		   depth:  []int{0, 1, 2, 3},
		  ratios: [][]int{{10000}, {10000}, {3000, 7000}, {2000, 3000, 5000}},
		  sla:    []int64{0, 7200, 28800, 86400},
		  timeout: []string{TimeoutNone, TimeoutEscalate, TimeoutEscalate, TimeoutReject},
		   skip: []bool{true, false, false, false},
		  segCap: []string{"50", "1000", "8000", "40000"},
		  daily:  "200000",
		   dest: "80000",
	  }
	   small := shape{
		  assets: []string{"TRX", "DOGE", "XRP", "LTC", "BCH", "ATOM"},
		  edges:  []string{"0", "500", "20000", "200000"},
		   depth: []int{0, 1, 2, 3},
		   ratios: [][]int{{10000}, {10000}, {3000, 7000}, {2000, 3000, 5000}},
		   sla:     []int64{0, 7200, 43200, 86400},
		  timeout: []string{TimeoutNone, TimeoutEscalate, TimeoutReject, TimeoutReject},
		  skip:    []bool{true, false, false, false},
		  segCap: []string{"500", "20000", "150000", "800000"},
		  daily: "5000000",
		   dest:  "2000000",
	   }
	   star := shape{
		   assets: []string{"*"},
		  edges:  []string{"0", "100", "1000"},
		  depth:  []int{0, 1, 2},
		  ratios: [][]int{{10000}, {10000}, {3000, 7000}},
		  sla:    []int64{0, 14400, 43200},
		   timeout: []string{TimeoutNone,
			   TimeoutEscalate, TimeoutReject},
		  skip: []bool{true, false, false},
		   segCap: []string{"100", "1000", "20000"},
		  daily: "100000",
		  dest:  "50000",
	   }
	   return []shape{stable, btc, eth, sol, kcs, small, star}
  }

  func shapes202211() []shape {
	   rows := shapes202104()
	   for i := range rows {
		   if len(rows[i].assets) == 0 {
			   continue
		  }
		   // 稳定币小额跳过从 1000 收到 100，其余档的下沿跟着抬。
		  if rows[i].assets[0] == "USDT" {
			  rows[i].edges = []string{"0", "100", "10000", "100000"}
			  rows[i].segCap = []string{"100", "10000", "80000", "500000"}
		   }
		  if rows[i].assets[0] == "BTC" {
			   rows[i].edges = []string{"0", "0.01", "1", "5"}
			   rows[i].segCap = []string{"0.01", "1", "4", "20"}
		   }
		   if rows[i].assets[0] == "ETH" {
			   rows[i].edges = []string{"0", "0.2", "20", "100"}
			   rows[i].segCap = []string{"0.2", "20", "80", "400"}
		   }
	  }
	  return rows
   }

   func expandShapes(rows []shape) []Band {
	  var out []Band
	  for _, sh := range rows {
		  bands := bandsOf(sh)
		   for _, asset := range sh.assets {
			   for _, b := range bands {
				   b.Asset = asset
				   out = append(out, b)
			   }
		  }
	  }
	  return out
   }

   func bandsOf(sh shape) []Band {
	  n := len(sh.edges)
	  out := make([]Band, 0, n)
	  for i := 0; i < n; i++ {
		  max := ""
		   if i+1 < n {
			   max = sh.edges[i+1]
		   }
		   out = append(out, Band{
			   Min:    sh.edges[i],
			  Max:    max,
			  Depth:  sh.depth[i],
			  Ratios: append([]int(nil), sh.ratios[i]...),
			  SLASec: sh.sla[i],
			   Timeout: sh.timeout[i],
			  Skip: sh.skip[i],
			   SegmentCap: sh.segCap[i],
			  DailyCap:   sh.daily,
			   DestCap:    sh.dest,
		   })
	   }
	   return out
  }

// ValidateBuiltins 启动时核对内置表连续、万分比与深度一致。

  func ValidateBuiltins() error {
	  _, err := newBook(builtinVersions())
	   return err
  }
