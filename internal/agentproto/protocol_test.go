package agentproto

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteReadMessage(t *testing.T) {
	var buf bytes.Buffer

	original := map[string]string{"hello": "world"}
	if err := WriteMessage(&buf, original); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	data, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got["hello"] != "world" {
		t.Errorf("got %v, want hello=world", got)
	}
}

func TestRequestResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	params, _ := MarshalParams(&ExecParams{
		Command: "echo hello",
		WorkDir: "/tmp",
	})
	req := &Request{
		ID:     "req-1",
		Method: MethodExec,
		Params: params,
	}

	if err := SendRequest(&buf, req); err != nil {
		t.Fatalf("SendRequest: %v", err)
	}

	got, err := ReadRequest(&buf)
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}

	if got.ID != "req-1" {
		t.Errorf("ID = %q, want %q", got.ID, "req-1")
	}
	if got.Method != MethodExec {
		t.Errorf("Method = %q, want %q", got.Method, MethodExec)
	}

	var gotParams ExecParams
	if err := UnmarshalParams(got.Params, &gotParams); err != nil {
		t.Fatalf("UnmarshalParams: %v", err)
	}
	if gotParams.Command != "echo hello" {
		t.Errorf("Command = %q, want %q", gotParams.Command, "echo hello")
	}
	if gotParams.WorkDir != "/tmp" {
		t.Errorf("WorkDir = %q, want %q", gotParams.WorkDir, "/tmp")
	}
}

func TestResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	result, _ := MarshalResult(&ExecResult{
		ExitCode: 0,
		Stdout:   "hello\n",
		Stderr:   "",
	})
	resp := &Response{
		ID:     "req-1",
		Result: result,
	}

	if err := SendResponse(&buf, resp); err != nil {
		t.Fatalf("SendResponse: %v", err)
	}

	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}

	if got.ID != "req-1" {
		t.Errorf("ID = %q, want %q", got.ID, "req-1")
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}

	var gotResult ExecResult
	if err := UnmarshalResult(got.Result, &gotResult); err != nil {
		t.Fatalf("UnmarshalResult: %v", err)
	}
	if gotResult.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", gotResult.Stdout, "hello\n")
	}
}

func TestStreamResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	sresp := &StreamResponse{
		ID:     "req-2",
		Stream: "stdout",
		Data:   "output data",
	}
	if err := SendStreamResponse(&buf, sresp); err != nil {
		t.Fatalf("SendStreamResponse: %v", err)
	}

	got, err := ReadStreamResponse(&buf)
	if err != nil {
		t.Fatalf("ReadStreamResponse: %v", err)
	}

	if got.ID != "req-2" {
		t.Errorf("ID = %q, want %q", got.ID, "req-2")
	}
	if got.Stream != "stdout" {
		t.Errorf("Stream = %q, want %q", got.Stream, "stdout")
	}
	if got.Data != "output data" {
		t.Errorf("Data = %q, want %q", got.Data, "output data")
	}
	if got.Done {
		t.Error("Done = true, want false")
	}
}

func TestStreamResponseDone(t *testing.T) {
	var buf bytes.Buffer

	sresp := &StreamResponse{
		ID:       "req-3",
		Done:     true,
		ExitCode: 42,
	}
	if err := SendStreamResponse(&buf, sresp); err != nil {
		t.Fatalf("SendStreamResponse: %v", err)
	}

	got, err := ReadStreamResponse(&buf)
	if err != nil {
		t.Fatalf("ReadStreamResponse: %v", err)
	}

	if !got.Done {
		t.Error("Done = false, want true")
	}
	if got.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", got.ExitCode)
	}
}

func TestErrorResponse(t *testing.T) {
	var buf bytes.Buffer

	resp := &Response{
		ID:    "req-err",
		Error: "something went wrong",
	}
	if err := SendResponse(&buf, resp); err != nil {
		t.Fatalf("SendResponse: %v", err)
	}

	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}

	if got.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", got.Error, "something went wrong")
	}
}

func TestWriteFileParamsSerialization(t *testing.T) {
	params := &WriteFileParams{
		Path:    "/tmp/test.txt",
		Content: []byte("hello world"),
		Mode:    "755",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got WriteFileParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Path != "/tmp/test.txt" {
		t.Errorf("Path = %q, want %q", got.Path, "/tmp/test.txt")
	}
	if string(got.Content) != "hello world" {
		t.Errorf("Content = %q, want %q", string(got.Content), "hello world")
	}
	if got.Mode != "755" {
		t.Errorf("Mode = %q, want %q", got.Mode, "755")
	}
}

func TestMultipleMessages(t *testing.T) {
	var buf bytes.Buffer

	// Write multiple messages.
	for i := range 5 {
		resp := &Response{ID: string(rune('a' + i))}
		if err := SendResponse(&buf, resp); err != nil {
			t.Fatalf("SendResponse %d: %v", i, err)
		}
	}

	// Read them back.
	for i := range 5 {
		got, err := ReadResponse(&buf)
		if err != nil {
			t.Fatalf("ReadResponse %d: %v", i, err)
		}
		want := string(rune('a' + i))
		if got.ID != want {
			t.Errorf("message %d: ID = %q, want %q", i, got.ID, want)
		}
	}
}

func TestMessageTooLarge(t *testing.T) {
	var buf bytes.Buffer
	// Write a header claiming 128MB (exceeds maxMessageSize of 64MB).
	buf.Write([]byte{0x08, 0x00, 0x00, 0x00}) // 128MB in big-endian

	_, err := ReadMessage(&buf)
	if err == nil {
		t.Fatal("expected error for oversized message")
	}
}
