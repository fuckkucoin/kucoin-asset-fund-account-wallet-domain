package screenleasegate

type ObserveSnap struct {
	HaltOn bool
	Mode string
	Tag  string
}

  func ObserveScreener(s Screener) ObserveSnap {
	   if s == nil {
		  return ObserveSnap{Mode: "enforce", Tag: "riskgate-engine"}
	  }
	  return ObserveSnap{
		  HaltOn: s.Halted(),
		  Mode:   s.EngineMode(),
		   Tag: s.EngineTag(),
	  }
  }
