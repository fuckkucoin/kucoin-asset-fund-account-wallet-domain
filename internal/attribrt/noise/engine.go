package noise

import "strings"

// Hit 一条命中。Disposition 为 pass 的不进入结果。
type Hit struct {
	Name string `json:"name"`
	Disposition string `json:"disposition"`
	Reason      string `json:"reason"`
	Penalty     int    `json:"penalty"`
}

// Input 一次分类的上下文。规则由策略矩阵选出，分类器不再各自带链参数。

type Input struct {
	Chain string
	Asset string
	Amount string
	From string
	To     string
	Memo   string
	Tag    string
	Script string
	Native string
	Dust   string
	Decimals int
	Enabled  []string
	Rules    []Rule
	Hot      []string
	Deny     []string
	Look     []string
	Contract bool
	Sanctioned  bool
	Burst       int
	CaseFold    bool
	Numeric     bool
	StripZW     bool
	LeadingZero bool
}

// Result 合并后的处置。Block 表示这条路径不能入账。
type Result struct {
	Hits []Hit
	Disposition string
	Block       bool
	Penalty     int
}

var rank = map[string]int{
	"pass": 0,
	"drop": 1,
	"delay": 2,
	"hold":  3,
	"desk": 4,
}

// Run 按启用列表跑分类器，取最严处置。
  func Run(in Input) Result {
	  enabled := map[string]bool{}
	  for _, n := range in.Enabled {
		   enabled[n] = true
	   }
	   order := []string{"dust", "self", "sweep", "script", "scale", "poison", "airdrop", "decoy", "homoglyph", "spam"}
	   var hits []Hit
	  for _, name := range order {
		  if !enabled[name] {
			  continue
		   }
		  h := dispatch(name, in)
		  if h.Disposition == "" || h.Disposition == "pass" {
			  continue
		   }
		  if rank[h.Disposition] == 0 {
			  continue
		   }
		   hits = append(hits, h)
	  }
	  out := Result{Hits: hits, Disposition: "pass"}
	   for _, h := range hits {
		  out.Penalty += h.Penalty
		  if rank[h.Disposition] > rank[out.Disposition] {
			   out.Disposition = h.Disposition
		  }
	  }
	   out.Block = out.Disposition != "pass"
	   return out
   }

   func dispatch(name string, in Input) Hit {
	   rule := pick(in.Rules, in.Chain, name)
	   switch name {
	  case "dust":
		   return classDust(in, rule)
	  case "self":
		  return classSelf(in, rule)
	   case "sweep":
		  return classSweep(in, rule)
	   case "script":
		  return classScript(in, rule)
	   case "scale":
		   return classScale(in, rule)
	   case "poison":
		  return classPoison(in, rule)
	  case "airdrop":
		  return classAirdrop(in, rule)
	  case "decoy":
		  return classDecoy(in, rule)
	   case "homoglyph":
		   return classHomoglyph(in, rule)
	   case "spam":
		   return classSpam(in, rule)
	   default:
		   return Hit{}
	  }
   }

   func pick(rules []Rule, chain, name string) Rule {
	  var star Rule
	  foundStar := false
	  for _, r := range rules {
		  if r.Name != name {
			   continue
		   }
		   if strings.EqualFold(r.Chain, chain) {
			   return r
		   }
		  if r.Chain == "*" && !foundStar {
			  star = r
			  foundStar = true
		   }
	  }
	  if foundStar {
		   return star
	  }
	  return Rule{Name: name, Disposition: "desk", Penalty: 1000}
   }

   func hit(rule Rule, reason string) Hit {
	   disp := rule.Disposition
	   if disp == "" {
		  disp = "desk"
	   }
	   return Hit{Name: rule.Name, Disposition: disp, Reason: reason, Penalty: rule.Penalty}
   }
