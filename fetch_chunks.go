package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// A fetch response whose envelope does not fit the plugin's host buffer is
// delivered in pieces: the host replies with status/headers plus a
// body_handle, and the plugin pulls the body with {"body_handle","offset",
// "limit"} requests, each chunk carried as base64 in the JSON envelope. This
// file holds the transport-independent half so it can be unit tested without
// a WASM runtime; wasm_backends.go supplies the real host call.

const (
	// fetchChunkSize is the raw bytes requested per chunk (~350 KiB once
	// base64-encoded), small enough to fit a fresh 1 MiB buffer with room to
	// spare.
	fetchChunkSize = 256 * 1024
	// minFetchChunkSize is where retries stop shrinking the chunk.
	minFetchChunkSize = 4 * 1024
	// maxFetchBodyBytes caps the body size a host may announce, so a bad
	// body_size becomes an error instead of an allocation that traps the
	// guest. It sits above the host's own 50 MiB per-response cap.
	maxFetchBodyBytes = 64 << 20
)

// fetchEnvelope is the paca.fetch response: a [FetchResponse], plus the
// chunking fields set when the body was parked host-side.
type fetchEnvelope struct {
	FetchResponse
	BodyHandle string `json:"body_handle"`
	BodySize   int    `json:"body_size"`
}

// fetchChunk is the response to a body_handle read.
type fetchChunk struct {
	Chunk    []byte `json:"chunk"` // base64 on the wire
	BodySize int    `json:"body_size"`
	Done     bool   `json:"done"`
	Error    string `json:"error"`
	Code     string `json:"code"`
}

// chunkReader performs one body_handle read against the host.
type chunkReader func(handle string, offset, limit int) (*fetchChunk, error)

// readBodyChunks drains a parked body of size bytes. If a chunk does not fit
// the host buffer it halves the chunk size and retries, down to
// minFetchChunkSize, so a plugin whose buffer is partly used still succeeds.
func readBodyChunks(handle string, size int, read chunkReader) (string, error) {
	if size < 0 || size > maxFetchBodyBytes {
		return "", fmt.Errorf("plugin: fetch: %w: host announced a body of %d bytes (limit %d)", ErrResponseTooLarge, size, maxFetchBodyBytes)
	}
	var body strings.Builder
	body.Grow(size)
	limit := fetchChunkSize
	for {
		c, err := read(handle, body.Len(), limit)
		if err != nil {
			// A reply that did not fit, or no data at all (an exhausted
			// buffer on a host that cannot say so), may succeed smaller.
			if (errors.Is(err, ErrResponseTooLarge) || errors.Is(err, errHostNoData)) && limit > minFetchChunkSize {
				limit /= 2
				continue
			}
			return "", err
		}
		if body.Len()+len(c.Chunk) > size {
			return "", fmt.Errorf("plugin: fetch: host sent more than the announced %d bytes", size)
		}
		body.Write(c.Chunk)
		if c.Done {
			if body.Len() != size {
				return "", fmt.Errorf("plugin: fetch: body is %d bytes, host announced %d", body.Len(), size)
			}
			return body.String(), nil
		}
		if len(c.Chunk) == 0 {
			return "", fmt.Errorf("plugin: fetch: host returned an empty chunk at offset %d of %d", body.Len(), size)
		}
	}
}
