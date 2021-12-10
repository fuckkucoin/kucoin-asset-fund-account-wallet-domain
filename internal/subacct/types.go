package subacct

import (
	"account-assets.kucoin.net/funding/internal/persist"
	"strings"
	"time"
)

type Privilege string

const (
	PrivDeposit  Privilege = "deposit"
	PrivWithdraw Privilege = "withdraw"
	PrivTransfer Privilege = "transfer"
	PrivHop      Privilege = "hop"
)

// ParseAccess 把创建子账户的 access 收成权限。Spot/Margin/Futures 记为划转权。
func ParseAccess(access string) ([]Privilege, error) {
	access = strings.TrimSpace(access)
	if access == "" {
		return nil, nil
	}
	if strings.EqualFold(access, "all") || strings.EqualFold(access, "general") {
		return []Privilege{PrivDeposit, PrivWithdraw, PrivTransfer, PrivHop}, nil
	}
	parts := strings.FieldsFunc(access, func(r rune) bool { return r == ',' || r == '|' || r == ' ' })
	out := make([]Privilege, 0, len(parts))
	seen := map[Privilege]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var priv Privilege
		switch strings.ToLower(p) {
		case "spot", "margin", "futures", "trade":
			priv = PrivTransfer
		default:
			got, err := ParsePrivilege(p)
			if err != nil {
				return nil, err
			}
			priv = got
		}
		if _, ok := seen[priv]; ok {
			continue
		}
		seen[priv] = struct{}{}
		out = append(out, priv)
	}
	return out, nil
}

func ParsePrivilege(s string) (Privilege, error) {
	switch Privilege(strings.ToLower(strings.TrimSpace(s))) {
	case PrivDeposit, PrivWithdraw, PrivTransfer, PrivHop:
		return Privilege(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", ErrPrivilege
	}
}

func AllPrivileges() []Privilege {
	return []Privilege{PrivDeposit,
		PrivWithdraw, PrivTransfer, PrivHop}
}

func hasPriv(got []Privilege, want Privilege) bool {
	for _, p := range got {
		if p == want {
			return true
		}
	}
	return false
}

type Status string

const (
	StatusActive Status = "active"
	StatusFrozen Status = "frozen"
	StatusClosed Status = "closed"
)

func (s Status) CanDebit() bool { return s == StatusActive }

func (s Status) CanCredit() bool { return s == StatusActive || s == StatusFrozen }

type Parent struct {
	ID        string        `json:"id"`
	UserID    string        `json:"user_id"`
	Label     string        `json:"label"`
	Status    Status        `json:"status"`
	Reason    string        `json:"reason,omitempty"`
	Fence     persist.Fence `json:"-"`
	OpenedAt  time.Time     `json:"opened_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func (p Parent) Copy() Parent { return p }

type Child struct {
	ID         string        `json:"id"`
	ParentID   string        `json:"parent_id"`
	UserID     string        `json:"user_id"`
	Label      string        `json:"label"`
	Status     Status        `json:"status"`
	Reason     string        `json:"reason,omitempty"`
	Privileges []Privilege   `json:"privileges"`
	Fence      persist.Fence `json:"-"`
	OpenedAt   time.Time     `json:"opened_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

func (c Child) Copy() Child {
	cp := c
	if c.Privileges != nil {
		cp.Privileges = append([]Privilege(nil), c.Privileges...)
	}
	return cp
}

type BookBal struct {
	OwnerID   string `json:"owner_id"`
	UserID    string `json:"user_id"`
	Book      string `json:"book"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

type HopRec struct {
	ID         string    `json:"id"`
	Idem       string    `json:"idempotency_key"`
	FromOwner  string    `json:"from_owner"`
	ToOwner    string    `json:"to_owner"`
	FromUser   string    `json:"from_user"`
	ToUser     string    `json:"to_user"`
	FromBook   string    `json:"from_book"`
	ToBook     string    `json:"to_book"`
	Asset      string    `json:"asset"`
	Amount     string    `json:"amount"`
	Kind       string    `json:"kind"`
	Reversed   bool      `json:"reversed,omitempty"`
	TransferID string    `json:"transfer_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h HopRec) Copy() HopRec { return h }

type Snapshot struct {
	Parent   Parent    `json:"parent"`
	Children []Child   `json:"children"`
	Books    []BookBal `json:"books"`
	Host     string    `json:"host"`
	Region   string    `json:"region,omitempty"`
}

type OpenParentReq struct {
	UserID string `json:"user_id"`
	Label  string `json:"label"`
}

type OpenChildReq struct {
	ParentID   string      `json:"parent_id"`
	UserID     string      `json:"user_id"`
	Label      string      `json:"label"`
	Privileges []Privilege `json:"privileges"`
}

type HopReq struct {
	FromOwner      string `json:"from_owner"`
	ToOwner        string `json:"to_owner"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PostReq struct {
	OwnerID        string `json:"owner_id"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	Kind           string `json:"kind"`
	IdempotencyKey string `json:"idempotency_key"`
	FreezeCASID    string `json:"freeze_cas_id,omitempty"`
}
