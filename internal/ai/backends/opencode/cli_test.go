package opencode

import (
	"testing"

	"clawbench/internal/ai"
)

func TestOpenCodePlugin_Registered(t *testing.T) {
	entry := ai.LookupBackendFactoryForTest("opencode")
	if entry == nil {
		t.Fatal("opencode backend factory not registered")
	}
}

func TestOpenCodePlugin_NewBackend(t *testing.T) {
	entry := ai.LookupBackendFactoryForTest("opencode")
	backend := entry.NewBackend()
	if backend == nil {
		t.Fatal("NewBackend returned nil")
	}
	if backend.Name() != "opencode" {
		t.Errorf("expected backend name 'opencode', got %q", backend.Name())
	}
}

func TestOpenCodePlugin_NewBackendIsCLIBackend(t *testing.T) {
	entry := ai.LookupBackendFactoryForTest("opencode")
	backend := entry.NewBackend()
	clib, ok := backend.(*ai.CLIBackend)
	if !ok {
		t.Fatal("expected *CLIBackend")
	}

	// Verify parser is an OpenCodeStreamParser
	parser := clib.NewParserFn()
	if _, ok := parser.(*ai.OpenCodeStreamParser); !ok {
		t.Errorf("expected *OpenCodeStreamParser, got %T", parser)
	}

	// Verify Cmd
	if clib.Cmd != "opencode" {
		t.Errorf("expected Cmd 'opencode', got %q", clib.Cmd)
	}

	// Verify PreStartFn is nil
	if clib.PreStartFn != nil {
		t.Error("opencode PreStartFn should be nil")
	}
}

func TestOpenCodePlugin_FilterLine(t *testing.T) {
	entry := ai.LookupBackendFactoryForTest("opencode")
	clib := entry.NewBackend().(*ai.CLIBackend)

	// JSON line should pass
	line, ok := clib.FilterLineFn(`{"type":"text"}`)
	if !ok {
		t.Error(`{"type":"text"} should pass filter`)
	}
	if line != `{"type":"text"}` {
		t.Errorf("expected line unchanged, got %q", line)
	}

	// Plain text should be rejected
	_, ok = clib.FilterLineFn("plain text")
	if ok {
		t.Error("plain text should be rejected")
	}

	// [opencode-mobile] prefix should be rejected
	_, ok = clib.FilterLineFn("[opencode-mobile] stuff")
	if ok {
		t.Error("[opencode-mobile] prefix should be rejected")
	}

	// Empty line should be rejected
	_, ok = clib.FilterLineFn("")
	if ok {
		t.Error("empty line should be rejected")
	}
}

func TestOpenCodePlugin_BuildArgs(t *testing.T) {
	req := ai.ChatRequest{
		Prompt:    "test prompt",
		SessionID: "opencode-sess-1",
		Resume:    true,
		WorkDir:   "/tmp/project",
		Model:     "opencode-model",
	}
	args := buildStreamArgs(req, "test prompt", 1)

	// Should start with "run <prompt> --format json --dangerously-skip-permissions"
	if len(args) < 5 {
		t.Fatalf("expected at least 5 args, got %d", len(args))
	}
	if args[0] != "run" {
		t.Errorf("expected first arg 'run', got %q", args[0])
	}
	if args[1] != "test prompt" {
		t.Errorf("expected second arg 'test prompt', got %q", args[1])
	}

	// Should have --format json
	hasFormatJSON := false
	for i, a := range args {
		if a == "--format" && i+1 < len(args) && args[i+1] == "json" {
			hasFormatJSON = true
		}
	}
	if !hasFormatJSON {
		t.Error("expected --format json in args")
	}

	// Should have --dangerously-skip-permissions
	hasDangerous := false
	for _, a := range args {
		if a == "--dangerously-skip-permissions" {
			hasDangerous = true
		}
	}
	if !hasDangerous {
		t.Error("expected --dangerously-skip-permissions in args")
	}

	// Should have --session for resume
	hasSession := false
	for i, a := range args {
		if a == "--session" && i+1 < len(args) && args[i+1] == "opencode-sess-1" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Error("expected --session opencode-sess-1 in args")
	}

	// Should have --dir
	hasDir := false
	for i, a := range args {
		if a == "--dir" && i+1 < len(args) && args[i+1] == "/tmp/project" {
			hasDir = true
		}
	}
	if !hasDir {
		t.Error("expected --dir /tmp/project in args")
	}

	// Should have --model
	hasModel := false
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "opencode-model" {
			hasModel = true
		}
	}
	if !hasModel {
		t.Error("expected --model opencode-model in args")
	}
}

