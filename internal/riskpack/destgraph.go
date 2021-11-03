package riskpack

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Node struct {
	Chain string `json:"chain"`
	Address string `json:"address"`
	Cluster string `json:"cluster"`
}

// Graph 出金地址相关图。同一 cluster 共享敞口；边是显式 Link 或家族归一。
type Graph struct {
	mu sync.Mutex
	nodes    map[string]Node
	edges    map[string]map[string]struct{}
	clusters map[string]string
	store    *Store
}

  func NewGraph() *Graph {
	   return &Graph{nodes: map[string]Node{}, edges: map[string]map[string]struct{}{}, clusters: map[string]string{}}
   }

   func nodeKey(chain, addr string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(addr))
  }

  func (g *Graph) Add(chain, addr, cluster string) (Node, error) {
	  if g == nil {
		   return Node{}, ErrUnavailable
	  }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		  return Node{}, err
	  }
	  addr, err = validate.Address(ch, addr)
	  if err != nil {
		   // 部分地址过不了链形态时，仍要求非空，避免空节点进图。
		  addr = strings.TrimSpace(addr)
		   if addr == "" {
			  return Node{}, err
		  }
		   if e2 := wallet.ValidChainAddress(ch, addr); e2 != nil && len(addr) < 8 {
			  return Node{}, err
		   }
	   }
	   cluster = strings.TrimSpace(cluster)
	   if cluster == "" {
		   cluster = familyCluster(ch, addr)
	  }
	   k := nodeKey(ch, addr)
	   if g.store != nil {
		   if _, err := g.store.UpsertNode(ch, addr, cluster); err != nil {
			  return Node{}, err
		   }
	  }
	  g.mu.Lock()
	   defer g.mu.Unlock()
	  n := Node{Chain: ch, Address: addr, Cluster: cluster}
	  g.nodes[k] = n
	   g.clusters[k] = cluster
	  return n, nil
   }

// BindStore 把内存图镜像到持久图谱。未绑定则保持原内存语义。

   func (g *Graph) BindStore(st *Store) *Graph {
	   if g != nil {
		   g.store = st
	   }
	  return g
   }

// Store 返回已绑定的持久图谱；未绑定为 nil。
   func (g *Graph) Store() *Store {
	   if g == nil {
		  return nil
	   }
	   return g.store
  }

   func (g *Graph) Link(aChain, aAddr, bChain, bAddr string) error {
	  if g == nil {
		  return ErrUnavailable
	   }
	   ak := nodeKey(aChain, aAddr)
	  bk := nodeKey(bChain, bAddr)
	   g.mu.Lock()
	   if _, ok := g.nodes[ak]; !ok {
		   g.mu.Unlock()
		  return fmt.Errorf("%w: left", ErrDest)
	  }
	   if _, ok := g.nodes[bk]; !ok {
		  g.mu.Unlock()
		  return fmt.Errorf("%w: right", ErrDest)
	   }
	   if g.edges[ak] == nil {
		   g.edges[ak] = map[string]struct{}{}
	  }
	   if g.edges[bk] == nil {
		   g.edges[bk] = map[string]struct{}{}
	   }
	   g.edges[ak][bk] = struct{}{}
	  g.edges[bk][ak] = struct{}{}
	   // 并到同一 cluster：取左边。持久图谱另按权重阈重算，不覆盖这里的内存并查。
	  left := g.clusters[ak]
	  g.clusters[bk] = left
	  nb := g.nodes[bk]
	  nb.Cluster = left
	  g.nodes[bk] = nb
	   g.mu.Unlock()
	  if g.store != nil {
		  if _, err := g.store.Link(aChain, aAddr, bChain, bAddr, "1", EdgeExplicit, "graph.link", 24*time.Hour); err != nil {
			  return err
		  }
	   }
	   return nil
   }

   func (g *Graph) ClusterOf(chain, addr string) string {
	   if g == nil {
		  return familyCluster(chain, addr)
	  }
	  if g.store != nil {
		  if n, ok := g.store.Node(chain, addr); ok && n.ClusterID != "" {
			  return n.ClusterID
		  }
	  }
	   g.mu.Lock()
	   defer g.mu.Unlock()
	  if c, ok := g.clusters[nodeKey(chain, addr)]; ok {
		   return c
	   }
	  return familyCluster(chain, addr)
   }

   func (g *Graph) Members(cluster string) []Node {
	  g.mu.Lock()
	   defer g.mu.Unlock()
	   out := make([]Node, 0)
	   for k, c := range g.clusters {
		   if c != cluster {
			  continue
		  }
		   out = append(out, g.nodes[k])
	  }
	   return out
  }

  func (g *Graph) Correlated(chain, addr string) []Node {
	  return g.Members(g.ClusterOf(chain, addr))
   }

   func familyCluster(chain, addr string) string {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  addr = strings.ToLower(strings.TrimSpace(addr))
	  if wallet.IsEVM(chain) {
		  return "evm|" + addr
	   }
	   if len(addr) >= 8 {
		   return chain + "|" + addr[len(addr)-8:]
	  }
	   return chain + "|" + addr
   }
