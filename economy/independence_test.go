package economy

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The economy stands on its own: nothing in it may import the game built
// on it, or speak of the game's nations and armies. A game rule that
// changes the economy goes through the levers (levers.go).
func TestEconomyKnowsNothingOfTheGame(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	words := regexp.MustCompile(`(?i)\b(nations?|nation's|armies|army|treasur(y|ies)|techs?|technolog\w*|schools?|schooling|universit(y|ies)|hospitals?|roads?|railways?|public services?|mercury/game)\b`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if m := words.FindString(line); m != "" {
				t.Errorf("%s:%d speaks of %q: %s", f, i+1, m, strings.TrimSpace(line))
			}
		}
	}
}
