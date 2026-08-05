package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Mocking an aux4 command, as opposed to an HTTP endpoint.
//
// HTTP stubbing only reaches packages that expose an API URL override; most do not. aux4
// core's `replace` hook runs instead of a command's own execute steps, so any command can be
// stubbed without the package under test knowing about it. Stubs are written as hooks into a
// generated .aux4 inside the mock's state directory, and aux4 loads it via AUX4_AUX4_FILES —
// so nothing is written to the user's own .aux4 and nothing depends on the working directory.

// CommandStub is a registered stand-in for one aux4 command.
type CommandStub struct {
	ID       string            `json:"id"`
	Command  string            `json:"command"`
	Params   map[string]string `json:"params,omitempty"`
	Output   string            `json:"output,omitempty"`
	ExitCode int               `json:"exitCode,omitempty"`
}

// RecordedInvocation is one call that a stub stood in for.
type RecordedInvocation struct {
	Command     string `json:"command"`
	CommandLine string `json:"commandLine,omitempty"`
	Params      string `json:"params,omitempty"`
	Timestamp   string `json:"ts"`
}

func commandStubsPath(stateDir string) string { return filepath.Join(stateDir, "command-stubs.json") }
func invocationsPath(stateDir string) string  { return filepath.Join(stateDir, "invocations.json") }
func commandsAux4Path(stateDir string) string { return filepath.Join(stateDir, "commands.aux4") }

func loadCommandStubs(stateDir string) []CommandStub {
	data, err := os.ReadFile(commandStubsPath(stateDir))
	if err != nil {
		return []CommandStub{}
	}
	var stubs []CommandStub
	if err := json.Unmarshal(data, &stubs); err != nil {
		return []CommandStub{}
	}
	return stubs
}

