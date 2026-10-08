package historypage

const DefaultPageSize = 128
const MaxPageSize = 1024

// PageSize preserves existing clients' sequence windows while allowing larger
// replay batches. Clamp before converting to int for 32-bit WASM runtimes.
func PageSize(requested uint32) int {
	if requested == 0 {
		return DefaultPageSize
	}
	return int(min(requested, uint32(MaxPageSize)))
}
