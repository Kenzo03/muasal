// Command licenses checks that every Go module the server builds with has a
// licence Muasal may ship under Apache-2.0 (FSD §18: licence scan in CI).
//
//	go run ./cmd/licenses          lists modules and licences; exit 1 on a problem
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// allowed licences are permissive: they may be shipped inside an Apache-2.0
// product without changing its terms.
var allowed = map[string]bool{"Apache-2.0": true, "MIT": true, "BSD-2-Clause": true, "BSD-3-Clause": true, "ISC": true, "MPL-2.0": true}

// kinds recognises a licence from its text; the first match wins.
var kinds = []struct {
	name string
	re   *regexp.Regexp
}{
	{"AGPL", regexp.MustCompile(`GNU AFFERO GENERAL PUBLIC LICENSE`)},
	{"GPL", regexp.MustCompile(`GNU GENERAL PUBLIC LICENSE`)},
	{"LGPL", regexp.MustCompile(`GNU LESSER GENERAL PUBLIC LICENSE`)},
	{"MPL-2.0", regexp.MustCompile(`Mozilla Public License,? [Vv]ersion 2\.0`)},
	{"Apache-2.0", regexp.MustCompile(`Apache License[\s\S]{0,40}Version 2\.0`)},
	{"MIT", regexp.MustCompile(`Permission is hereby granted, free of charge`)},
	{"BSD-3-Clause", regexp.MustCompile(`(?s)Redistribution and use in source and binary forms.*Neither the name`)},
	{"BSD-2-Clause", regexp.MustCompile(`Redistribution and use in source and binary forms`)},
	{"ISC", regexp.MustCompile(`Permission to use, copy, modify, and(/or)? distribute this software for any`)},
}

type module struct {
	Path, Version, Dir string
	Main, Indirect     bool
}

func main() {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", "./...").Output()
	if err != nil {
		fail(err)
	}
	used := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if p := strings.TrimSpace(sc.Text()); p != "" {
			used[p] = true
		}
	}
	raw, err := exec.Command("go", "list", "-m", "-json", "all").Output()
	if err != nil {
		fail(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var problems []string
	var rows []string
	for {
		var m module
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			fail(err)
		}
		if m.Main || !used[m.Path] {
			continue // only modules compiled into the server
		}
		lic := classify(m.Dir)
		rows = append(rows, fmt.Sprintf("%-60s %-24s %s", m.Path, m.Version, lic))
		if !allowed[lic] {
			problems = append(problems, fmt.Sprintf("%s %s: %s", m.Path, m.Version, lic))
		}
	}
	sort.Strings(rows)
	fmt.Println(strings.Join(rows, "\n"))
	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "\nNot allowed:\n  "+strings.Join(problems, "\n  "))
		os.Exit(1)
	}
}

func classify(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, f := range files {
		base := strings.ToUpper(filepath.Base(f))
		if !strings.HasPrefix(base, "LICENSE") && !strings.HasPrefix(base, "LICENCE") && !strings.HasPrefix(base, "COPYING") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, k := range kinds {
			if k.re.Match(b) {
				return k.name
			}
		}
		return "unrecognised (" + filepath.Base(f) + ")"
	}
	return "no licence file"
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "licenses:", err)
	os.Exit(2)
}
