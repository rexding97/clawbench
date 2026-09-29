package opencode

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"clawbench/internal/ai"
	"clawbench/internal/ai/backends"
	"clawbench/internal/model"
)

// OpenCodeInputRemaps maps OpenCode CLI input field names to canonical names.
// Injected into OpenCodeStreamParser at construction time.
var OpenCodeInputRemaps = map[string]string{
	"oldString":  "old_string",
	"newString":  "new_string",
	"replaceAll": "replace_all", // Edit replaceAll → replace_all
	"include":    "glob",        // Grep include → canonical glob
	"name":       "skill",       // Skill name → skill
}

// OpenCodeToolNameMap maps OpenCode CLI tool names to canonical names.
// Injected into OpenCodeStreamParser at construction time.
var OpenCodeToolNameMap = map[string]string{
	"read_file":  "Read",
	"write_file": "Write",
	"edit_file":  "Edit",
	"replace":    "Edit",
	"bash":       "Bash",
	"list_files": "LS",
	"grep":       "Grep",
	"glob":       "Glob",
	"web_fetch":  "WebFetch",
	"agent":      "Agent",
	"skill":      "Skill",
}

func init() {
	ai.RegisterBackend("opencode", newOpenCodeBackend)
	backends.Register(&backends.BackendPlugin{
		ID: "opencode",
		Spec: model.BackendSpec{
			ID: "opencode", Backend: "opencode", DefaultCmd: "opencode", Name: "OpenCode", Specialty: "终端编码工具",
			ThinkingEffortLevels: []string{"minimal", "high", "max"},
			AcpCommand:           "opencode acp",
			ACPLoadSession:       true,
			InstallCmd:           "npm install -g opencode-ai",
			SortOrder:            3,
		},
		ACP: &backends.ACPPlugin{
			InputRemaps: OpenCodeACPInputRemaps,
		},
	})
}

// newOpenCodeBackend returns a CLIBackend instance configured for OpenCode CLI.
func newOpenCodeBackend() ai.AIBackend {
	return &ai.CLIBackend{
		BackendName: "opencode",
		Cmd:         "opencode",
		BuildArgsFn: BuildOpenCodeStreamArgs,
		NewParserFn: func() ai.LineParser {
			return &ai.OpenCodeStreamParser{
				ToolNameMap: OpenCodeToolNameMap,
				InputRemaps: OpenCodeInputRemaps,
			}
		},
		FilterLineFn: OpenCodeFilterLine,
		PreStartFn:   nil,
	}
}

// OpenCodeFilterLine filters raw CLI output lines for OpenCode and its forks.
// Exported for reuse by MiMo-Code and any other OpenCode-fork backends.
func OpenCodeFilterLine(line string) (string, bool) {
	if line == "" || strings.HasPrefix(line, "[opencode-mobile]") {
		return "", false
	}
	if !strings.HasPrefix(line, "{") {
		return "", false
	}
	return line, true
}

// buildOpenCodeStreamArgsV1 builds args for OpenCode v1.x (npm opencode-ai):
// `run` accepts --dir, --dangerously-skip-permissions and --variant.
func buildOpenCodeStreamArgsV1(req ai.ChatRequest, prompt string) []string {
	args := []string{
		"run",
		prompt,
		"--format", "json",
		"--dangerously-skip-permissions",
	}

	// Pass OpenCode session ID for continuing conversations.
	// Only pass --session when resuming an existing OpenCode session
	// (indicated by Resume=true and a ses_ prefixed session ID).
	// On first message, SessionID contains ClawBench's UUID which OpenCode
	// doesn't recognize — let OpenCode create its own session.
	if req.SessionID != "" && req.Resume {
		args = append(args, "--session", req.SessionID)
	}

	// Working directory
	if req.WorkDir != "" {
		args = append(args, "--dir", req.WorkDir)
	}

	// Model override (format: provider/model, e.g., "minimax-cn-coding-plan/MiniMax-M2.7")
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	// Thinking effort level (e.g., --variant high)
	if req.ThinkingEffort != "" {
		args = append(args, "--variant", req.ThinkingEffort)
	}

	return args
}

// buildOpenCodeStreamArgsV2 builds args for OpenCode v2.x: `run` dropped
// --dir (the process working directory applies — CLIBackend already sets
// cmd.Dir), --dangerously-skip-permissions (replaced by --auto) and
// --variant (thinking effort now travels inside --model as model#variant).
// The prompt is placed after a "--" end-of-flags separator because citty
// treats any leading-dash argument as flags (e.g. a user message starting
// with "- 功能测试" would otherwise be parsed as clustered short flags).
func buildOpenCodeStreamArgsV2(req ai.ChatRequest, prompt string) []string {
	args := []string{
		"run",
		"--format", "json",
		"--auto",
	}

	if req.SessionID != "" && req.Resume {
		args = append(args, "--session", req.SessionID)
	}

	if req.Model != "" {
		model := req.Model
		// v2 variant ids are none/minimal/low/medium/high/xhigh — clawbench's
		// "max" maps to v2's top reasoning tier "xhigh".
		if req.ThinkingEffort != "" {
			variant := req.ThinkingEffort
			if variant == "max" {
				variant = "xhigh"
			}
			model += "#" + variant
		}
		args = append(args, "--model", model)
	}

	args = append(args, "--", prompt)
	return args
}

// opencodeMajorVersion caches the detected major version of the `opencode`
// binary so BuildOpenCodeStreamArgsV2 can pick the right flag set. Defaults
// to 1 when detection fails (e.g. opencode not on PATH).
var (
	opencodeMajorVersion     = 1
	opencodeMajorVersionOnce sync.Once
)

// parseOpenCodeMajorVersion extracts the major version from `opencode --version`
// output (e.g. "opencode v2.0.18" → 2). Returns 1 when unparseable so unknown
// binaries keep the v1 flag set.
func parseOpenCodeMajorVersion(output string) int {
	for _, field := range strings.Fields(output) {
		ver, ok := strings.CutPrefix(field, "v")
		if !ok {
			// Accept a bare "N.x.y" token as well, e.g. "opencode 2.0.18".
			if major, _, found := strings.Cut(field, "."); found {
				if n, err := strconv.Atoi(major); err == nil {
					return n
				}
			}
			continue
		}
		major, _, _ := strings.Cut(ver, ".")
		if n, err := strconv.Atoi(major); err == nil {
			return n
		}
		return 1
	}
	return 1
}

func detectOpenCodeMajorVersion() {
	opencodeMajorVersionOnce.Do(func() {
		out, err := exec.Command("opencode", "--version").Output()
		if err != nil {
			return
		}
		opencodeMajorVersion = parseOpenCodeMajorVersion(string(out))
	})
}

// buildStreamArgs picks the flag set for the given opencode major version.
func buildStreamArgs(req ai.ChatRequest, prompt string, majorVersion int) []string {
	if majorVersion >= 2 {
		return buildOpenCodeStreamArgsV2(req, prompt)
	}
	return buildOpenCodeStreamArgsV1(req, prompt)
}

// BuildOpenCodeStreamArgs constructs the CLI arguments for OpenCode streaming.
// Exported for reuse by MiMo-Code and any other OpenCode-fork backends.
// OpenCode v2 removed several `run` flags — dispatch on the detected binary
// version so both v1 and v2 installs keep working.
func BuildOpenCodeStreamArgs(req ai.ChatRequest) []string {
	// OpenCode CLI has no --system-prompt flag — inject into user prompt.
	prompt := ai.InjectSystemPrompt(req)

	detectOpenCodeMajorVersion()
	return buildStreamArgs(req, prompt, opencodeMajorVersion)
}
