package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yusing/goutils/env"
)

type config struct {
	Addr string
}

type stringSliceFlag struct {
	set bool
	v   []string
}

func (s *stringSliceFlag) String() string {
	return strings.Join(s.v, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	s.set = true
	if value == "" {
		s.v = nil
		return nil
	}
	s.v = strings.Split(value, ",")
	return nil
}

func run(args []string) error {
	cfg, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		printHelp()
		return nil
	}
	if rest[0] == "help" {
		printHelp()
		return nil
	}
	ep, matchedLen := findEndpoint(rest)
	if ep == nil {
		ep, matchedLen = findEndpointAlias(rest)
	}
	if ep == nil {
		return unknownCommandError(rest)
	}
	cmdArgs := rest[matchedLen:]
	return executeEndpoint(cfg.Addr, *ep, cmdArgs)
}

func parseGlobal(args []string) (config, []string, error) {
	var cfg config
	fs := flag.NewFlagSet("godoxy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Addr, "addr", "", "API address, e.g. 127.0.0.1:8888 or http://127.0.0.1:8888")
	if err := fs.Parse(args); err != nil {
		return cfg, nil, err
	}
	return cfg, fs.Args(), nil
}

func resolveBaseURL(addrFlag string) (string, error) {
	if addrFlag != "" {
		return normalizeURL(addrFlag), nil
	}
	_, _, _, fullURL := env.GetAddrEnv("LOCAL_API_ADDR", "", "http")
	if fullURL == "" {
		return "", errors.New("missing LOCAL_API_ADDR (or GODOXY_LOCAL_API_ADDR). set env var or pass --addr")
	}
	return normalizeURL(fullURL), nil
}

func normalizeURL(addr string) string {
	a := strings.TrimSpace(addr)
	if strings.Contains(a, "://") {
		return strings.TrimRight(a, "/")
	}
	return "http://" + strings.TrimRight(a, "/")
}

func findEndpoint(args []string) (*Endpoint, int) {
	var best *Endpoint
	bestLen := -1
	for i := range generatedEndpoints {
		ep := &generatedEndpoints[i]
		if len(ep.CommandPath) > len(args) {
			continue
		}
		ok := true
		for j, tok := range ep.CommandPath {
			if args[j] != tok {
				ok = false
				break
			}
		}
		if ok && len(ep.CommandPath) > bestLen {
			best = ep
			bestLen = len(ep.CommandPath)
		}
	}
	return best, bestLen
}

func executeEndpoint(addrFlag string, ep Endpoint, args []string) error {
	fs := flag.NewFlagSet(strings.Join(ep.CommandPath, "-"), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	useWS := false
	if ep.IsWebSocket {
		fs.BoolVar(&useWS, "ws", false, "use websocket")
	}
	typedValues := make(map[string]any, len(ep.Params))
	isSet := make(map[string]bool, len(ep.Params))
	for _, p := range ep.Params {
		switch p.Type {
		case "integer":
			v := new(int)
			fs.IntVar(v, p.FlagName, 0, p.Description)
			typedValues[p.FlagName] = v
		case "number":
			v := new(float64)
			fs.Float64Var(v, p.FlagName, 0, p.Description)
			typedValues[p.FlagName] = v
		case "boolean":
			v := new(bool)
			fs.BoolVar(v, p.FlagName, false, p.Description)
			typedValues[p.FlagName] = v
		case "array":
			v := &stringSliceFlag{}
			fs.Var(v, p.FlagName, p.Description+" (comma-separated)")
			typedValues[p.FlagName] = v
		default:
			v := new(string)
			fs.StringVar(v, p.FlagName, "", p.Description)
			typedValues[p.FlagName] = v
		}
	}
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, formatEndpointHelp(ep))
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("unexpected args: %s\n\n%s", strings.Join(fs.Args(), " "), formatEndpointHelp(ep))
	}
	fs.Visit(func(f *flag.Flag) {
		isSet[f.Name] = true
	})

	for _, p := range ep.Params {
		if !p.Required {
			continue
		}
		if !isSet[p.FlagName] {
			return fmt.Errorf("missing required flag --%s\n\n%s", p.FlagName, formatEndpointHelp(ep))
		}
	}

	baseURL, err := resolveBaseURL(addrFlag)
	if err != nil {
		return err
	}
	reqURL, body, err := buildRequest(ep, baseURL, typedValues, isSet)
	if err != nil {
		return err
	}

	if useWS {
		if !ep.IsWebSocket {
			return errors.New("--ws is only supported for websocket endpoints")
		}
		return execWebsocket(ep, reqURL)
	}
	return execHTTP(ep, reqURL, body)
}

