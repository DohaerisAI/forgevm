package agentproto

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const maxMessageSize = 64 * 1024 * 1024 // 64 MB

// WriteMessage writes a length-prefixed JSON message to w.
func WriteMessage(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	// 4-byte big-endian length prefix
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

// ReadMessage reads a length-prefixed message from r into buf.
func ReadMessage(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	size := binary.BigEndian.Uint32(hdr[:])
	if size > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}
	return buf, nil
}

// SendRequest writes a Request to w.
func SendRequest(w io.Writer, req *Request) error {
	return WriteMessage(w, req)
}

// ReadRequest reads a Request from r.
func ReadRequest(r io.Reader) (*Request, error) {
	data, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}
	return &req, nil
}

// SendResponse writes a Response to w.
func SendResponse(w io.Writer, resp *Response) error {
	return WriteMessage(w, resp)
}

// ReadResponse reads a Response from r.
func ReadResponse(r io.Reader) (*Response, error) {
	data, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

// SendStreamResponse writes a StreamResponse to w.
func SendStreamResponse(w io.Writer, resp *StreamResponse) error {
	return WriteMessage(w, resp)
}

// ReadStreamResponse reads a StreamResponse from r.
func ReadStreamResponse(r io.Reader) (*StreamResponse, error) {
	data, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	var resp StreamResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal stream response: %w", err)
	}
	return &resp, nil
}

// MarshalParams marshals v into json.RawMessage for use in Request.Params.
func MarshalParams(v any) (json.RawMessage, error) {
	return json.Marshal(v)
}

// UnmarshalParams unmarshals Request.Params into v.
func UnmarshalParams(raw json.RawMessage, v any) error {
	return json.Unmarshal(raw, v)
}

// MarshalResult marshals v into json.RawMessage for use in Response.Result.
func MarshalResult(v any) (json.RawMessage, error) {
	return json.Marshal(v)
}

// UnmarshalResult unmarshals Response.Result into v.
func UnmarshalResult(raw json.RawMessage, v any) error {
	return json.Unmarshal(raw, v)
}
