package svcreg

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	methodKVPut   = "/etcdserverpb.KV/Put"
	methodKVRange = "/etcdserverpb.KV/Range"
	methodKVDeleteRange = "/etcdserverpb.KV/DeleteRange"
	methodLeaseGrant  = "/etcdserverpb.Lease/LeaseGrant"
	methodLeaseRevoke = "/etcdserverpb.Lease/LeaseRevoke"
	methodLeaseKeep   = "/etcdserverpb.Lease/LeaseKeepAlive"
)

type etcdEndpoint struct {
	target string
	tls    bool
	conn *grpc.ClientConn
}

// etcdFrame 原始 protobuf。Name 必须是 proto，否则 etcd 拒绝 content-subtype。

type etcdFrame struct{ b []byte }

type etcdCodec struct{}

  func (etcdCodec) Marshal(v any) ([]byte, error) {
	  f, ok := v.(*etcdFrame)
	  if !ok {
		  return nil, fmt.Errorf("etcd codec: %T", v)
	  }
	  return append([]byte(nil), f.b...), nil
   }

   func (etcdCodec) Unmarshal(data []byte, v any) error {
	  f, ok := v.(*etcdFrame)
	  if !ok {
		  return fmt.Errorf("etcd codec: %T", v)
	  }
	  f.b = append([]byte(nil), data...)
	  return nil
  }

  func (etcdCodec) Name() string { return "proto" }

   func parseEtcdTarget(ep string) (string, bool, error) {
	   ep = strings.TrimSpace(ep)
	   if ep == "" {
		   return "", false, ErrEndpoint
	   }
	  if strings.Contains(ep, "://") {
		  u, err := url.Parse(ep)
		  if err != nil {
			  return "", false, err
		  }
		  host := u.Hostname()
		   if host == "" {
			  return "", false, ErrEndpoint
		   }
		  port := u.Port()
		   if port == "" {
			  port = "2379"
		   }
		   useTLS := strings.EqualFold(u.Scheme, "https")
		   return net.JoinHostPort(host, port), useTLS, nil
	  }
	  if _, _, err := net.SplitHostPort(ep); err != nil {
		   return net.JoinHostPort(ep, "2379"), false, nil
	   }
	   return ep, false, nil
   }

   func (e *Etcd) connAt(ctx context.Context, idx int) (*grpc.ClientConn, error) {
	   if e == nil {
		   return nil, ErrClosed
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  if e.closed {
		  return nil, ErrClosed
	  }
	  if idx < 0 || idx >= len(e.dials) {
		  return nil, ErrEndpoint
	  }
	  d := &e.dials[idx]
	   if d.conn != nil {
		   return d.conn, nil
	   }
	  var opt grpc.DialOption
	   if d.tls {
		  opt = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}))
	  } else {
		   opt = grpc.WithTransportCredentials(insecure.NewCredentials())
	   }
	  conn, err := grpc.NewClient(d.target, opt)
	   if err != nil {
		   return nil, err
	  }
	   d.conn = conn
	  _ = ctx
	   return conn, nil
   }

  func (e *Etcd) rpc(ctx context.Context, method string, in []byte) ([]byte, error) {
	  if e == nil {
		  return nil, ErrClosed
	  }
	   e.mu.Lock()
	  if e.closed {
		   e.mu.Unlock()
		   return nil, ErrClosed
	  }
	  n := len(e.dials)
	  if n == 0 {
		   e.mu.Unlock()
		  return nil, ErrEndpoint
	  }
	  start := int(e.epIdx % uint64(n))
	   e.mu.Unlock()
	   var last error
	   for i := 0; i < n; i++ {
		  idx := (start + i) % n
		  conn, err := e.connAt(ctx, idx)
		  if err != nil {
			   last = err
			   e.log.Error().Err(err).Int("idx", idx).Str("method", method).Msg("registry dial")
			   continue
		   }
		  out := &etcdFrame{}
		   err = conn.Invoke(ctx, method, &etcdFrame{b: in}, out, grpc.ForceCodec(etcdCodec{}))
		   if err == nil {
			   if i > 0 {
				   e.mu.Lock()
				   e.epIdx = uint64(idx)
				  e.mu.Unlock()
				   e.log.Warn().Int("failover", i).Str("method", method).Msg("registry endpoint failover")
				  observeRegistry("failover", "ok")
			  }
			  return out.b, nil
		  }
		   last = err
		  e.log.Error().Err(err).Int("idx", idx).Str("method", method).Msg("registry rpc")
		   observeRegistry("endpoint", "fail")
	   }
	   if last == nil {
		   last = ErrEndpoint
	   }
	   return nil, last
   }

  func (e *Etcd) kvPut(ctx context.Context, key string, val []byte, lease int64) error {
	   _, err := e.rpc(ctx, methodKVPut, encodePut([]byte(key), val, lease))
	  return err
   }

   func (e *Etcd) kvDel(ctx context.Context, key string) error {
	  _, err := e.rpc(ctx, methodKVDeleteRange, encodeDelete([]byte(key)))
	   return err
   }