func saveCommandStubs(stateDir string, stubs []CommandStub) error {
	data, err := json.MarshalIndent(stubs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(commandStubsPath(stateDir), data, 0600)
}

func loadInvocations(stateDir string) []RecordedInvocation {
	data, err := os.ReadFile(invocationsPath(stateDir))
	if err != nil {
		return []RecordedInvocation{}
	}
	var calls []RecordedInvocation
	if err := json.Unmarshal(data, &calls); err != nil {
		return []RecordedInvocation{}
	}
	return calls
}

func saveInvocations(stateDir string, calls []RecordedInvocation) error {
	data, err := json.MarshalIndent(calls, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(invocationsPath(stateDir), data, 0600)
}

// writeCommandsAux4 regenerates the .aux4 that carries the stubs as replace hooks.
//
// Each hook delegates to `aux4 mock record`, which records the invocation and prints the
// stub's output. Keeping the output out of the hook step means no shell quoting of arbitrary
// payloads (JSON, quotes, newlines) — the value only ever travels through a file.
func writeCommandsAux4(stateDir string, stubs []CommandStub) error {
	type hook struct {
		Command string            `json:"command"`
		Params  map[string]string `json:"params,omitempty"`
		Replace []string          `json:"replace"`
	}
	doc := struct {
		Hooks []hook `json:"hooks"`
	}{}

	for _, stub := range stubs {
		// --name=value, not --name value: the recorded parameters start with "--", and aux4
		// reads a flag whose next argument begins with "-" as a boolean, which would record
		// "true" instead of the arguments. value() does the shell quoting, so a payload
		// containing quotes or spaces survives intact.
		step := fmt.Sprintf(
			"aux4 mock record --stateDir %s --stub %s --command=value(__command) --commandLine=value(__commandLine) --params=value(__params)",
			shellQuote(stateDir), stub.ID)
		doc.Hooks = append(doc.Hooks, hook{Command: stub.Command, Params: stub.Params, Replace: []string{step}})
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(commandsAux4Path(stateDir), data, 0600)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// runCommandStub registers a stand-in for an aux4 command.
func runCommandStub(args []string) {
	fs := flag.NewFlagSet("command", flag.ExitOnError)
	name := fs.String("name", "", "stable handle for this set of stubs (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	command := fs.String("command", "", "command pattern to stub, e.g. google:gmail/list")
	output := fs.String("output", "", "text the stubbed command prints")
	outputFile := fs.String("outputFile", "", "file whose contents the stubbed command prints")
	exitCode := fs.Int("exitCode", 0, "exit code the stubbed command returns")
	var params stringList
	fs.Var(&params, "param", "only stub when a parameter matches, as key=value (repeatable)")
	_ = fs.Parse(args)

	if strings.TrimSpace(*command) == "" {
		fmt.Fprintln(os.Stderr, "Error: --command is required (e.g. --command google:gmail/list)")
		os.Exit(2)
	}

	body := *output
	if *outputFile != "" {
		data, err := os.ReadFile(*outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot read --outputFile %s: %v\n", *outputFile, err)
			os.Exit(2)
		}
		body = string(data)
	}

	dir := resolveCommandStateDir(*stateDir, *name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state directory: %v\n", err)
		os.Exit(1)
	}

	stubs := loadCommandStubs(dir)
	stub := CommandStub{
		ID:       fmt.Sprintf("stub-%d", len(stubs)+1),
		Command:  strings.TrimSpace(*command),
		Params:   parseHeaderPairs(params),
		Output:   body,
		ExitCode: *exitCode,
	}
	stubs = append(stubs, stub)

	if err := saveCommandStubs(dir, stubs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot save command stubs: %v\n", err)
		os.Exit(1)
	}
	if err := writeCommandsAux4(dir, stubs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot write commands.aux4: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("stubbed command %s\n", stub.Command)
	fmt.Printf("AUX4_AUX4_FILES=%s\n", commandsAux4Path(dir))
}

// runRecord is what a stub's replace hook calls: it records the invocation and prints the
// stub's output. Doing both here keeps the generated hook to a single step and means an
// arbitrary payload never has to survive shell quoting.
func runRecord(args []string) {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	name := fs.String("name", "", "stable handle for this set of stubs (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	stub := fs.String("stub", "", "id of the stub standing in for the command")
	command := fs.String("command", "", "command that was called")
	commandLine := fs.String("commandLine", "", "command line as typed")
	params := fs.String("params", "", "parameters as typed")
	_ = fs.Parse(args)

	dir := resolveCommandStateDir(*stateDir, *name)

	unlock, err := acquireLock(dir)
	if err == nil {
		defer unlock()
	}

	calls := append(loadInvocations(dir), RecordedInvocation{
		Command:     *command,
		CommandLine: *commandLine,
		Params:      *params,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})
	if err := saveInvocations(dir, calls); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot record invocation: %v\n", err)
	}

	for _, s := range loadCommandStubs(dir) {
		if s.ID != *stub {
			continue
		}
		if s.Output != "" {
			fmt.Print(s.Output)
			if !strings.HasSuffix(s.Output, "\n") {
				fmt.Println()
			}
		}
		if s.ExitCode != 0 {
			os.Exit(s.ExitCode)
		}
		return
	}
}

// runInvocations prints the recorded command invocations.
func runInvocations(args []string) {
	fs := flag.NewFlagSet("invocations", flag.ExitOnError)
	name := fs.String("name", "", "stable handle for this set of stubs (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	_ = fs.Parse(args)

	dir := resolveCommandStateDir(*stateDir, *name)
	data, err := json.MarshalIndent(loadInvocations(dir), "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// runVerifyCommand asserts that a command was invoked, mirroring `verify` for HTTP.
func runVerifyCommand(args []string) {
	fs := flag.NewFlagSet("verify-command", flag.ExitOnError)
	name := fs.String("name", "", "stable handle for this set of stubs (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	command := fs.String("command", "", "match by command")
	timesStr := fs.String("times", "", "exact number of matches (omit = at least 1)")
	var paramsContains stringList
	fs.Var(&paramsContains, "params-contains", "require the parameters to contain this substring (repeatable, AND)")
	_ = fs.Parse(args)

	dir := resolveCommandStateDir(*stateDir, *name)
	calls := loadInvocations(dir)
	wantParams := nonEmpty(paramsContains)

	times := -1
	if strings.TrimSpace(*timesStr) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(*timesStr))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: --times must be an integer, got %q\n", *timesStr)
			os.Exit(2)
		}
		times = n
	}

	matches := 0
	for _, call := range calls {
		if *command != "" && call.Command != strings.TrimSpace(*command) {
			continue
		}
		if !containsAll(call.Params, wantParams) {
			continue
		}
		matches++
	}

	ok := matches >= 1
	if times >= 0 {
		ok = matches == times
	}

	if ok {
		fmt.Printf("verify ok: %d match(es)\n", matches)
		return
	}

	expected := "at least 1"
	if times >= 0 {
		expected = fmt.Sprintf("exactly %d", times)
	}
	fmt.Fprintf(os.Stderr, "verify failed: expected %s match(es), found %d\n", expected, matches)
	fmt.Fprintf(os.Stderr, "criteria: command=%q params-contains=%v\n", *command, wantParams)
	fmt.Fprintf(os.Stderr, "recorded %d invocation(s):\n", len(calls))
	for _, call := range calls {
		fmt.Fprintf(os.Stderr, "  %s %s\n", call.Command, call.Params)
	}
	os.Exit(1)
}

func containsAll(value string, substrings []string) bool {
	for _, want := range substrings {
		if !strings.Contains(value, want) {
			return false
		}
	}
	return true
}

// runResetCommands clears command stubs and invocations. Separate from `reset` because
// command mocking has no server: addressing it by port would only invite starting one.
func runResetCommands(args []string) {
	fs := flag.NewFlagSet("reset-commands", flag.ExitOnError)
	name := fs.String("name", "", "stable handle for this set of stubs (derives stateDir)")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	stubs := fs.String("stubs", "", "clear command stubs only (true)")
	invocations := fs.String("invocations", "", "clear recorded invocations only (true)")
	_ = fs.Parse(args)

	dir := resolveCommandStateDir(*stateDir, *name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state directory: %v\n", err)
		os.Exit(1)
	}

	clearStubs := *stubs == "true"
	clearInvocations := *invocations == "true"
	if !clearStubs && !clearInvocations {
		clearStubs = true
		clearInvocations = true
	}

	if clearStubs {
		_ = saveCommandStubs(dir, []CommandStub{})
		_ = writeCommandsAux4(dir, []CommandStub{})
	}
	if clearInvocations {
		_ = saveInvocations(dir, []RecordedInvocation{})
	}
	fmt.Println("reset ok")
}
