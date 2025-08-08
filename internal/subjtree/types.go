package subjtree

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSystemSubject 试图改写系统内置科目时返回。

var ErrSystemSubject = errors.New("system subject is read-only")

// ErrVersionExists 重复发布同一代次号时返回。

var ErrVersionExists = errors.New("subject tree version already exists")

// ErrVersionNotFound 查询不存在的代次时返回。

var ErrVersionNotFound = errors.New("subject tree version not found")

// ErrNodeNotFound 查询不存在的节点时返回。
var ErrNodeNotFound = errors.New("subject tree node not found")

// ErrUnclosable 对不可关闭科目发出关闭指令时返回。

var ErrUnclosable = errors.New("subject is not closable")

// ErrDuplicateEdge 同一代次中重复定义边时返回。

var ErrDuplicateEdge = errors.New("duplicate edge in tree version")

// SubjectKind 科目类别。

type SubjectKind string

const (
	// KindAsset 资产科目：余额正方向为借方。
	KindAsset SubjectKind = "asset"
	// KindLiability 负债科目：余额正方向为贷方。
	KindLiability SubjectKind = "liability"
	// KindEquity 权益科目。
	KindEquity SubjectKind = "equity"
	// KindRevenue 收入科目。
	KindRevenue SubjectKind = "revenue"
	// KindExpense 费用科目。
	KindExpense SubjectKind = "expense"
	// KindBridge 内部过渡科目（不对用户展示）。
	KindBridge SubjectKind = "bridge"
)

// Node 科目树节点。节点创建后代次内不可修改；is_system=true 则跨代次只读。

type Node struct {
	// ID 科目标识符，全局唯一，推荐用 funding.{account_type} 形态。
	ID string `json:"id"`
	Label string      `json:"label"`
	Kind  SubjectKind `json:"kind"`
	// IsSystem 系统内置科目；一旦为 true 不允许删除或重命名。
	IsSystem bool `json:"is_system"`
	// Closable 余额归零后是否允许运营将科目关闭（冻结新增分录）。
	// 系统科目不得关闭；is_system=true 则本字段恒为 false。
	Closable bool `json:"closable"`
	// Closed 已关闭：不再接受新分录（需先确认余额为零）。
	Closed bool `json:"closed"`
	// ParentID 父科目 ID，根节点为空。
	ParentID string `json:"parent_id,omitempty"`
	Memo string `json:"memo,omitempty"`
}

// Edge 科目树边：允许从 From → To 的 hop 方向。
// 一条边代表一类合法的资金流转方向，与 BizKind 关联见 acctpol 包。
type Edge struct {
	From string `json:"from"`
	To        string `json:"to"`
	BidirFlag bool   `json:"bidir,omitempty"` // true 时 To→From 亦合法
	Memo string `json:"memo,omitempty"`
}

// TreeVersion 科目树的一个不可变代次快照。

type TreeVersion struct {
	Version     int       `json:"version"`
	PublishedAt time.Time `json:"published_at"`
	Author      string    `json:"author"`
	Memo  string `json:"memo,omitempty"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Validate 检查代次内部一致性：节点 ID 唯一、边引用存在、系统科目不为 closed、
// 边不重复。
   func (tv TreeVersion) Validate() error {
	  if tv.Version <= 0 {
		   return fmt.Errorf("version must be > 0")
	  }
	  seen := map[string]bool{}
	  for _, n := range tv.Nodes {
		  n.ID = strings.TrimSpace(n.ID)
		  if n.ID == "" {
			  return fmt.Errorf("node missing id")
		   }
		  if seen[n.ID] {
			  return fmt.Errorf("duplicate node id: %s", n.ID)
		   }
		   seen[n.ID] = true
		   if n.IsSystem && n.Closed {
			  return fmt.Errorf("system node %s cannot be closed", n.ID)
		   }
	   }
	  type edgeKey struct{ from, to string }
	   edgeSeen := map[edgeKey]bool{}
	   for _, e := range tv.Edges {
		   from, to := strings.TrimSpace(e.From), strings.TrimSpace(e.To)
		  if from == "" || to == "" {
			  return fmt.Errorf("edge has empty from or to")
		   }
		  if !seen[from] {
			   return fmt.Errorf("edge references unknown from node: %s", from)
		  }
		  if !seen[to] {
			  return fmt.Errorf("edge references unknown to node: %s", to)
		  }
		   k := edgeKey{from, to}
		  if edgeSeen[k] {
			  return ErrDuplicateEdge
		   }
		   edgeSeen[k] = true
		   if e.BidirFlag {
			   kb := edgeKey{to, from}
			  if edgeSeen[kb] {
				  return ErrDuplicateEdge
			   }
			  edgeSeen[kb] = true
		   }
	   }
	  return nil
  }

// NodeByID 在代次内按 ID 查节点，不存在返回 ErrNodeNotFound。

   func (tv TreeVersion) NodeByID(id string) (Node, error) {
	  for _, n := range tv.Nodes {
		  if n.ID == id {
			  return n, nil
		  }
	   }
	  return Node{}, fmt.Errorf("%w: %s", ErrNodeNotFound, id)
  }

// AllowsHop 检查 from→to 的 hop 是否在本代次树中存在合法边。
   func (tv TreeVersion) AllowsHop(from, to string) bool {
	   for _, e := range tv.Edges {
		   if e.From == from && e.To == to {
			   return true
		   }
		   if e.BidirFlag && e.From == to && e.To == from {
			  return true
		  }
	  }
	   return false
   }

// ClosableNodes 返回本代次中可关闭（且尚未关闭）的科目列表。

   func (tv TreeVersion) ClosableNodes() []Node {
	   var out []Node
	  for _, n := range tv.Nodes {
		   if n.Closable && !n.Closed && !n.IsSystem {
			   out = append(out, n)
		   }
	   }
	   return out
   }