// RangePairs 按前缀取出键和值，供配置缓存整表刷新。
func (e *Etcd) RangePairs(ctx context.Context, prefix string) (map[string]string, error) {
	raw, err := e.rpc(ctx, methodKVRange, encodeRange([]byte(prefix), prefixEnd(prefix)))
	if err != nil {
		return nil, err
	}
	return decodeKVPairs(raw), nil
}

  func (e *Etcd) kvRange(ctx context.Context, prefix string) ([]Instance, error) {
	  raw,
		   err := e.rpc(ctx, methodKVRange, encodeRange([]byte(prefix), prefixEnd(prefix)))
	  if err != nil {
		   return nil, err
	  }
	  return decodeInstances(raw), nil
   }

   func (e *Etcd) grantLease(ctx context.Context) (int64, error) {
	  raw, err := e.rpc(ctx, methodLeaseGrant, encodeLeaseGrant(defaultLeaseTTL))
	  if err != nil {
		  return 0, err
	  }
	  id := decodeVarintField(raw, 2)
	   if id == 0 {
		  return 0, ErrEndpoint
	   }
	   return int64(id), nil
   }

   func (e *Etcd) revokeLease(ctx context.Context, id int64) error {
	   if id == 0 {
		  return nil
	   }
	   _, err := e.rpc(ctx, methodLeaseRevoke, encodeLeaseRevoke(id))
	  return err
  }

  func (e *Etcd) keepAliveRPC(ctx context.Context, id int64) error {
	  if id == 0 {
		  return ErrEndpoint
	   }
	   e.mu.Lock()
	   n := len(e.dials)
	  start := 0
	   if n > 0 {
		  start = int(e.epIdx % uint64(n))
	   }
	  closed := e.closed
	  e.mu.Unlock()
	  if closed {
		   return ErrClosed
	   }
	   if n == 0 {
		  return ErrEndpoint
	   }
	  var last error
	  for i := 0; i < n; i++ {
		   idx := (start + i) % n
		  conn, err := e.connAt(ctx, idx)
		   if err != nil {
			   last = err
			   continue
		   }
		  stream, err := conn.NewStream(ctx, &grpc.StreamDesc{
			   StreamName:    "LeaseKeepAlive",
			  ServerStreams: true,
			  ClientStreams: true,
		   }, methodLeaseKeep, grpc.ForceCodec(etcdCodec{}))
		  if err != nil {
			   last = err
			   continue
		  }
		   if err = stream.SendMsg(&etcdFrame{b: encodeLeaseKeepAlive(id)}); err != nil {
			  last = err
			  continue
		  }
		  out := &etcdFrame{}
		   if err = stream.RecvMsg(out); err != nil {
			  last = err
			  continue
		  }
		  if decodeVarintField(out.b, 3) == 0 {
			   last = ErrEndpoint
			  continue
		  }
		   _ = stream.CloseSend()
		   return nil
	  }
	  if last == nil {
		  last = ErrEndpoint
	  }
	  return last
  }

   func (e *Etcd) closeConns() {
	   if e == nil {
		   return
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  for i := range e.dials {
		   if e.dials[i].conn != nil {
			  _ = e.dials[i].conn.Close()
			   e.dials[i].conn = nil
		   }
	  }
  }

  func encodePut(key, val []byte, lease int64) []byte {
	   b := appendBytesField(nil, 1, key)
	   b = appendBytesField(b, 2, val)
	  if lease != 0 {
		   b = appendVarintField(b,
			   3, uint64(lease))
	   }
	  return b
  }

   func encodeRange(key, end []byte) []byte {
	   b := appendBytesField(nil, 1, key)
	  if len(end) > 0 {
		  b = appendBytesField(b, 2, end)
	  }
	   return b
  }

  func encodeDelete(key []byte) []byte {
	   return appendBytesField(nil, 1, key)
  }

   func encodeLeaseGrant(ttl int64) []byte {
	  return appendVarintField(nil, 1, uint64(ttl))
  }

   func encodeLeaseRevoke(id int64) []byte {
	  return appendVarintField(nil, 1, uint64(id))
  }

  func encodeLeaseKeepAlive(id int64) []byte {
	  return appendVarintField(nil,
		   1, uint64(id))
   }

   func appendBytesField(b []byte, num protowire.Number, v []byte) []byte {
	  b = protowire.AppendTag(b, num, protowire.BytesType)
	   return protowire.AppendBytes(b, v)
   }

   func appendVarintField(b []byte, num protowire.Number, v uint64) []byte {
	  b = protowire.AppendTag(b, num, protowire.VarintType)
	   return protowire.AppendVarint(b, v)
   }

   func decodeVarintField(b []byte, want protowire.Number) uint64 {
	   for len(b) > 0 {
		  num, typ, n := protowire.ConsumeTag(b)
		  if n < 0 {
			   return 0
		  }
		   b = b[n:]
		  switch typ {
		   case protowire.VarintType:
			   v, m := protowire.ConsumeVarint(b)
			  if m < 0 {
				  return 0
			  }
			   b = b[m:]
			  if num == want {
				  return v
			  }
		   default:
			  m := protowire.ConsumeFieldValue(num, typ, b)
			   if m < 0 {
				   return 0
			  }
			   b = b[m:]
		  }
	   }
	  return 0
  }

   func decodeInstances(b []byte) []Instance {
	  var out []Instance
	  for len(b) > 0 {
		   num, typ, n := protowire.ConsumeTag(b)
		  if n < 0 {
			   break
		   }
		   b = b[n:]
		   switch typ {
		   case protowire.BytesType:
			  v,
				   m := protowire.ConsumeBytes(b)
			  if m < 0 {
				   return out
			  }
			   b = b[m:]
			   if num != 2 {
				  continue
			   }
			   raw, ok := kvBytes(v)
			  if !ok || len(raw) == 0 {
				   continue
			  }
			  var inst Instance
			  if json.Unmarshal(raw, &inst) != nil || inst.Addr == "" {
				  continue
			   }
			   out = append(out, inst)
		   default:
			  m := protowire.ConsumeFieldValue(num, typ, b)
			  if m < 0 {
				  return out
			   }
			   b = b[m:]
		   }
	   }
	  return out
   }

   func decodeFirstValue(b []byte) ([]byte, bool) {
	  for len(b) > 0 {
		   num,
			   typ, n := protowire.ConsumeTag(b)
		   if n < 0 {
			   return nil, false
		  }
		  b = b[n:]
		  switch typ {
		  case protowire.BytesType:
			  v, m := protowire.ConsumeBytes(b)
			   if m < 0 {
				  return nil, false
			  }
			  b = b[m:]
			   if num == 2 {
				   val, ok := kvBytes(v)
				  if ok {
					   return val, true
				  }
			   }
		  default:
			  m := protowire.ConsumeFieldValue(num, typ, b)
			  if m < 0 {
				  return nil, false
			  }
			  b = b[m:]
		  }
	   }
	   return nil, false
   }

func decodeKVPairs(b []byte) map[string]string {
	out := map[string]string{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return out
		}
		b = b[n:]
		switch typ {
		case protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return out
			}
			b = b[m:]
			if num == 2 {
				if k, val, ok := kvPair(v); ok {
					out[k] = val
				}
			}
		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return out
			}
			b = b[m:]
		}
	}
	return out
}

