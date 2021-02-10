package collide

import "time"

// State 仲裁机状态。

type State string

const (
	StClear     State = "clear"
	StSuspect   State = "suspect"
	StBlocked   State = "blocked"
	StDesk      State = "desk"
	StEvidence  State = "evidence"
	StArbitrate State = "arbitrate"
	StReleased State = "released"
	StRejected State = "rejected"
	StExpired  State = "expired"
)

// Event 一次迁移。Actor 不能是 system，超时只接受 attrib-worker。

type Event struct {
	Kind string `json:"kind"`
	Actor string `json:"actor,omitempty"`
	User   string `json:"user,omitempty"`
	Winner string `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`
	Evidence string `json:"evidence,omitempty"`
	At time.Time `json:"at"`
}

// Case 一条碰撞案。Origin 是先占住折叠键的用户。

type Case struct {
	ID    string `json:"id"`
	Chain string `json:"chain"`
	Namespace string   `json:"namespace"`
	Fold      string   `json:"fold"`
	Raw       []string `json:"raw,omitempty"`
	Origin    string   `json:"origin"`
	Claimants []string `json:"claimants"`
	State     State    `json:"state"`
	Winner string `json:"winner,omitempty"`
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
	Evidence string    `json:"evidence,omitempty"`
	Log      []Event   `json:"log,omitempty"`
	Deadline time.Time `json:"deadline,omitempty"`
	Updated  time.Time `json:"updated"`
	Version int `json:"version"`
}

// Allows 该用户此刻能否对这个折叠键产生入账意图。
  func (c Case) Allows(user string) bool {
	   if user == "" {
		  return false
	   }
	   switch c.State {
	   case StClear, "":
		   return c.Origin != "" && user == c.Origin
	   case StReleased:
		  return c.Winner != "" && user == c.Winner
	   default:
		  return false
	   }
  }
