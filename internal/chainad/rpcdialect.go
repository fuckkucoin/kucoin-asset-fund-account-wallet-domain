package chainad

// rpcDialect 一族链的节点方法名。ETH、BTC、TRON、SOL、MEMO 的观察和广播仍走 RPCAdapter。
type rpcDialect struct {
	head    string
	inspect string
	submit  string
	verbose bool
}

func dialectOf(fam Family) rpcDialect {
	switch fam {
	case FamBTC:
		return rpcDialect{
			head: "getblockcount", inspect: "getrawtransaction", submit: "sendrawtransaction", verbose: true,
		}
	case FamSOL:
		return rpcDialect{head: "getSlot", inspect: "getTransaction", submit: "sendTransaction"}
	default:
		return rpcDialect{
			head: "eth_blockNumber", inspect: "eth_getTransactionByHash", submit: "eth_sendRawTransaction",
		}
	}
}

func (d rpcDialect) inspectParams(hash string) []any {
	if d.verbose {
		return []any{hash, true}
	}
	return []any{hash}
}