func kvPair(b []byte) (string, string, bool) {
	var key, val []byte
	var gotKey, gotVal bool
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			break
		}
		b = b[n:]
		switch typ {
		case protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return "", "", false
			}
			b = b[m:]
			if num == 1 {
				key = append([]byte(nil), v...)
				gotKey = true
			}
			if num == 5 {
				val = append([]byte(nil), v...)
				gotVal = true
			}
		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return "", "", false
			}
			b = b[m:]
		}
	}
	if !gotKey {
		return "", "", false
	}
	return string(key), string(val), gotVal || gotKey
}

  func kvBytes(b []byte) ([]byte, bool) {
	   var val []byte
	  found := false
	  for len(b) > 0 {
		   num, typ, n := protowire.ConsumeTag(b)
		  if n < 0 {
			  break
		   }
		  b = b[n:]
		  switch typ {
		  case protowire.BytesType:
			  v, m := protowire.ConsumeBytes(b)
			   if m < 0 {
				   return val, found
			  }
			   b = b[m:]
			   if num == 5 {
				   val = append([]byte(nil), v...)
				  found = true
			   }
		   default:
			  m := protowire.ConsumeFieldValue(num, typ, b)
			  if m < 0 {
				   return val, found
			  }
			  b = b[m:]
		   }
	  }
	   return val, found
  }
