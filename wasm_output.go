package plugin

// decodeQuery2Output parses the 16-byte output buffer written by the
// paca.db_query2 host import: [0:4]=resultPtr [4:8]=resultLen
// [8:12]=errPtr [12:16]=errLen. Kept in a non-wasip1-tagged file so it can
// be unit tested on the host build; wasm_backends.go (wasip1-only) is the
// real caller.
func decodeQuery2Output(buf []byte) (resPtr, resLen, errPtr, errLen int32) {
	resPtr = int32(uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24)
	resLen = int32(uint32(buf[4]) | uint32(buf[5])<<8 | uint32(buf[6])<<16 | uint32(buf[7])<<24)
	errPtr = int32(uint32(buf[8]) | uint32(buf[9])<<8 | uint32(buf[10])<<16 | uint32(buf[11])<<24)
	errLen = int32(uint32(buf[12]) | uint32(buf[13])<<8 | uint32(buf[14])<<16 | uint32(buf[15])<<24)
	return
}
