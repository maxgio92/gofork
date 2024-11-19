package utils

import (
	"bytes"
	"io"
)

func Read(reader io.Reader) (string, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, reader)

	return buf.String(), err
}
