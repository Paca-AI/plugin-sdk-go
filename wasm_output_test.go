package plugin

import "testing"

func TestDecodeQuery2Output(t *testing.T) {
	tests := []struct {
		name                                           string
		buf                                            []byte
		wantResPtr, wantResLen, wantErrPtr, wantErrLen int32
	}{
		{
			name:       "success with rows",
			buf:        []byte{0x10, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0, 0, 0, 0, 0, 0, 0, 0},
			wantResPtr: 0x10,
			wantResLen: 0x20,
			wantErrPtr: 0,
			wantErrLen: 0,
		},
		{
			name:       "success with zero rows",
			buf:        make([]byte, 16),
			wantResPtr: 0,
			wantResLen: 0,
			wantErrPtr: 0,
			wantErrLen: 0,
		},
		{
			name:       "execution error",
			buf:        []byte{0, 0, 0, 0, 0, 0, 0, 0, 0x40, 0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00},
			wantResPtr: 0,
			wantResLen: 0,
			wantErrPtr: 0x40,
			wantErrLen: 0x05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resPtr, resLen, errPtr, errLen := decodeQuery2Output(tt.buf)
			if resPtr != tt.wantResPtr || resLen != tt.wantResLen || errPtr != tt.wantErrPtr || errLen != tt.wantErrLen {
				t.Fatalf("decodeQuery2Output(%v) = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					tt.buf, resPtr, resLen, errPtr, errLen, tt.wantResPtr, tt.wantResLen, tt.wantErrPtr, tt.wantErrLen)
			}
		})
	}
}
