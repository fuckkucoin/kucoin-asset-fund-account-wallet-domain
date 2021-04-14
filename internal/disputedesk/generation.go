package disputedesk

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// CaseGeneration 记录单案资金副作用世代。
// 功能：pay_gen 从 1 起，结案签发或证据校正必须递增。
type CaseGeneration struct {
	CaseID string `json:"case_id"`
	PayGen       int64  `json:"pay_gen"`
	EvidenceRoot string `json:"evidence_root"`
}

  func (s *Service) ensureGenLocked(id string) *CaseGeneration {
	  if s.gens == nil {
		  s.gens = map[string]*CaseGeneration{}
	  }
	  g, ok := s.gens[id]
	  if !ok {
		  g = &CaseGeneration{CaseID: id, PayGen: 1}
		  s.gens[id] = g
	   }
	  return g
   }

// ComputeEvidenceRoot 对证据哈希排序后做二次 SHA-256。
// 重点：不改写 EvidenceHash；空证据返回全零根，结案路径须另拦空证。
  func ComputeEvidenceRoot(rows []Evidence) string {
	   hs := make([]string, 0, len(rows))
	   for _, e := range rows {
		   h := strings.ToLower(strings.TrimSpace(e.SHA256))
		  if h == "" {
			  continue
		  }
		   hs = append(hs, h)
	   }
	  sort.Strings(hs)
	   joined := strings.Join(hs, "|")
	   sum := sha256.Sum256([]byte("disputedesk:er:" + joined))
	   return hex.EncodeToString(sum[:])
  }

  func (s *Service) refreshRootLocked(id string) string {
	  root := ComputeEvidenceRoot(s.ev[id])
	  g := s.ensureGenLocked(id)
	  g.EvidenceRoot = root
	  return root
  }

// EvidenceRoot 返回当前案件证据根。
  func (s *Service) EvidenceRoot(id string) (string,
	  error) {
	   if s == nil {
		   return "", ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  id = strings.TrimSpace(id)
	  if _, ok := s.cases[id]; !ok {
		   return "", ErrCase
	   }
	  return s.refreshRootLocked(id), nil
  }

// GenerationOf 返回案件资金世代快照。
  func (s *Service) GenerationOf(id string) (CaseGeneration, error) {
	   if s == nil {
		   return CaseGeneration{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  id = strings.TrimSpace(id)
	  if _, ok := s.cases[id]; !ok {
		  return CaseGeneration{}, ErrCase
	   }
	  g := s.ensureGenLocked(id)
	   if g.EvidenceRoot == "" {
		   s.refreshRootLocked(id)
	   }
	  return *g, nil
  }

// BumpGeneration 抬升 pay_gen 并刷新证据根。
   func (s *Service) BumpGeneration(id string) (CaseGeneration, error) {
	  if s == nil {
		  return CaseGeneration{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.bumpGenerationLocked(strings.TrimSpace(id))
   }

  func (s *Service) bumpGenerationLocked(id string) (CaseGeneration, error) {
	  if _, ok := s.cases[id]; !ok {
		   return CaseGeneration{}, ErrCase
	   }
	   g := s.ensureGenLocked(id)
	   g.PayGen++
	   s.refreshRootLocked(id)
	   return *g, nil
  }

// OverlayEvidenceHash 结案后核验校正单条证据哈希。
// 功能：替换已挂接 SHA，抬升世代；旧租约证据根立即失效。
// 风险点：不得回退 pay_gen；禁止用本接口绕过四眼结案。

   func (s *Service) OverlayEvidenceHash(id, oldSHA, newSHA, actor string) (Evidence, error) {
	  if s == nil {
		   return Evidence{}, ErrUnavailable
	   }
	  actor = strings.TrimSpace(actor)
	   if actor == "" {
		  return Evidence{}, ErrActor
	  }
	   if !hexOK(oldSHA) || !hexOK(newSHA) {
		  return Evidence{}, ErrHash
	  }
	  oldSHA = strings.ToLower(strings.TrimSpace(oldSHA))
	   newSHA = strings.ToLower(strings.TrimSpace(newSHA))
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.cases[strings.TrimSpace(id)]
	  if !ok {
		  return Evidence{}, ErrCase
	   }
	  rows := s.ev[cur.ID]
	   idx := -1
	   for i, e := range rows {
		   if strings.EqualFold(e.SHA256, oldSHA) {
			   idx = i
			  break
		  }
	  }
	  if idx < 0 {
		  return Evidence{}, ErrEvidence
	  }
	  rows[idx].SHA256 = newSHA
	   s.ev[cur.ID] = rows
	  g := s.ensureGenLocked(cur.ID)
	   g.PayGen++
	  s.refreshRootLocked(cur.ID)
	  cur.UpdatedAt = s.now()
	  s.noteLocked(cur.ID, actor, "evidence_overlay", newSHA)
	  return rows[idx].Copy(), nil
  }

// TerminalByIdem 按幂等键判断案件是否已终态，供二次 Open 拒放款。

   func (s *Service) TerminalByIdem(idem string) (bool, string, error) {
	  if s == nil {
		  return false, "", ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  id, ok := s.byIdem[strings.TrimSpace(idem)]
	  if !ok {
		   return false, "", ErrCase
	   }
	  cur := s.cases[id]
	  return cur.Status.Terminal(), cur.ID, nil
  }
