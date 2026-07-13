package updater

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// gunzip decompresses gzip data and returns the raw bytes.
func gunzip(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()
	out, err := io.ReadAll(io.LimitReader(gz, 64<<20)) // 64 MiB cap
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}
	return out, nil
}
