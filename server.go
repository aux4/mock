package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// runStart launches an aux4/api server whose single catch-all route runs
// "aux4 mock respond", detaches it, and records its pid. The server keeps
// running across tests; stubs are reprogrammed at runtime.
func runStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	port := fs.String("port", "7070", "port to run the mock server on")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	timeout := fs.Int("timeout", 15000, "per-request handler timeout (ms)")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *port)

	// Fresh state: recreate dir, empty stub store + journal, drop stale pid/lock.
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state dir: %v\n", err)
		os.Exit(1)
	}
	os.Remove(pidPath(dir))
	os.Remove(lockPath(dir))
	_ = saveStubs(dir, []Stub{})
	_ = saveRequests(dir, []RecordedRequest{})

	// Build the catch-all route. body is a JSON string per API-Gateway shape.
	route := map[string]map[string]any{
		"ANY /{path...}": {
			"command": fmt.Sprintf("aux4 mock respond --stateDir %s", dir),
			"public":  true,
		},
	}
	routeJSON, _ := json.Marshal(route)
	serverJSON := fmt.Sprintf(`{"timeout":%d}`, *timeout)

	cmd := exec.Command("aux4", "api", "start",
		"--port", *port,
		"--api", string(routeJSON),
		"--server", serverJSON,
	)
	cmd.SysProcAttr = detachAttr()

	logFile, err := os.Create(strings.TrimRight(dir, "/") + "/server.log")
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting api server: %v\n", err)
		os.Exit(1)
	}

	_ = os.WriteFile(pidPath(dir), []byte(strconv.Itoa(cmd.Process.Pid)), 0600)

	// Detach: do not Wait, let the server run independently.
	_ = cmd.Process.Release()

	fmt.Printf("http://localhost:%s\n", *port)
}

// runStop terminates the server (via pidfile process group, with a pkill
// fallback) and removes the state directory.
func runStop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	port := fs.String("port", "7070", "port of the mock server to stop")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	keep := fs.Bool("keep", false, "keep the state directory after stopping")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *port)

	if data, err := os.ReadFile(pidPath(dir)); err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil {
			killGroup(pid)
		}
	}

	// Fallback: kill anything still bound to this server's identity.
	_ = exec.Command("pkill", "-f", "api start --port "+*port).Run()
	_ = exec.Command("pkill", "-f", dir).Run()

	if !*keep {
		os.RemoveAll(dir)
	}
	fmt.Printf("stopped mock server on port %s\n", *port)
}
