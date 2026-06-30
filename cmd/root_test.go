package cmd

import (
	"bytes"
	"testing"
)

func TestRootVersionFlag(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--version"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("azrl")) {
		t.Fatalf("version output missing 'azrl': %q", buf.String())
	}
}