func TestOpenCodePlugin_BuildArgs_Variant(t *testing.T) {
	req := ai.ChatRequest{
		Prompt:         "test",
		ThinkingEffort: "high",
	}
	args := buildStreamArgs(req, "test", 1)

	hasVariant := false
	for i, a := range args {
		if a == "--variant" && i+1 < len(args) && args[i+1] == "high" {
			hasVariant = true
		}
	}
	if !hasVariant {
		t.Error("expected --variant high in args when ThinkingEffort is set")
	}
}

func TestOpenCodePlugin_BuildArgs_NoSessionWithoutResume(t *testing.T) {
	req := ai.ChatRequest{
		Prompt:    "test",
		SessionID: "opencode-sess-1",
		Resume:    false,
	}
	args := buildStreamArgs(req, "test", 1)

	for _, a := range args {
		if a == "--session" {
			t.Error("--session should NOT be in args when Resume=false")
		}
	}
}

func TestOpenCodePlugin_BuildArgsV2(t *testing.T) {
	req := ai.ChatRequest{
		Prompt:         "test prompt",
		SessionID:      "opencode-sess-1",
		Resume:         true,
		WorkDir:        "/tmp/project",
		Model:          "volcengine-agent-plan/ark-code-latest",
		ThinkingEffort: "high",
	}
	args := buildStreamArgs(req, "test prompt", 2)

	// v2 `run` has no --dir (process cwd applies), no
	// --dangerously-skip-permissions (replaced by --auto) and no --variant
	// (travels inside --model as model#variant).
	for _, a := range args {
		if a == "--dir" {
			t.Error("--dir must NOT be in v2 args (flag removed in opencode v2)")
		}
		if a == "--dangerously-skip-permissions" {
			t.Error("--dangerously-skip-permissions must NOT be in v2 args (replaced by --auto)")
		}
		if a == "--variant" {
			t.Error("--variant must NOT be in v2 args (use model#variant)")
		}
	}
	if args[0] != "run" {
		t.Errorf("expected first arg 'run', got %q", args[0])
	}
	// Prompt must be the final arg, preceded by the "--" end-of-flags marker
	// so leading-dash messages are not parsed as flags by citty.
	if len(args) < 2 || args[len(args)-1] != "test prompt" {
		t.Errorf("expected prompt as last arg, got %v", args)
	}
	if args[len(args)-2] != "--" {
		t.Errorf("expected '--' before the prompt, got %v", args[len(args)-2:])
	}
	hasAuto := false
	for _, a := range args {
		if a == "--auto" {
			hasAuto = true
		}
	}
	if !hasAuto {
		t.Error("expected --auto in v2 args")
	}
	hasSession := false
	for i, a := range args {
		if a == "--session" && i+1 < len(args) && args[i+1] == "opencode-sess-1" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Error("expected --session opencode-sess-1 in v2 args for resume")
	}
	hasModel := false
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "volcengine-agent-plan/ark-code-latest#high" {
			hasModel = true
		}
	}
	if !hasModel {
		t.Error("expected --model volcengine-agent-plan/ark-code-latest#high in v2 args")
	}
}

func TestOpenCodePlugin_BuildArgsV2_MaxVariantMapsXhigh(t *testing.T) {
	req := ai.ChatRequest{
		Prompt:         "test",
		Model:          "provider/model",
		ThinkingEffort: "max",
	}
	args := buildStreamArgs(req, "test", 2)

	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "provider/model#xhigh" {
			return
		}
	}
	t.Error("expected --model provider/model#xhigh (max → xhigh) in v2 args")
}

func TestOpenCodePlugin_BuildArgsV2_LeadingDashPrompt(t *testing.T) {
	// A user message starting with "- " (e.g. "- 功能测试") must survive as a
	// single positional arg — citty otherwise parses each character as a
	// clustered short flag and the run fails with "Unrecognized flag".
	req := ai.ChatRequest{
		Prompt: "- 功能测试",
		Model:  "provider/model",
	}
	args := buildStreamArgs(req, "- 功能测试", 2)

	if args[len(args)-1] != "- 功能测试" {
		t.Errorf("expected leading-dash prompt as last arg, got %v", args)
	}
	if args[len(args)-2] != "--" {
		t.Errorf("expected '--' before the prompt, got %v", args[len(args)-2:])
	}
}

func TestParseOpenCodeMajorVersion(t *testing.T) {
	cases := map[string]int{
		"opencode v2.0.18\n": 2,
		"opencode v1.18.33":  1,
		"opencode 2.0.18":    2,
		"v2.0.10":            2,
		"garbage":            1,
		"":                   1,
	}
	for input, want := range cases {
		if got := parseOpenCodeMajorVersion(input); got != want {
			t.Errorf("parseOpenCodeMajorVersion(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestOpenCodePlugin_CmdName(t *testing.T) {
	entry := ai.LookupBackendFactoryForTest("opencode")
	clib := entry.NewBackend().(*ai.CLIBackend)
	if clib.Cmd != "opencode" {
		t.Errorf("expected Cmd 'opencode', got %q", clib.Cmd)
	}
}
