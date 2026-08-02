package main

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// paramToken matches a {name} or {name...} token inside a stub path.
var paramToken = regexp.MustCompile(`\{([^}]+)\}`)

// pathPattern is a compiled stub path. It carries the anchored regexp used to
// match incoming request paths (with named capture groups for {name} tokens)
// plus the metadata used to rank stubs by specificity.
type pathPattern struct {
	re         *regexp.Regexp
	staticSegs int  // number of path segments that contain no {token}
	hasGreedy  bool // path contains a {name...} greedy wildcard
	hasParam   bool // path contains any {token}
}

// compilePathPattern turns a stub path into a pathPattern. Tokens mirror
// aux4/api: {name} matches exactly one path segment ([^/]+) and {name...}
// greedily matches the rest of the path including slashes (.*). Static text is
// escaped so a plain path still matches exactly as before. If the resulting
// expression fails to compile (an odd token name), it falls back to an exact
// literal match on the raw path so a bad pattern can never crash respond.
func compilePathPattern(pattern string) *pathPattern {
	var b strings.Builder
	b.WriteString("^")
	hasGreedy := false
	hasParam := false
	last := 0
	for _, idx := range paramToken.FindAllStringSubmatchIndex(pattern, -1) {
		b.WriteString(regexp.QuoteMeta(pattern[last:idx[0]]))
		name := pattern[idx[2]:idx[3]]
		hasParam = true
		if strings.HasSuffix(name, "...") {
			name = strings.TrimSuffix(name, "...")
			b.WriteString("(?P<" + name + ">.*)")
			hasGreedy = true
		} else {
			b.WriteString("(?P<" + name + ">[^/]+)")
		}
		last = idx[1]
	}
	b.WriteString(regexp.QuoteMeta(pattern[last:]))
	b.WriteString("$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		re = regexp.MustCompile("^" + regexp.QuoteMeta(pattern) + "$")
		hasParam = false
		hasGreedy = false
	}

	staticSegs := 0
	for _, seg := range strings.Split(pattern, "/") {
		if seg != "" && !strings.Contains(seg, "{") {
			staticSegs++
		}
	}

	return &pathPattern{re: re, staticSegs: staticSegs, hasGreedy: hasGreedy, hasParam: hasParam}
}

// matchPath tests an incoming path against the compiled pattern and returns the
// captured named params (empty map when the pattern has no tokens).
func (p *pathPattern) matchPath(path string) (bool, map[string]string) {
	m := p.re.FindStringSubmatch(path)
	if m == nil {
		return false, nil
	}
	params := map[string]string{}
	for i, name := range p.re.SubexpNames() {
		if i > 0 && name != "" {
			params[name] = m[i]
		}
	}
	return true, params
}

// interpolate substitutes ${...} placeholders in s using the current request.
// Interpolation is always on. Supported placeholders:
//
//	${path.<name>}          a named path param captured by the stub pattern
//	${query.<name>}         a query-string parameter
//	${header.<name>}        a request header (case-insensitive name)
//	${body.<dotted.path>}   a field of the JSON request body (nested via dots)
//	${body}                 the whole raw request body
//	${path}                 the whole request path
//
// Unknown or unresolved placeholders collapse to the empty string (the literal
// ${...} is never left behind). A literal ${ can be emitted by writing $${.
// The stub author owns any surrounding JSON quoting.
func interpolate(s string, params map[string]string, event apiGatewayEvent, fullPath string) string {
	if !strings.Contains(s, "${") {
		return s
	}

	var bodyJSON interface{}
	if len(event.Body) > 0 && json.Valid([]byte(event.Body)) {
		_ = json.Unmarshal([]byte(event.Body), &bodyJSON)
	}

	var out strings.Builder
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "$${") {
			out.WriteString("${")
			i += 3
			continue
		}
		if strings.HasPrefix(s[i:], "${") {
			end := strings.Index(s[i+2:], "}")
			if end < 0 {
				out.WriteString(s[i:])
				break
			}
			expr := strings.TrimSpace(s[i+2 : i+2+end])
			out.WriteString(resolvePlaceholder(expr, params, event, fullPath, bodyJSON))
			i = i + 2 + end + 1
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// resolvePlaceholder resolves a single placeholder expression to its value, or
// the empty string when it cannot be resolved.
func resolvePlaceholder(expr string, params map[string]string, event apiGatewayEvent, fullPath string, bodyJSON interface{}) string {
	switch {
	case expr == "body":
		return event.Body
	case expr == "path":
		return fullPath
	case strings.HasPrefix(expr, "path."):
		return params[strings.TrimPrefix(expr, "path.")]
	case strings.HasPrefix(expr, "query."):
		if v, ok := event.Query[strings.TrimPrefix(expr, "query.")]; ok {
			return v
		}
		return ""
	case strings.HasPrefix(expr, "header."):
		if v, ok := lookupHeader(event.Headers, strings.TrimPrefix(expr, "header.")); ok {
			return v
		}
		return ""
	case strings.HasPrefix(expr, "body."):
		return lookupJSONPath(bodyJSON, strings.TrimPrefix(expr, "body."))
	default:
		return ""
	}
}

// lookupJSONPath walks a dotted path (a.b.c) into a decoded JSON value and
// returns the addressed field as a string. A missing field, a non-object on the
// way down, or a nil root all resolve to the empty string.
func lookupJSONPath(root interface{}, path string) string {
	cur := root
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur, ok = m[part]
		if !ok {
			return ""
		}
	}
	return scalarToString(cur)
}

// scalarToString renders a decoded JSON value as a string. Scalars render
// naturally (numbers without scientific notation); objects and arrays render as
// compact JSON.
func scalarToString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
