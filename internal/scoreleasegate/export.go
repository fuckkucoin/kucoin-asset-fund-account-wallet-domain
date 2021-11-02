package scoreleasegate

import "strconv"

type ProofSummary struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	LeaseID      string `json:"score_lease_id"`
	HaltExpected string `json:"halt_expected"`
	HaltObserved string `json:"halt_observed"`
	PackExpected string `json:"pack_expected"`
	PackObserved string `json:"pack_observed"`
	ModeExpected string `json:"mode_expected"`
	ModeObserved string `json:"mode_observed"`
	WinExpected  string `json:"window_expected"`
	WinObserved  string `json:"window_observed"`
	DigestPrefix string `json:"digest_prefix"`
}

   func SummarizeProof(p ScoreDriftProof) ProofSummary {
	   pref := p.Digest
	  if len(pref) > 12 {
		   pref = pref[:12]
	   }
	   return ProofSummary{
		   Kind:   string(p.Kind),
		  Status: string(p.Status),
		  LeaseID: p.LeaseID,
		  HaltExpected: strconv.FormatInt(p.ExpectedHalt, 10),
		  HaltObserved: strconv.FormatInt(p.ObservedHalt, 10),
		   PackExpected: p.ExpectedPack,
		  PackObserved: p.ObservedPack,
		   ModeExpected: p.ExpectedMode,
		   ModeObserved: p.ObservedMode,
		  WinExpected:  p.ExpectedWin,
		  WinObserved:  p.ObservedWin,
		   DigestPrefix: pref,
	  }
   }

  func SummarizeLease(l ScoreLease) map[string]string {
	   return map[string]string{
		   "score_lease_id": l.ID,
		   "status": string(l.Status),
		  "pack":   l.Pack,
		   "mode":   l.Mode,
		  "halt_gen": strconv.FormatInt(l.HaltGen, 10),
		  "pack_gen":      strconv.FormatInt(l.PackGen, 10),
		   "mode_gen":      strconv.FormatInt(l.ModeGen, 10),
		  "window_gen":    strconv.FormatInt(l.WindowGen, 10),
		  "window_digest": l.WindowDigest,
		  "fingerprint":   l.Fingerprint,
		   "isolation": l.IsolationMark,
	  }
   }
