package meshgrpc

import (
	"encoding/binary"
	"errors"
	"fmt"

	"google.golang.org/grpc/encoding"
)

// CallRequest 内部调用的方法、路径和报文体。字段与原来的 HTTP 调用一致。
type CallRequest struct {
	Method string
	Path   string
	Body   []byte
}

// CallResponse 对端 HTTP 处理结果。状态码和响应体原样返回。
type CallResponse struct {
	Status int32
	Body   []byte
}

const codecName = "meshcall"

type meshCodec struct{}

func (meshCodec) Name() string { return codecName }

func (meshCodec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case *CallRequest:
		return packCall(m.Method, m.Path, m.Body), nil
	case *CallResponse:
		return packResp(m.Status, m.Body), nil
	default:
		return nil, fmt.Errorf("mesh grpc: marshal %T", v)
	}
}

func (meshCodec) Unmarshal(data []byte, v any) error {
	switch m := v.(type) {
	case *CallRequest:
		method, path, body, err := unpackCall(data)
		if err != nil {
			return err
		}
		m.Method, m.Path, m.Body = method, path, body
		return nil
	case *CallResponse:
		status, body, err := unpackResp(data)
		if err != nil {
			return err
		}
		m.Status, m.Body = status, body
		return nil
	default:
		return fmt.Errorf("mesh grpc: unmarshal %T", v)
	}
}

func init() {
	encoding.RegisterCodec(meshCodec{})
}

func packCall(method, path string, body []byte) []byte {
	mb, pb := []byte(method), []byte(path)
	out := make([]byte, 8+len(mb)+len(pb)+len(body))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(mb)))
	copy(out[4:], mb)
	binary.BigEndian.PutUint32(out[4+len(mb):8+len(mb)], uint32(len(pb)))
	copy(out[8+len(mb):], pb)
	copy(out[8+len(mb)+len(pb):], body)
	return out
}

func unpackCall(data []byte) (string, string, []byte, error) {
	if len(data) < 8 {
		return "", "", nil, errors.New("mesh grpc: short call")
	}
	ml := int(binary.BigEndian.Uint32(data[0:4]))
	if ml < 0 || 4+ml+4 > len(data) {
		return "", "", nil, errors.New("mesh grpc: bad method")
	}
	method := string(data[4 : 4+ml])
	pl := int(binary.BigEndian.Uint32(data[4+ml : 8+ml]))
	if pl < 0 || 8+ml+pl > len(data) {
		return "", "", nil, errors.New("mesh grpc: bad path")
	}
	path := string(data[8+ml : 8+ml+pl])
	body := append([]byte(nil), data[8+ml+pl:]...)
	return method, path, body, nil
}

func packResp(status int32, body []byte) []byte {
	out := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(out[0:4], uint32(status))
	copy(out[4:], body)
	return out
}

func unpackResp(data []byte) (int32, []byte, error) {
	if len(data) < 4 {
		return 0, nil, errors.New("mesh grpc: short response")
	}
	status := int32(binary.BigEndian.Uint32(data[0:4]))
	body := append([]byte(nil), data[4:]...)
	return status, body, nil
}
