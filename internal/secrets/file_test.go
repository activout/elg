package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReservationNoOverwriteCleanupAndDeliveryFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	d, err := Reserve(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Reserve(path); err == nil {
		t.Fatal("overwrote reserved file")
	}
	d.Close()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unused reservation not removed")
	}
	d, err = Reserve(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = d.file.Close()
	if err = d.Write("secret"); err == nil {
		t.Fatal("closed-file write unexpectedly succeeded")
	}
	d.Close()
	if _, err = os.Stat(path); err != nil {
		t.Fatal("delivery failure removed partial file")
	}
}
