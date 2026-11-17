package extractor

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestScriptBecomesRulesAndHandlesBodies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.lua")
	src := `
match_patterns = { "example.*/*" }
block_urls = { "disable-devtool" }
click = { ".play img" }
intercepts = {
  { pattern = "source.php", handler = function(url, body)
      local data = json_decode(body)
      if data and data.file then return data.file end
      return nil
  end },
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if !matchesURLPattern(path, "https://www.example.fit/film/1/") || matchesURLPattern(path, "https://exampleplus.com/film/") {
		t.Fatal("match_patterns misapplied")
	}
	s, err := loadScript(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.L.Close()
	if !slices.Equal(s.rules.Block, []string{"disable-devtool"}) || !slices.Equal(s.rules.Click, []string{".play img"}) ||
		!slices.Equal(s.rules.Intercept, []string{"source.php"}) {
		t.Fatalf("rules = %+v", s.rules)
	}
	if got := s.onBody("https://player.test/source.php?id=1", `{"file":"/hls/master.m3u8"}`); got != "/hls/master.m3u8" {
		t.Fatalf("onBody = %q", got)
	}
	if got := s.onBody("https://player.test/source.php", "not json"); got != "" {
		t.Fatalf("onBody on bad JSON = %q", got)
	}
}