func buildRequest(ep Endpoint, baseURL string, typedValues map[string]any, isSet map[string]bool) (string, []byte, error) {
	path := ep.Path
	for _, p := range ep.Params {
		if p.In != "path" {
			continue
		}
		raw, err := paramValueString(p, typedValues[p.FlagName], isSet[p.FlagName])
		if err != nil {
			return "", nil, err
		}
		if raw == "" {
			continue
		}
		esc := url.PathEscape(raw)
		path = strings.ReplaceAll(path, "{"+p.Name+"}", esc)
		path = strings.ReplaceAll(path, ":"+p.Name, esc)
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid base url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path

	q := u.Query()
	for _, p := range ep.Params {
		if p.In != "query" || !isSet[p.FlagName] {
			continue
		}
		val, err := paramQueryValues(p, typedValues[p.FlagName])
		if err != nil {
			return "", nil, err
		}
		for _, v := range val {
			q.Add(p.Name, v)
		}
	}
	u.RawQuery = q.Encode()

	bodyMap := map[string]any{}
	rawBody := ""
	for _, p := range ep.Params {
		if p.In != "body" || !isSet[p.FlagName] {
			continue
		}
		if p.Name == "file" {
			s, err := paramValueString(p, typedValues[p.FlagName], true)
			if err != nil {
				return "", nil, err
			}
			rawBody = s
			continue
		}
		v, err := paramBodyValue(p, typedValues[p.FlagName])
		if err != nil {
			return "", nil, err
		}
		bodyMap[p.Name] = v
	}

	if rawBody != "" {
		return u.String(), []byte(rawBody), nil
	}
	if len(bodyMap) == 0 {
		return u.String(), nil, nil
	}
	data, err := json.Marshal(bodyMap)
	if err != nil {
		return "", nil, fmt.Errorf("marshal body: %w", err)
	}
	return u.String(), data, nil
}

func paramValueString(p Param, raw any, wasSet bool) (string, error) {
	if !wasSet {
		return "", nil
	}
	switch v := raw.(type) {
	case *string:
		return *v, nil
	case *int:
		return strconv.Itoa(*v), nil
	case *float64:
		return strconv.FormatFloat(*v, 'f', -1, 64), nil
	case *bool:
		if *v {
			return "true", nil
		}
		return "false", nil
	case *stringSliceFlag:
		return strings.Join(v.v, ","), nil
	default:
		return "", fmt.Errorf("unsupported flag value for %s", p.FlagName)
	}
}

func paramQueryValues(p Param, raw any) ([]string, error) {
	switch v := raw.(type) {
	case *string:
		return []string{*v}, nil
	case *int:
		return []string{strconv.Itoa(*v)}, nil
	case *float64:
		return []string{strconv.FormatFloat(*v, 'f', -1, 64)}, nil
	case *bool:
		if *v {
			return []string{"true"}, nil
		}
		return []string{"false"}, nil
	case *stringSliceFlag:
		if len(v.v) == 0 {
			return nil, nil
		}
		return v.v, nil
	default:
		return nil, fmt.Errorf("unsupported query flag type for %s", p.FlagName)
	}
}

func paramBodyValue(p Param, raw any) (any, error) {
	switch v := raw.(type) {
	case *string:
		if p.Type == "object" || p.Type == "array" {
			var decoded any
			if err := json.Unmarshal([]byte(*v), &decoded); err != nil {
				return nil, fmt.Errorf("invalid JSON for --%s: %w", p.FlagName, err)
			}
			return decoded, nil
		}
		return *v, nil
	case *int:
		return *v, nil
	case *float64:
		return *v, nil
	case *bool:
		return *v, nil
	case *stringSliceFlag:
		return v.v, nil
	default:
		return nil, fmt.Errorf("unsupported body flag type for %s", p.FlagName)
	}
}

func execHTTP(ep Endpoint, reqURL string, body []byte) error {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(ep.Method, reqURL, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(payload) == 0 {
			return fmt.Errorf("%s %s failed: %s", ep.Method, ep.Path, resp.Status)
		}
		return fmt.Errorf("%s %s failed: %s: %s", ep.Method, ep.Path, resp.Status, strings.TrimSpace(string(payload)))
	}

	printJSON(payload)
	return nil
}

func execWebsocket(ep Endpoint, reqURL string) error {
	wsURL := strings.Replace(reqURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	if strings.ToUpper(ep.Method) != http.MethodGet {
		return fmt.Errorf("--ws requires GET endpoint, got %s", ep.Method)
	}
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer c.Close()

	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if err := c.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || strings.Contains(err.Error(), "close") {
				return nil
			}
			return err
		}
		if string(msg) == "pong" {
			continue
		}
		fmt.Println(string(msg))
	}
}

