package wdrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Fixture 一条可回放的放行场景。
type Fixture struct {
	Name   string `json:"name"`
	Now    string `json:"now"`
	Policy string `json:"policy"`
	Input Input    `json:"input"`
	Steps []StepOp `json:"steps"`
}

// StepOp 回放的一步。ok 缺省时期望成功。

type StepOp struct {
	Op string `json:"op"`
	Actor string `json:"actor,omitempty"`
	Role   string `json:"role,omitempty"`
	Amount string `json:"amount,omitempty"`
	OK     *bool  `json:"ok,omitempty"`
	Code   string `json:"code,omitempty"`
	State  string `json:"state,omitempty"`
	Seq      int    `json:"seq,omitempty"`
	SegState string `json:"seg_state,omitempty"`
	Depth    int    `json:"depth,omitempty"`
	Skip     *bool  `json:"skip,omitempty"`
	At       string `json:"at,omitempty"`
	Note     string `json:"note,omitempty"`
	Frozen   string `json:"frozen,omitempty"`
	Residual string `json:"residual,omitempty"`
}

// LoadFixture 读一份 JSON 夹具。
   func LoadFixture(path string) (Fixture, error) {
	  raw, err := os.ReadFile(path)
	   if err != nil {
		  return Fixture{}, err
	   }
	   var fx Fixture
	   if err := json.Unmarshal(raw, &fx); err != nil {
		   return Fixture{}, err
	   }
	  return fx, nil
  }

// Replay 在新台上跑完夹具。失败返回第一步对不上的原因。

   func Replay(ctx context.Context, fx Fixture) (Ticket, error) {
	   d := New()
	  if fx.Now != "" {
		   at, err := time.Parse(time.RFC3339, fx.Now)
		  if err != nil {
			  return Ticket{}, err
		  }
		   d.SetNow(at)
	  }
	   if fx.Policy != "" {
		   if err := d.Publish(fx.Policy); err != nil {
			   return Ticket{}, err
		   }
	   }
	   in := fx.Input
	  if in.Version == "" {
		  in.Version = fx.Policy
	   }
	  var cur Ticket
	   for i, st := range fx.Steps {
		   var err error
		  cur, err = applyStep(ctx, d, in, cur, st)
		   if err != nil {
			  return cur, fmt.Errorf("step %d %s: %w", i, st.Op, err)
		   }
	  }
	  return cur, nil
  }

   func applyStep(ctx context.Context, d *Desk, in Input, cur Ticket, st StepOp) (Ticket, error) {
	  wantOK := true
	   if st.OK != nil {
		  wantOK = *st.OK
	  }
	  var (
		   next Ticket
		   err  error
	   )
	  switch st.Op {
	   case "open":
		   next, err = d.Open(ctx, in)
	   case "submit":
		   actor := st.Actor
		   if actor == "" {
			  actor = in.Applicant
		  }
		  next, err = d.Submit(ctx, cur.ID, actor)
	   case "advance":
		   next,
			  err = d.Advance(ctx, cur.ID, st.Actor, st.Role, true)
	  case "reject":
		  next, err = d.Reject(ctx, cur.ID, st.Actor, st.Role, st.Code, st.Note)
	   case "freeze":
		   next, err = freezeStep(ctx, d, in, cur, st.Amount)
	  case "tick":
		   if st.At != "" {
			   at, perr := time.Parse(time.RFC3339, st.At)
			  if perr != nil {
				   return cur, perr
			   }
			  d.SetNow(at)
		   }
		   if _, err = d.Tick(ctx); err != nil {
			   break
		  }
		   next, err = d.Get(cur.ID)
	  case "expect_state":
		  next, err = d.Get(cur.ID)
		  if err != nil {
			  return cur, err
		   }
		  if next.State != st.State {
			  return next, fmt.Errorf("state %s want %s", next.State, st.State)
		   }
		  return next, nil
	   case "expect_segment":
		   next, err = d.Get(cur.ID)
		  if err != nil {
			   return cur, err
		   }
		   if err := expectSeg(next, st.Seq, st.SegState); err != nil {
			   return next, err
		   }
		  return next, nil
	  case "expect_depth":
		  next, err = d.Get(cur.ID)
		  if err != nil {
			  return cur, err
		  }
		   if next.Depth != st.Depth {
			  return next, fmt.Errorf("depth %d want %d", next.Depth, st.Depth)
		  }
		   return next, nil
	  case "expect_skip":
		  next, err = d.Get(cur.ID)
		   if err != nil {
			  return cur, err
		   }
		  if st.Skip != nil && next.Skip != *st.Skip {
			  return next, fmt.Errorf("skip %v", next.Skip)
		  }
		  return next, nil
	  case "expect_frozen":
		  next, err = d.Get(cur.ID)
		  if err != nil {
			   return cur, err
		  }
		  if next.Frozen != st.Frozen {
			  return next, fmt.Errorf("frozen %s want %s", next.Frozen, st.Frozen)
		  }
		   return next, nil
	   case "expect_code":
		   next, err = d.Get(cur.ID)
		  if err != nil {
			   return cur, err
		  }
		  if next.Code != st.Code {
			  return next, fmt.Errorf("code %s want %s", next.Code, st.Code)
		  }
		  return next, nil
	   default:
		   return cur, fmt.Errorf("unknown op %s", st.Op)
	   }
	  if wantOK && err != nil {
		   return next, err
	  }
	  if !wantOK && err == nil {
		   return next, fmt.Errorf("want failure")
	   }
	   if !wantOK && st.Code != "" && CodeOf(err) != st.Code {
		   return next, fmt.Errorf("code %s want %s", CodeOf(err), st.Code)
	   }
	   if next.ID == "" && cur.ID != "" {
		   got, gerr := d.Get(cur.ID)
		  if gerr == nil {
			   next = got
		  }
	  }
	   if st.State != "" && next.State != st.State {
		   return next, fmt.Errorf("state %s want %s", next.State, st.State)
	   }
	   return next, nil
   }

  func freezeStep(ctx context.Context, d *Desk, in Input, cur Ticket, amount string) (Ticket, error) {
	  if amount == "" {
		  amount = in.Amount
	   }
	   ask := releaseAsk(in, cur.ID, amount)
	   g, err := d.BeforeFreeze(ctx, ask)
	   if err != nil {
		  return cur, err
	   }
	  if err := d.CommitFreeze(ctx, g); err != nil {
		  _ = d.AbortFreeze(ctx, g)
		  return cur, err
	  }
	  return d.Get(cur.ID)
  }

  func expectSeg(t Ticket, seq int, state string) error {
	   for _, seg := range t.Segments {
		   if seg.Seq == seq {
			  if seg.State != state {
				   return fmt.Errorf("segment %d %s want %s", seq, seg.State, state)
			  }
			  return nil
		  }
	  }
	   return fmt.Errorf("segment %d missing", seq)
  }
