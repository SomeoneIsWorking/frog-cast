// Package extractor finds a web page's HLS stream with pkg/webextract,
// adapted to the site by a Lua script from {configDir}/scripts/.
//
// A script sets these globals, each optional:
//
//	match_patterns = { "example.com/*" }   -- pages it applies to
//	block_urls = { "disable-devtool" }     -- URL substrings never loaded
//	click = { ".play-overlay" }            -- clicked in any frame once they appear
//	intercepts = { { pattern = "api/source", handler = function(url, body) return stream_url end } }
//
// An intercept's handler sees the response bodies (and frame HTML) whose URL
// contains its pattern and returns the stream URL, or nil. Handlers may call
// log(text) and json_decode(text).
package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"

	"wails-cast/pkg/folders"
	"wails-cast/pkg/webextract"
)

const (
	// scriptedTimeout bounds a page a script plays by itself.
	scriptedTimeout = 45 * time.Second
	// manualTimeout leaves time for someone to click play in the window.
	manualTimeout = 5 * time.Minute
)

// Extract finds pageURL's stream. With a matching script the page loads
// hidden; without one it opens in a window for someone to click play. trace,
// when set, sees what the page reports.
func Extract(pageURL string, trace func(webextract.Message)) (*webextract.Result, error) {
	opts := webextract.Options{Trace: trace}
	rules := webextract.Rules{}
	timeout := manualTimeout
	if path := findScript(pageURL); path != "" {
		fmt.Printf("Found Lua script: %s\n", path)
		s, err := loadScript(path)
		if err != nil {
			return nil, err
		}
		defer s.L.Close()
		rules = s.rules
		opts.OnBody = s.onBody
		timeout = scriptedTimeout
	} else {
		fmt.Println("No Lua script found, opening the page to play by hand")
		opts.Visible = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return webextract.Extract(ctx, pageURL, rules, opts)
}

// HasScript reports whether a Lua script matches the page, so extracting it
// needs no one at the window.
func HasScript(pageURL string) bool {
	return findScript(pageURL) != ""
}

type intercept struct {
	pattern string
	handler lua.LValue
}

// script is a loaded site script.
type script struct {
	L          *lua.LState
	rules      webextract.Rules
	intercepts []intercept
}

func loadScript(path string) (*script, error) {
	L := lua.NewState()
	registerFuncs(L)
	if err := L.DoFile(path); err != nil {
		L.Close()
		return nil, fmt.Errorf("load script: %w", err)
	}
	s := &script{L: L}
	s.rules.Block = stringList(L.GetGlobal("block_urls"))
	s.rules.Click = stringList(L.GetGlobal("click"))
	if tbl, ok := L.GetGlobal("intercepts").(*lua.LTable); ok {
		tbl.ForEach(func(_, v lua.LValue) {
			if rule, ok := v.(*lua.LTable); ok {
				i := intercept{lua.LVAsString(rule.RawGetString("pattern")), rule.RawGetString("handler")}
				s.intercepts = append(s.intercepts, i)
				s.rules.Intercept = append(s.rules.Intercept, i.pattern)
			}
		})
	}
	return s, nil
}

func stringList(v lua.LValue) []string {
	var out []string
	if tbl, ok := v.(*lua.LTable); ok {
		tbl.ForEach(func(_, item lua.LValue) { out = append(out, lua.LVAsString(item)) })
	}
	return out
}

// onBody runs the first intercept whose pattern the URL contains.
func (s *script) onBody(reqURL, body string) string {
	for _, rule := range s.intercepts {
		if !strings.Contains(reqURL, rule.pattern) {
			continue
		}
		s.L.Push(rule.handler)
		s.L.Push(lua.LString(reqURL))
		s.L.Push(lua.LString(body))
		if err := s.L.PCall(2, 1, nil); err != nil {
			fmt.Printf("Lua intercept %q: %v\n", rule.pattern, err)
			return ""
		}
		res := s.L.Get(-1)
		s.L.Pop(1)
		if res == lua.LNil {
			return ""
		}
		fmt.Printf("Lua intercept returned HLS URL: %s\n", lua.LVAsString(res))
		return lua.LVAsString(res)
	}
	return ""
}

// findScript scans all Lua scripts for match_patterns matching the URL.
func findScript(pageURL string) string {
	scriptsDir := filepath.Join(folders.GetConfig(), "scripts")
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		scriptPath := filepath.Join(scriptsDir, entry.Name())
		if matchesURLPattern(scriptPath, pageURL) {
			return scriptPath
		}
	}
	return ""
}

// matchesURLPattern checks if a script's match_patterns table contains
// a wildcard pattern matching the given URL.
// Patterns are matched against the full URL, then against host+path.
func matchesURLPattern(scriptPath, pageURL string) bool {
	L := lua.NewState()
	defer L.Close()
	registerFuncs(L)
	if err := L.DoFile(scriptPath); err != nil {
		return false
	}
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	hostPath := strings.TrimPrefix(parsed.Host+parsed.Path, "www.")
	for _, pattern := range stringList(L.GetGlobal("match_patterns")) {
		if pattern != "" && (wildcardMatch(pattern, pageURL) || wildcardMatch(pattern, hostPath)) {
			return true
		}
	}
	return false
}

// wildcardMatch checks if text matches a pattern containing * wildcards.
func wildcardMatch(pattern, text string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == text
	}
	if !strings.HasPrefix(text, parts[0]) {
		return false
	}
	text = text[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(text, parts[i])
		if idx < 0 {
			return false
		}
		text = text[idx+len(parts[i]):]
	}
	return strings.HasSuffix(text, parts[len(parts)-1])
}

func registerFuncs(L *lua.LState) {
	L.SetGlobal("log", L.NewFunction(func(L *lua.LState) int {
		fmt.Println("[LUA]", L.CheckString(1))
		return 0
	}))
	L.SetGlobal("json_decode", L.NewFunction(func(L *lua.LState) int {
		var val interface{}
		if err := json.Unmarshal([]byte(L.CheckString(1)), &val); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(toLuaValue(L, val))
		return 1
	}))
}

func toLuaValue(L *lua.LState, v interface{}) lua.LValue {
	switch vv := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(vv)
	case float64:
		return lua.LNumber(vv)
	case string:
		return lua.LString(vv)
	case []interface{}:
		tbl := L.NewTable()
		for i, item := range vv {
			tbl.RawSetInt(i+1, toLuaValue(L, item))
		}
		return tbl
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, val := range vv {
			tbl.RawSetString(k, toLuaValue(L, val))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", vv))
	}
}