func printJSON(payload []byte) {
	if len(payload) == 0 {
		fmt.Println("null")
		return
	}
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		fmt.Println(strings.TrimSpace(string(payload)))
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func printHelp() {
	fmt.Println("godoxy [--addr ADDR] <command>")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  godoxy version")
	fmt.Println("  godoxy route list")
	fmt.Println("  godoxy route route --which whoami")
	fmt.Println()
	printGroupedCommands()
}

func printGroupedCommands() {
	grouped := map[string][]Endpoint{}
	groupOrder := make([]string, 0)
	seen := map[string]bool{}
	for _, ep := range generatedEndpoints {
		group := "root"
		if len(ep.CommandPath) > 1 {
			group = ep.CommandPath[0]
		}
		grouped[group] = append(grouped[group], ep)
		if !seen[group] {
			seen[group] = true
			groupOrder = append(groupOrder, group)
		}
	}
	sort.Strings(groupOrder)
	for _, group := range groupOrder {
		fmt.Printf("Commands (%s):\n", group)
		sort.Slice(grouped[group], func(i, j int) bool {
			li := strings.Join(grouped[group][i].CommandPath, " ")
			lj := strings.Join(grouped[group][j].CommandPath, " ")
			return li < lj
		})
		maxCmdWidth := 0
		for _, ep := range grouped[group] {
			cmd := strings.Join(ep.CommandPath, " ")
			if len(cmd) > maxCmdWidth {
				maxCmdWidth = len(cmd)
			}
		}
		for _, ep := range grouped[group] {
			cmd := strings.Join(ep.CommandPath, " ")
			fmt.Printf("  %-*s  %s\n", maxCmdWidth, cmd, ep.Summary)
		}
		fmt.Println()
	}
}

func unknownCommandError(rest []string) error {
	cmd := strings.Join(rest, " ")
	var b strings.Builder
	b.WriteString("unknown command: ")
	b.WriteString(cmd)
	if len(rest) > 0 && hasGroup(rest[0]) {
		if len(rest) > 1 {
			if hint := nearestForGroup(rest[0], rest[1]); hint != "" {
				b.WriteString("\nDo you mean ")
				b.WriteString(hint)
				b.WriteString("?")
			}
		}
		b.WriteString("\n\n")
		b.WriteString(formatGroupHelp(rest[0]))
		return errors.New(b.String())
	}
	if hint := nearestCommand(cmd); hint != "" {
		b.WriteString("\nDo you mean ")
		b.WriteString(hint)
		b.WriteString("?")
	}
	b.WriteString("\n\n")
	b.WriteString("Run `godoxy help` for available commands.")
	return errors.New(b.String())
}

func findEndpointAlias(args []string) (*Endpoint, int) {
	var best *Endpoint
	bestLen := -1
	for i := range generatedEndpoints {
		alias := aliasCommandPath(generatedEndpoints[i])
		if len(alias) == 0 || len(alias) > len(args) {
			continue
		}
		ok := true
		for j, tok := range alias {
			if args[j] != tok {
				ok = false
				break
			}
		}
		if ok && len(alias) > bestLen {
			best = &generatedEndpoints[i]
			bestLen = len(alias)
		}
	}
	return best, bestLen
}

func aliasCommandPath(ep Endpoint) []string {
	rawPath := strings.TrimPrefix(ep.Path, "/api/v1/")
	rawPath = strings.Trim(rawPath, "/")
	if rawPath == "" {
		return nil
	}
	parts := strings.Split(rawPath, "/")
	if len(parts) == 1 {
		if isPathParam(parts[0]) {
			return nil
		}
		return []string{toKebabToken(parts[0])}
	}
	if isPathParam(parts[0]) || isPathParam(parts[1]) {
		return nil
	}
	return []string{toKebabToken(parts[0]), toKebabToken(parts[1])}
}

func isPathParam(s string) bool {
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, ":")
}

func toKebabToken(s string) string {
	s = strings.ReplaceAll(s, "_", "-")
	return strings.ToLower(strings.Trim(s, "-"))
}

