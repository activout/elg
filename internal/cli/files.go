package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const maxInputBytes = 4 << 20

func readFile(path string, stdin io.Reader) ([]byte, error) {
	var r io.Reader = stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, invalid(err.Error())
		}
		defer f.Close()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, maxInputBytes+1))
	if err != nil {
		return nil, invalid(err.Error())
	}
	if len(b) > maxInputBytes {
		return nil, invalid("input exceeds 4 MiB")
	}
	return b, nil
}
func decodeFile(path string, stdin io.Reader, v any) error {
	b, err := readFile(path, stdin)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return invalid("invalid JSON input: " + err.Error())
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return invalid("input must contain exactly one JSON value")
	}
	return nil
}
func textFile(path string) (string, error) {
	if path == "-" {
		return "", invalid("this file option does not accept stdin")
	}
	b, err := readFile(path, nil)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", invalid("text input must be valid UTF-8")
	}
	return string(b), nil
}
func passwordFile(path string) (string, error) {
	s, err := textFile(path)
	if err != nil {
		return "", err
	}
	// Remove one conventional file line ending, never arbitrary whitespace.
	if strings.HasSuffix(s, "\r\n") {
		s = strings.TrimSuffix(s, "\r\n")
	} else {
		s = strings.TrimSuffix(s, "\n")
	}
	if s == "" {
		return "", invalid("password file must not be empty")
	}
	return s, nil
}
