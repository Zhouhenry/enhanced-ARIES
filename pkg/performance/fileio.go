package performance

import "io"

// FileIOCounters counts calls at a particular file boundary, not events or
// physical disk operations. Its single writer owns mutation; read after close.
// Metadata files and other processes are not included in this scope.
type FileIOCounters struct {
	Scope          string `json:"measurement_scope"`
	WritesStarted  uint64 `json:"write_calls_started"`
	WritesReturned uint64 `json:"write_calls_returned"`
	RequestedBytes uint64 `json:"write_requested_bytes"`
	ReturnedBytes  uint64 `json:"write_returned_bytes"`
	WriteErrors    uint64 `json:"write_errors"`
	ShortWrites    uint64 `json:"short_writes"`
	InvalidReturns uint64 `json:"invalid_write_returns"`
	SyncsStarted   uint64 `json:"sync_calls_started"`
	SyncsReturned  uint64 `json:"sync_calls_returned"`
	SyncErrors     uint64 `json:"sync_errors"`
}

func (c *FileIOCounters) Write(file io.Writer, data []byte) (int, error) {
	c.WritesStarted++
	c.RequestedBytes += uint64(len(data))
	n, err := file.Write(data)
	c.WritesReturned++
	if n < 0 || n > len(data) {
		c.InvalidReturns++
	} else {
		c.ReturnedBytes += uint64(n)
		if n < len(data) {
			c.ShortWrites++
		}
	}
	if err != nil {
		c.WriteErrors++
	}
	return n, err // Preserve caller handling, including short writes with nil error.
}

func (c *FileIOCounters) Sync(file interface{ Sync() error }) error {
	c.SyncsStarted++
	err := file.Sync()
	c.SyncsReturned++
	if err != nil {
		c.SyncErrors++
	}
	return err
}
