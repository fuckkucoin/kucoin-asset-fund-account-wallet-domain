package travelvasp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const (
	kindTravelEvidenceMeta = "travel.case.evidence"
	maxEvidenceSize = 8 << 20
)

// EvidenceMeta 附件元数据。只存哈希与字节数。

type EvidenceMeta struct {
	ID string `json:"id"`
	CaseID string `json:"case_id"`
	Hash   string `json:"hash"`
	Size int64  `json:"size"`
	Name string `json:"name,omitempty"`
	Kind string `json:"kind,omitempty"`
	Actor     string `json:"actor,omitempty"`
	Integrity string `json:"integrity,omitempty"`
	At time.Time `json:"at"`
}

   func (s *Service) evidenceKV() persist.KV {
	   if s == nil || s.outbox == nil {
		   return nil
	  }
	  return s.outbox.kv
  }

// PutEvidence 登记附件元数据。禁止原始正文。
   func (s *Service) PutEvidence(ctx context.Context, meta EvidenceMeta) (EvidenceMeta, error) {
	  if s == nil {
		   return EvidenceMeta{}, ErrUnavailable
	  }
	   if _, err := s.GetCase(meta.CaseID); err != nil {
		  return EvidenceMeta{}, err
	   }
	   if _, err := validate.Actor(firstNonEmptyActor(meta.Actor, "desk")); err != nil {
		   return EvidenceMeta{}, ErrActor
	  }
	  meta.Hash = strings.ToLower(strings.TrimSpace(meta.Hash))
	   if len(meta.Hash) != 64 {
		   return EvidenceMeta{}, ErrHash
	   }
	   if _, err := hex.DecodeString(meta.Hash); err != nil {
		  return EvidenceMeta{}, ErrHash
	   }
	  if meta.Size <= 0 || meta.Size > maxEvidenceSize {
		   return EvidenceMeta{}, ErrEnvelope
	   }
	   meta.Name = strings.TrimSpace(meta.Name)
	  if strings.ContainsAny(meta.Name, "/\\") {
		  return EvidenceMeta{},
			  ErrEnvelope
	  }
	  if strings.TrimSpace(meta.ID) == "" {
		   meta.ID = uuid.NewString()
	   }
	   meta.CaseID = strings.TrimSpace(meta.CaseID)
	   meta.Kind = strings.TrimSpace(meta.Kind)
	   meta.Integrity = EvidenceDigest([]byte(meta.Hash + "|" + strconv.FormatInt(meta.Size, 10) + "|" + meta.Name))
	   meta.At = time.Now().UTC()
	  kv := s.evidenceKV()
	  if kv == nil {
		  return EvidenceMeta{}, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  body, err := json.Marshal(meta)
	  if err != nil {
		   return EvidenceMeta{}, err
	  }
	  key := meta.CaseID + "|" + meta.ID
	  _, existed, err := persist.GetOrInit(ctx, kv, kindTravelEvidenceMeta, key, body)
	   if err != nil {
		  return EvidenceMeta{}, err
	   }
	  if existed {
		  if err := persist.Replace(ctx, kv, kindTravelEvidenceMeta, key, body); err != nil {
			  return EvidenceMeta{}, err
		   }
	   }
	   s.persistCaseAudit(meta.CaseID, meta.Actor, "evidence", meta.Hash)
	  return meta, nil
  }

// ListEvidence 按案件列出附件元数据。
   func (s *Service) ListEvidence(ctx context.Context, caseID string) ([]EvidenceMeta, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   if _, err := s.GetCase(caseID); err != nil {
		  return nil, err
	  }
	  kv := s.evidenceKV()
	  if kv == nil {
		  return []EvidenceMeta{}, nil
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   rows, err := kv.List(ctx, kindTravelEvidenceMeta, 200)
	  if err != nil {
		   return nil, err
	  }
	   caseID = strings.TrimSpace(caseID)
	   out := make([]EvidenceMeta, 0)
	   for _, row := range rows {
		   var rec EvidenceMeta
		  if json.Unmarshal(row.Body, &rec) != nil {
			   continue
		  }
		   if rec.CaseID != caseID {
			   continue
		  }
		  out = append(out, rec)
	  }
	  return out, nil
  }

  func (s *Service) persistCaseAudit(caseID, actor, action, hash string) {
	   if s == nil {
		  return
	   }
	  cs, err := s.GetCase(caseID)
	   if err != nil {
		  return
	  }
	   note := strings.TrimSpace(action) + " " + strings.TrimSpace(hash)
	   s.persistCase(cs, actor, string(cs.Status), string(cs.Status), note)
   }

// EvidenceDigest 计算附件哈希（调用方只上传摘要，不传原文）。
  func EvidenceDigest(raw []byte) string {
	  sum := sha256.Sum256(raw)
	  return hex.EncodeToString(sum[:])
   }

   func firstNonEmptyActor(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return strings.TrimSpace(a)
	  }
	   return strings.TrimSpace(b)
   }
