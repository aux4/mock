package main

import (
	"bufio"
	"flag"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// commandLine matches a command entry in `aux4 <profile> --help` output: exactly
// two leading spaces followed by a single bare token (the command name). aux4
// indents command names by two spaces, their descriptions by two spaces but with
// prose (spaces/punctuation), and their variables by four+ spaces — so a bare
// two-space token is unambiguously a command name.
var commandLine = regexp.MustCompile(`^  ([a-zA-Z][a-zA-Z0-9_-]*)$`)

// runPresetList lists the service presets currently available under
// `mock preset`. Presets are ordinary aux4 commands contributed by installed
// `community/mock-preset-<service>` packages and merged into the
// `profile:mock:preset` profile via global.aux4. The core ships none, so with
// nothing installed this prints an install hint. Implemented by parsing
// `aux4 mock preset --help` (the merged profile), so it reflects exactly what is
// installed with zero built-in service knowledge.
func runPresetList(args []string) {
	fs := flag.NewFlagSet("presets", flag.ExitOnError)
	_ = fs.Parse(args)

	out, _ := exec.Command("aux4", "mock", "preset", "--help").CombinedOutput()

	seen := map[string]bool{}
	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		m := commandLine.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		fmt.Println("No presets installed.")
		fmt.Println("Install one, e.g.: aux4 aux4 pkger install community/mock-preset-google")
		fmt.Println("Then apply it with: aux4 mock preset google --port <port>")
		return
	}

	fmt.Println("Installed presets (apply with: aux4 mock preset <service> --port <port>):")
	for _, n := range names {
		fmt.Printf("  %s\n", n)
	}
}
