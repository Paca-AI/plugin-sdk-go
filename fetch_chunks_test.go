package plugin

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeHost serves body in limit-capped chunks and can reject chunks above
// maxFit with ErrResponseTooLarge, like a plugin whose buffer is partly full.
type fakeHost struct {
	body   string
	maxFit int
	calls  []int // limits requested
}

func (f *fakeHost) read(_ string, offset, limit int) (*fetchChunk, error) {
	f.calls = append(f.calls, limit)
	if f.maxFit > 0 && limit > f.maxFit {
		return nil, hostResultError("fetch", "does not fit", hostCodeResponseTooLarge)
	}
	end := min(offset+limit, len(f.body))
	return &fetchChunk{Chunk: []byte(f.body[offset:end]), BodySize: len(f.body), Done: end == len(f.body)}, nil
}

func TestReadBodyChunks_Reassembles(t *testing.T) {
	body := strings.Repeat("0123456789", 100_000) // 1 MB, > several chunks
	h := &fakeHost{body: body}
	got, err := readBodyChunks("h", len(body), h.read)
	if err != nil || got != body {
		t.Fatalf("err=%v, equal=%v", err, got == body)
	}
	if len(h.calls) != 4 { // ceil(1_000_000 / 262_144)
		t.Errorf("calls = %d, want 4", len(h.calls))
	}
}

func TestReadBodyChunks_ShrinksOnTooLarge(t *testing.T) {
	body := strings.Repeat("x", 100_000)
	h := &fakeHost{body: body, maxFit: 20_000}
	got, err := readBodyChunks("h", len(body), h.read)
	if err != nil || got != body {
		t.Fatalf("err=%v, equal=%v", err, got == body)
	}
	if h.calls[0] != fetchChunkSize || h.calls[len(h.calls)-1] > 20_000 {
		t.Errorf("limits did not shrink as expected: %v", h.calls)
	}
}

func TestReadBodyChunks_GivesUpAtMinimum(t *testing.T) {
	h := &fakeHost{body: strings.Repeat("x", 100_000), maxFit: 100}
	_, err := readBodyChunks("h", 100_000, h.read)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
}

func TestReadBodyChunks_Errors(t *testing.T) {
	boom := errors.New("boom")
	if _, err := readBodyChunks("h", 5, func(string, int, int) (*fetchChunk, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("host error not propagated: %v", err)
	}
	empty := func(string, int, int) (*fetchChunk, error) { return &fetchChunk{}, nil }
	if _, err := readBodyChunks("h", 5, empty); err == nil || !strings.Contains(err.Error(), "empty chunk") {
		t.Errorf("empty chunk must not loop forever: %v", err)
	}
	short := func(string, int, int) (*fetchChunk, error) { return &fetchChunk{Chunk: []byte("ab"), Done: true}, nil }
	if _, err := readBodyChunks("h", 5, short); err == nil || !strings.Contains(err.Error(), "announced") {
		t.Errorf("size mismatch must be reported: %v", err)
	}
}

// The wire shape must match the host: chunk is base64, the meta envelope
// flattens FetchResponse next to body_handle.
func TestFetchWireShapes(t *testing.T) {
	var c fetchChunk
	if err := json.Unmarshal([]byte(`{"chunk":"aGVsbG8=","body_size":5,"done":true}`), &c); err != nil || string(c.Chunk) != "hello" || !c.Done {
		t.Fatalf("chunk decode: %v %+v", err, c)
	}
	var e fetchEnvelope
	if err := json.Unmarshal([]byte(`{"status":200,"headers":{"A":"b"},"body_handle":"h1","body_size":42}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Status != 200 || e.Headers["A"] != "b" || e.BodyHandle != "h1" || e.BodySize != 42 {
		t.Fatalf("envelope decode: %+v", e)
	}
}
