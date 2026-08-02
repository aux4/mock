package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// apiGatewayEvent is the subset of the AWS-API-Gateway event that aux4/api
// delivers on stdin to a mounted command.
type apiGatewayEvent struct {
	HTTPMethod     string            `json:"httpMethod"`
	PathParameters map[string]string `json:"pathParameters"`
	Query          map[string]string `json:"queryStringParameters"`
	Headers        map[string]string `json:"headers"`
	Body           string            `json:"body"`
}

// apiGatewayResponse is the shape aux4/api turns into an HTTP response.
type apiGatewayResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

func writeResponse(resp apiGatewayResponse) {
	if resp.Headers == nil {
		resp.Headers = map[string]string{}
	}
	out, _ := json.Marshal(resp)
	fmt.Println(string(out))
}

// runRespond is the mounted catch-all handler. It reads the event from stdin,
// records the request, finds the first matching stub, and prints the response.
// It must never exit non-zero on bad state — that would surface as a 500.
func runRespond(stateDir string) {
	if stateDir == "" {
		writeResponse(apiGatewayResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"mock respond: --stateDir is required"}`,
		})
		return
	}

	raw, _ := io.ReadAll(os.Stdin)
	var event apiGatewayEvent
	_ = json.Unmarshal(raw, &event)

	method := event.HTTPMethod
	if method == "" {
		method = "GET"
	}
	path := "/"
	if event.PathParameters != nil {
		if p, ok := event.PathParameters["path"]; ok {
			path = normalizePath(p)
		}
	}

	// Record the request first so verify sees even unmatched calls.
	_ = appendRequest(stateDir, RecordedRequest{
		Method:  method,
		Path:    path,
		Query:   event.Query,
		Headers: event.Headers,
		Body:    event.Body,
		Ts:      nowTs(),
	})

	stubs := loadStubs(stateDir)
	for _, stub := range stubs {
		if stubMatches(stub, method, path) {
			headers := stub.Headers
			if headers == nil {
				headers = map[string]string{}
			}
			status := stub.Status
			if status == 0 {
				status = 200
			}
			writeResponse(apiGatewayResponse{
				StatusCode: status,
				Headers:    headers,
				Body:       stub.Body,
			})
			return
		}
	}

	// No match: 404, but the request was still recorded above.
	body, _ := json.Marshal(map[string]string{
		"error":  "no matching stub",
		"method": method,
		"path":   path,
	})
	writeResponse(apiGatewayResponse{
		StatusCode: 404,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	})
}

func stubMatches(stub Stub, method, path string) bool {
	if stub.Path != path {
		return false
	}
	if stub.Method == "" || stub.Method == "ANY" {
		return true
	}
	return stub.Method == method
}
