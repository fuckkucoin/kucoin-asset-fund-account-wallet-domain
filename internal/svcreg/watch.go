package svcreg

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protowire"
)

// WatchPrefix 订阅前缀下的变更。有变更时回调键、值和是否删除。
func (e *Etcd) WatchPrefix(ctx context.Context, prefix string, fn func(key, val string, deleted bool)) error {
	if e == nil {
		return ErrClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := e.connAt(ctx, 0)
	if err != nil {
		return err
	}
	desc := &grpc.StreamDesc{StreamName: "Watch", ServerStreams: true, ClientStreams: true}
	stream, err := conn.NewStream(ctx, desc, "/etcdserverpb.Watch/Watch", grpc.ForceCodec(etcdCodec{}))
	if err != nil {
		return err
	}
	if err := stream.SendMsg(&etcdFrame{b: encodeWatchCreate([]byte(prefix), prefixEnd(prefix))}); err != nil {
		return err
	}
	for {
		out := &etcdFrame{}
		if err := stream.RecvMsg(out); err != nil {
			return err
		}
		for _, ev := range decodeWatchEvents(out.b) {
			if fn != nil {
				fn(ev.key, ev.val, ev.del)
			}
		}
	}
}

func encodeWatchCreate(key, end []byte) []byte {
	var create []byte
	create = protowire.AppendTag(create, 1, protowire.BytesType)
	create = protowire.AppendBytes(create, key)
	if len(end) > 0 {
		create = protowire.AppendTag(create, 2, protowire.BytesType)
		create = protowire.AppendBytes(create, end)
	}
	var req []byte
	req = protowire.AppendTag(req, 1, protowire.BytesType)
	req = protowire.AppendBytes(req, create)
	return req
}

type watchEvent struct {
	key string
	val string
	del bool
}

func decodeWatchEvents(b []byte) []watchEvent {
	var out []watchEvent
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
			if num == 11 {
				if ev, ok := decodeEvent(v); ok {
					out = append(out, ev)
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

func decodeEvent(b []byte) (watchEvent, bool) {
	var ev watchEvent
	var got bool
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			break
		}
		b = b[n:]
		switch typ {
		case protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				return ev, false
			}
			b = b[m:]
			if num == 1 && v == 1 {
				ev.del = true
			}
		case protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return ev, false
			}
			b = b[m:]
			if num == 2 {
				if k, val, ok := kvPair(v); ok {
					ev.key, ev.val = k, val
					got = true
				}
			}
		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return ev, false
			}
			b = b[m:]
		}
	}
	return ev, got
}
