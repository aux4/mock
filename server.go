package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// freePort asks the OS for an unused TCP port by binding :0, then releases it
// so the mock's aux4/api child can claim it. There is a small window between
// release and re-bind, but for local test servers it is more than adequate and
// removes any need for callers to hand-pick collision-free ports.
func freePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}

// resolveStartPort decides which port the server should bind. An explicit
// numeric port is used verbatim (back-compat). "0", "auto", or an empty value
// combined with --name auto-selects a free ephemeral port. An empty value with
// no name falls back to the historical default of 7070.
func resolveStartPort(port, name string) string {
	p := strings.TrimSpace(port)
	if p == "0" || strings.EqualFold(p, "auto") {
		if chosen, err := freePort(); err == nil {
			return chosen
		}
	}
	if p == "" {
		if name != "" {
			if chosen, err := freePort(); err == nil {
				return chosen
			}
		}
		return "7070"
	}
	return p
}

// runStart launches an aux4/api server whose single catch-all route runs
// "aux4 mock respond", detaches it, and records its pid. The server keeps
// running across tests; stubs are reprogrammed at runtime.
func runStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	port := fs.String("port", "", "port to run the mock server on (0/auto = free port; empty = 7070 or free when --name is set)")
	name := fs.String("name", "", "stable handle to address this server without its port")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	timeout := fs.Int("timeout", 15000, "per-request handler timeout (ms)")
	_ = fs.Parse(args)

	actualPort := resolveStartPort(*port, *name)

	dir := resolveStateDir(*stateDir, *name, actualPort)

	// Fresh state: recreate dir, empty stub store + journal, drop stale pid/lock.
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create state dir: %v\n", err)
		os.Exit(1)
	}
	os.Remove(pidPath(dir))
	os.Remove(lockPath(dir))
	_ = saveStubs(dir, []Stub{})
	_ = saveRequests(dir, []RecordedRequest{})
	_ = savePort(dir, actualPort)

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
		"--port", actualPort,
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

	// Line 1 is the full base URL (point the code-under-test here); line 2 is
	// the bare chosen port so a test can capture it directly (e.g. with
	// `tail -1`) after an auto/free-port start.
	fmt.Printf("http://localhost:%s\n", actualPort)
	fmt.Println(actualPort)
}

// runStop terminates the server (via pidfile process group, with a pkill
// fallback) and removes the state directory.
func runStop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	port := fs.String("port", "7070", "port of the mock server to stop")
	name := fs.String("name", "", "stable handle of the server to stop")
	stateDir := fs.String("stateDir", "", "explicit state directory")
	keep := fs.Bool("keep", false, "keep the state directory after stopping")
	_ = fs.Parse(args)

	dir := resolveStateDir(*stateDir, *name, *port)

	// The port actually bound may differ from --port (auto/free-port or
	// name-addressed). Prefer the persisted port for the pkill fallback.
	killPort := *port
	if persisted := loadPort(dir); persisted != "" {
		killPort = persisted
	}

	if data, err := os.ReadFile(pidPath(dir)); err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil {
			killGroup(pid)
		}
	}

	// Fallback: kill anything still bound to this server's identity.
	_ = exec.Command("pkill", "-f", "api start --port "+killPort).Run()
	_ = exec.Command("pkill", "-f", dir).Run()

	if !*keep {
		os.RemoveAll(dir)
	}
	fmt.Printf("stopped mock server on port %s\n", killPort)
}