func hasGroup(group string) bool {
	for _, ep := range generatedEndpoints {
		if len(ep.CommandPath) > 1 && ep.CommandPath[0] == group {
			return true
		}
	}
	return false
}

func nearestCommand(input string) string {
	commands := make([]string, 0, len(generatedEndpoints))
	for _, ep := range generatedEndpoints {
		commands = append(commands, strings.Join(ep.CommandPath, " "))
	}
	return nearestByDistance(input, commands)
}

func nearestForGroup(group, input string) string {
	choiceSet := map[string]struct{}{}
	for _, ep := range generatedEndpoints {
		if len(ep.CommandPath) < 2 || ep.CommandPath[0] != group {
			continue
		}
		choiceSet[ep.CommandPath[1]] = struct{}{}
		alias := aliasCommandPath(ep)
		if len(alias) == 2 && alias[0] == group {
			choiceSet[alias[1]] = struct{}{}
		}
	}
	choices := make([]string, 0, len(choiceSet))
	for choice := range choiceSet {
		choices = append(choices, choice)
	}
	if len(choices) == 0 {
		return ""
	}
	return group + " " + nearestByDistance(input, choices)
}

func formatGroupHelp(group string) string {
	commands := make([]Endpoint, 0)
	for _, ep := range generatedEndpoints {
		if len(ep.CommandPath) > 1 && ep.CommandPath[0] == group {
			commands = append(commands, ep)
		}
	}
	sort.Slice(commands, func(i, j int) bool {
		return strings.Join(commands[i].CommandPath, " ") < strings.Join(commands[j].CommandPath, " ")
	})
	maxWidth := 0
	for _, ep := range commands {
		cmd := strings.Join(ep.CommandPath, " ")
		if len(cmd) > maxWidth {
			maxWidth = len(cmd)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Available subcommands for %s:\n", group)
	for _, ep := range commands {
		cmd := strings.Join(ep.CommandPath, " ")
		fmt.Fprintf(&b, "  %-*s  %s\n", maxWidth, cmd, ep.Summary)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatEndpointHelp(ep Endpoint) string {
	cmd := "godoxy " + strings.Join(ep.CommandPath, " ")
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: %s [flags]\n", cmd)
	if ep.Summary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", ep.Summary)
	}
	if alias := aliasCommandPath(ep); len(alias) > 0 && strings.Join(alias, " ") != strings.Join(ep.CommandPath, " ") {
		fmt.Fprintf(&b, "Alias: godoxy %s\n", strings.Join(alias, " "))
	}
	params := make([]Param, 0, len(ep.Params))
	params = append(params, ep.Params...)
	if ep.IsWebSocket {
		params = append(params, Param{
			FlagName:    "ws",
			Type:        "boolean",
			Description: "use websocket",
			Required:    false,
		})
	}
	if len(params) == 0 {
		return strings.TrimRight(b.String(), "\n")
	}
	b.WriteString("Flags:\n")
	maxWidth := 0
	flagNames := make([]string, 0, len(params))
	for _, p := range params {
		name := "--" + p.FlagName
		if p.Required {
			name += " (required)"
		}
		flagNames = append(flagNames, name)
		if len(name) > maxWidth {
			maxWidth = len(name)
		}
	}
	for i, p := range params {
		desc := p.Description
		if desc == "" {
			desc = p.In + " " + p.Type
		}
		fmt.Fprintf(&b, "  %-*s  %s\n", maxWidth, flagNames[i], desc)
	}
	return strings.TrimRight(b.String(), "\n")
}

func nearestByDistance(input string, choices []string) string {
	if len(choices) == 0 {
		return ""
	}
	nearest := choices[0]
	minDistance := levenshteinDistance(input, nearest)
	for _, choice := range choices[1:] {
		d := levenshteinDistance(input, choice)
		if d < minDistance {
			minDistance = d
			nearest = choice
		}
	}
	return nearest
}

//nolint:intrange
func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	v0 := make([]int, len(b)+1)
	v1 := make([]int, len(b)+1)

	for i := 0; i <= len(b); i++ {
		v0[i] = i
	}

	for i := 0; i < len(a); i++ {
		v1[0] = i + 1

		for j := 0; j < len(b); j++ {
			cost := 0
			if a[i] != b[j] {
				cost = 1
			}

			v1[j+1] = min3(v1[j]+1, v0[j+1]+1, v0[j]+cost)
		}

		for j := 0; j <= len(b); j++ {
			v0[j] = v1[j]
		}
	}

	return v1[len(b)]
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < a && b < c {
		return b
	}
	return c
}
