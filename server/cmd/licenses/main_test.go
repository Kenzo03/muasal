package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The scanner names a licence from its text and flags what Zettra may not ship.
func TestClassify(t *testing.T) {
	for want, text := range map[string]string{
		"MIT":             "Permission is hereby granted, free of charge, to any person obtaining a copy",
		"Apache-2.0":      "                                 Apache License\n                           Version 2.0, January 2004",
		"BSD-3-Clause":    "Redistribution and use in source and binary forms, with or without\n... Neither the name of the copyright holder",
		"GPL":             "GNU GENERAL PUBLIC LICENSE\nVersion 3",
		"AGPL":            "GNU AFFERO GENERAL PUBLIC LICENSE",
		"MPL-2.0":         "Mozilla Public License Version 2.0",
		"no licence file": "",
	} {
		dir := t.TempDir()
		if text != "" {
			if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := classify(dir); got != want {
			t.Errorf("%q: got %q", want, got)
		}
	}
	if allowed["GPL"] || allowed["AGPL"] || !allowed["MPL-2.0"] {
		t.Error("allowed list is wrong")
	}
}
