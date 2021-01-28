package custody

import "strings"

// SweepCeremonyRef 归集冷腿要的投影。没有分片，也不能拿去签名。

type SweepCeremonyRef struct {
	ID     string `json:"id"`
	Phase  string `json:"phase"`
	Shares int    `json:"shares"`
	Threshold int      `json:"threshold"`
	Actors    []string `json:"actors"`
	Eligible  bool     `json:"eligible"`
	Reason string `json:"reason,omitempty"`
}

// SweepRef 按编号读取。不存在返回 ErrCeremonyNotFound，不得编造一条激活记录。
  func (b *CeremonyBook) SweepRef(id string) (SweepCeremonyRef, error) {
	  if b == nil {
		  return SweepCeremonyRef{}, ErrCeremonyNotFound
	   }
	   c, err := b.Get(strings.TrimSpace(id))
	   if err != nil {
		   return SweepCeremonyRef{}, err
	   }
	   ref := SweepCeremonyRef{
		   ID: c.ID, Phase: string(c.Phase), Shares: c.Shares, Threshold: c.Threshold,
		   Actors: append([]string{},
			   c.Actors...),
	  }
	  if c.Phase != PhaseActivate {
		   ref.Reason = "phase " + string(c.Phase)
		  return ref, nil
	   }
	  if c.Threshold < 2 || c.Shares < c.Threshold {
		  ref.Reason = "quorum"
		   return ref, nil
	   }
	  if distinctActors(c.Actors) < 2 {
		  ref.Reason = "actors"
		  return ref, nil
	  }
	   ref.Eligible = true
	  return ref, nil
   }

  func distinctActors(in []string) int {
	   seen := map[string]struct{}{}
	   for _, a := range in {
		   a = strings.ToLower(strings.TrimSpace(a))
		   if a == "" || a == "system" {
			  continue
		   }
		   seen[a] = struct{}{}
	  }
	   return len(seen)
  }
