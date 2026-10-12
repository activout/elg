package secrets

import (
	"fmt"
	"os"
)

type Destination struct {
	file     *os.File
	path     string
	complete bool
	preserve bool
}

// Reserve prevents overwrites and discovers unusable paths before mutation.
func Reserve(path string) (*Destination, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot reserve secret output file: %w", err)
	}
	return &Destination{file: f, path: path}, nil
}
func (d *Destination) Write(secret string) error {
	d.preserve = true
	if _, err := d.file.WriteString(secret); err != nil {
		return err
	}
	if err := d.file.Sync(); err != nil {
		return err
	}
	if err := d.file.Close(); err != nil {
		return err
	}
	d.complete = true
	return nil
}
func (d *Destination) Close() {
	if !d.complete {
		_ = d.file.Close()
		if !d.preserve {
			_ = os.Remove(d.path)
		}
	}
}
