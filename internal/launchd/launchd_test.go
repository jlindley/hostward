package launchd

import (
	"strings"
	"testing"
	"time"

	"hostward/internal/config"
)

func TestRenderIncludesSchedulerCommand(t *testing.T) {
	paths := config.Paths{Home: "/Users/jim"}
	agent := DefaultAgent(paths, "/usr/local/bin/hostward", 5*time.Second)

	rendered, err := Render(agent)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	for _, want := range []string{
		"<string>com.hostward.scheduler</string>",
		"<string>/usr/local/bin/hostward</string>",
		"<string>scheduler</string>",
		"<string>run</string>",
		"<string>--tick</string>",
		"<string>5s</string>",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered plist missing %s:\n%s", want, rendered)
		}
	}
}

func TestLooksEphemeralBinary(t *testing.T) {
	if !LooksEphemeralBinary("/var/folders/x/go-build1234/b001/exe/hostward") {
		t.Fatalf("expected go-build path to look ephemeral")
	}
	if LooksEphemeralBinary("/Users/jim/.local/bin/hostward") {
		t.Fatalf("expected stable path not to look ephemeral")
	}
}
