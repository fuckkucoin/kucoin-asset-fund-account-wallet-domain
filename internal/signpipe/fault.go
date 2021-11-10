package signpipe

import (
	"fmt"
	"strings"
	"time"
)

var (
	ErrUnavailable = fmt.Errorf("sign pipeline unavailable")
	ErrActor       = fmt.Errorf("sign pipeline actor invalid")
	ErrFence       = fmt.Errorf("sign pipeline fence stale")
	ErrExpired     = fmt.Errorf("sign pipeline expired")
	ErrDual        = fmt.Errorf("sign pipeline dual control")
)

type FaultKind string

const (
	FaultSigner FaultKind = "signer"
	FaultExpire FaultKind = "expire"
	FaultFence FaultKind = "fence"
	FaultActor FaultKind = "actor"
)

type FaultReport struct {
	Kind  FaultKind `json:"kind"`
	State State     `json:"state"`
	JobID string    `json:"job_id"`
	Note string `json:"note,omitempty"`
	Signed bool `json:"signed,omitempty"`
}

// DualOpen 三人开单。同一人兼岗直接拒。

  func (p *Pipe) DualOpen(req, appr, sign, movement, chain, to, amount string) (*Job, error) {
	  if p == nil {
		   return nil, ErrUnavailable
	   }
	   if err := distinctActors(req, appr, sign); err != nil {
		  return nil, err
	  }
	  return p.Open(req,
		  movement, chain, to, amount)
   }

  func distinctActors(req, appr, sign string) error {
	  req, appr, sign = strings.TrimSpace(req), strings.TrimSpace(appr), strings.TrimSpace(sign)
	  if err := actorOK(req); err != nil {
		  return ErrActor
	   }
	  if err := actorOK(appr); err != nil {
		   return ErrActor
	  }
	  if err := actorOK(sign); err != nil {
		   return ErrActor
	  }
	   if strings.EqualFold(req, appr) || strings.EqualFold(req, sign) || strings.EqualFold(appr, sign) {
		  return ErrDual
	   }
	   return nil
  }

// InjectFault 在已批准单上注入故障。signer 失败换围栏；过期不得再签。
   func (p *Pipe) InjectFault(id, actor string, epoch int64, token string, kind FaultKind, signer Signer) (FaultReport, error) {
	   if p == nil {
		   return FaultReport{}, ErrUnavailable
	  }
	  j, err := p.Get(id)
	  if err != nil {
		   return FaultReport{}, err
	   }
	  switch kind {
	  case FaultActor:
		  if _, err := p.Sign(id, j.RequestedBy, epoch, token, signer); err == nil {
			   return FaultReport{Kind: kind, JobID: id, State: j.State}, ErrDual
		  }
		  return FaultReport{Kind: kind, JobID: id, State: j.State, Note: "actor refused"}, nil
	  case FaultExpire:
		  p.mu.Lock()
		  if cur, ok := p.rows[id]; ok {
			  cur.ExpiresAt = p.clock().Add(-time.Second)
		  }
		   p.mu.Unlock()
		  if _, err := p.Sign(id, actor, epoch, token, signer); err == nil {
			  return FaultReport{Kind: kind, JobID: id}, ErrExpired
		  }
		   got, _ := p.Get(id)
		   return FaultReport{Kind: kind, JobID: id, State: got.State, Note: "expired"}, nil
	   case FaultFence:
		   if _, err := p.Sign(id, actor, epoch+9, "stale", signer); err == nil {
			  return FaultReport{Kind: kind, JobID: id}, ErrFence
		  }
		   return FaultReport{Kind: kind, JobID: id, State: j.State, Note: "stale fence"}, nil
	  default:
		   fail := func(string) (string, error) { return "", fmt.Errorf("signer fault") }
		  if _, err := p.Sign(id, actor, epoch, token, fail); err == nil {
			   return FaultReport{Kind: FaultSigner, JobID: id}, fmt.Errorf("signer fault must fail")
		  }
		   failed, err := p.Get(id)
		  if err != nil {
			   return FaultReport{}, err
		   }
		  if signer == nil {
			  return FaultReport{Kind: FaultSigner, JobID: id, State: failed.State, Note: failed.Note}, nil
		   }
		  got,
			   err := p.RetrySign(failed.ID, actor, failed.Fence.Epoch, failed.Fence.Token, signer)
		   if err != nil {
			   return FaultReport{Kind: FaultSigner, JobID: id, State: failed.State, Note: err.Error()}, err
		  }
		  return FaultReport{Kind: FaultSigner, JobID: got.ID, State: got.State, Signed: true}, nil
	   }
   }

// ApproveSign 换人复核后第三人签。canonical 必须对得上。

  func (p *Pipe) ApproveSign(id, appr, sign string, epoch int64, token, movement, chain, to, amount string, signer Signer) (*Job, error) {
	  if p == nil {
		  return nil, ErrUnavailable
	   }
	  job, err := p.Approve(id, appr, epoch, token, movement, chain, to, amount)
	  if err != nil {
		   return nil, err
	   }
	  return p.Sign(job.ID, sign, job.Fence.Epoch, job.Fence.Token, signer)
  }
