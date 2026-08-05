package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "respond":
		// respond takes a single --stateDir flag and reads the event on stdin.
		fs := flag.NewFlagSet("respond", flag.ExitOnError)
		stateDir := fs.String("stateDir", "", "state directory")
		port := fs.String("port", "7070", "server port (derives stateDir)")
		_ = fs.Parse(args)
		dir := *stateDir
		if dir == "" {
			dir = resolveStateDir("", "", *port)
		}
		runRespond(dir)
	case "start":
		runStart(args)
	case "stop":
		runStop(args)
	case "stub":
		runStub(args)
	case "presets":
		runPresetList(args)
	case "reset":
		runReset(args)
	case "requests":
		runRequests(args)
	case "verify":
		runVerify(args)
	case "command":
		runCommandStub(args)
	case "record":
		runRecord(args)
	case "invocations":
		runInvocations(args)
	case "verify-command":
		runVerifyCommand(args)
	case "reset-commands":
		runResetCommands(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: aux4-mock <command> [options]\n")
	fmt.Fprintf(os.Stderr, "Commands: respond, start, stop, stub, presets, reset, requests, verify, command, record, invocations, verify-command, reset-commands\n")
}
