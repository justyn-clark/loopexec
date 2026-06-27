package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// codeCharsPerToken is a code-calibrated estimate (code tokenizes denser than
// prose's ~4 chars/token). Tokens are rounded UP so the budget is a true
// ceiling, not an optimistic label (SPEC.md section 4).
const codeCharsPerToken = 3.3

// maxCandidates caps how many paths untrusted failure text can drive us to stat.
const maxCandidates = 256

// maxFailureBytes bounds the --failure / stdin read so a giant input cannot
// exhaust memory before we even start.
const maxFailureBytes = 4 << 20 // 4 MiB

func runesToTokens(n int) int { return int(math.Ceil(float64(n) / codeCharsPerToken)) }
func estimateTokens(s string) int {
	return runesToTokens(utf8.RuneCountInString(s))
}

// codePathRe matches file-ish paths ending in a known source extension.
// Extensions are ordered longest-first within a family (tsx before ts, jsx
// before js, cpp before c) and anchored with \b so multi-char extensions are
// not truncated. The relevance heuristic is deliberately dumb (stack-trace
// files + last diff): cheap, deterministic, explainable -- and it only ever
// surfaces the *symptom* file, not necessarily the root cause.
var codePathRe = regexp.MustCompile(`[A-Za-z0-9_./-]+\.(tsx|ts|jsx|mjs|cjs|js|go|py|rs|rb|java|kt|cpp|cc|hpp|c|h|sh|sql|yaml|yml|json|toml)\b`)

// diffBaseRe restricts --diff-base to safe git-ref characters so it cannot be a
// shell-injection sink when passed through `sh -c`.
var diffBaseRe = regexp.MustCompile(`^[A-Za-z0-9_./~^@{}-]+$`)

// relevantFile pairs the display path (workdir-relative) with the symlink-
// resolved absolute path that is safe to read.
type relevantFile struct {
	Rel  string
	Real string
}

// confine resolves an untrusted candidate under the workdir, following
// symlinks, and accepts it only if it stays inside the workdir AND is a regular
// file. This is the load-bearing security control: failure text is untrusted,
// so a `../`, an absolute path, or a symlink pointing outside the tree must not
// read a file off the host, and a device/FIFO must not hang the loop.
func confine(absWork, realWork, candidate string) (relevantFile, bool) {
	if filepath.IsAbs(candidate) {
		return relevantFile{}, false
	}
	full := filepath.Clean(filepath.Join(absWork, candidate))
	if r, err := filepath.Rel(absWork, full); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return relevantFile{}, false
	}
	real, err := filepath.EvalSymlinks(full) // requires existence; follows links
	if err != nil {
		return relevantFile{}, false
	}
	if r, rerr := filepath.Rel(realWork, real); rerr != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return relevantFile{}, false // resolved target escapes the workdir
	}
	info, err := os.Lstat(real)
	if err != nil || !info.Mode().IsRegular() {
		return relevantFile{}, false // reject dir / device / FIFO / socket
	}
	rel, err := filepath.Rel(absWork, full)
	if err != nil {
		return relevantFile{}, false
	}
	return relevantFile{Rel: rel, Real: real}, true
}

func workdirBases(workdir string) (absWork, realWork string, ok bool) {
	absWork, err := filepath.Abs(workdir)
	if err != nil {
		return "", "", false
	}
	realWork, err = filepath.EvalSymlinks(absWork)
	if err != nil {
		realWork = absWork
	}
	return absWork, realWork, true
}

// extractPaths pulls candidate paths out of untrusted failure text and keeps
// the ones that exist, are regular files, and stay inside workdir.
func extractPaths(workdir, failure string) []relevantFile {
	absWork, realWork, ok := workdirBases(workdir)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	var out []relevantFile
	for _, m := range codePathRe.FindAllString(failure, -1) {
		if len(out) >= maxCandidates {
			break
		}
		m = strings.TrimPrefix(m, "./")
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		if rf, valid := confine(absWork, realWork, m); valid {
			out = append(out, rf)
		}
	}
	return out
}

// gitDiffFiles returns last-diff and untracked files under workdir, each
// confined. Best-effort: a non-git workdir simply yields nothing.
func gitDiffFiles(workdir, base string) []relevantFile {
	if !diffBaseRe.MatchString(base) {
		return nil
	}
	absWork, realWork, ok := workdirBases(workdir)
	if !ok {
		return nil
	}
	var out []relevantFile
	collect := func(cmd string) {
		rc, o := runShell(workdir, cmd)
		if rc != 0 {
			return
		}
		for _, ln := range strings.Split(o, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			if rf, valid := confine(absWork, realWork, ln); valid {
				out = append(out, rf)
			}
		}
	}
	collect("git diff --name-only --relative " + base + " 2>/dev/null")
	collect("git ls-files --others --exclude-standard 2>/dev/null") // untracked (track_untracked)
	return out
}

func mergeRelevant(lists ...[]relevantFile) []relevantFile {
	seen := map[string]struct{}{}
	var out []relevantFile
	for _, list := range lists {
		for _, rf := range list {
			if _, ok := seen[rf.Rel]; ok {
				continue
			}
			seen[rf.Rel] = struct{}{}
			out = append(out, rf)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

// fenceFor returns a backtick fence longer than any backtick run in content, so
// untrusted file/failure content cannot break out of its code fence and inject
// agent-facing instructions.
func fenceFor(content string) string {
	max, cur := 0, 0
	for _, r := range content {
		if r == '`' {
			cur++
			if cur > max {
				max = cur
			}
		} else {
			cur = 0
		}
	}
	n := 3
	if max+1 > n {
		n = max + 1
	}
	return strings.Repeat("`", n)
}

func fencedBlock(lang, content string) string {
	fc := fenceFor(content)
	return fc + lang + "\n" + strings.TrimRight(content, "\n") + "\n" + fc + "\n\n"
}

func newBuildContextCmd() *cobra.Command {
	var failureArg, strategy, diffBase, workdir, out string
	var budgetTokens int

	cmd := &cobra.Command{
		Use:          "build-context",
		Short:        "Assemble a narrow, budgeted context slice (state + failure + relevant files)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			if diffBase == "" {
				diffBase = "HEAD"
			}
			if budgetTokens <= 0 {
				budgetTokens = 8000
			}
			if strategy != "" && strategy != "stacktrace_last_diff" {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: strategy %q is Planned (import_closure / dep_graph); using stacktrace_last_diff\n", strategy)
			}
			absWork, _, ok := workdirBases(workdir)
			if !ok {
				return failResponse(cmd, "", exitWorkspaceInvalid, "workspace_invalid", "cannot resolve --workdir")
			}

			failure, err := readFailure(cmd, failureArg, workdir)
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot read --failure", Cause: err}
			}

			// Mandatory header: machine state first, then the open failure.
			var hdr strings.Builder
			hdr.WriteString("## State\n\n")
			if st, serr := os.ReadFile(statePath(workdir)); serr == nil {
				hdr.WriteString(fencedBlock("json", string(st)))
			} else {
				hdr.WriteString("(no recorded run state)\n\n")
			}
			hdr.WriteString("## Current failure\n\n")
			hdr.WriteString(fencedBlock("", failure))

			const relevantHeader = "## Relevant files\n\n"
			const noFilesFooter = "(no relevant files found within budget)\n"

			// The budget is a TRUE ceiling: gate the worst-case minimal skeleton
			// (header + the relevant-files section header + the no-files footer).
			skeleton := hdr.String() + relevantHeader + noFilesFooter
			if estimateTokens(skeleton) > budgetTokens {
				return failResponse(cmd, "", exitLivenessDrift, "context_budget_unsatisfiable",
					fmt.Sprintf("the mandatory state+failure slice (~%d tokens) exceeds --budget-tokens %d",
						estimateTokens(skeleton), budgetTokens))
			}

			var b strings.Builder
			b.WriteString(hdr.String())
			b.WriteString(relevantHeader)
			curRunes := utf8.RuneCountInString(b.String())
			maxBytes := int64(float64(budgetTokens) * codeCharsPerToken)

			relevant := mergeRelevant(extractPaths(workdir, failure), gitDiffFiles(workdir, diffBase))
			included := []string{}
			dropped := []string{}
			for _, rf := range relevant {
				info, serr := os.Stat(rf.Real)
				if serr != nil || !info.Mode().IsRegular() {
					dropped = append(dropped, rf.Rel)
					continue
				}
				if info.Size() > maxBytes { // cannot fit -> drop UNREAD (no memory blowup, no hang)
					dropped = append(dropped, rf.Rel)
					continue
				}
				f, oerr := os.Open(rf.Real)
				if oerr != nil {
					dropped = append(dropped, rf.Rel)
					continue
				}
				data, rerr := io.ReadAll(io.LimitReader(f, maxBytes+1))
				f.Close()
				if rerr != nil {
					dropped = append(dropped, rf.Rel)
					continue
				}
				section := "### " + rf.Rel + "\n\n" + fencedBlock("", string(data))
				secRunes := utf8.RuneCountInString(section)
				if runesToTokens(curRunes+secRunes) > budgetTokens {
					dropped = append(dropped, rf.Rel)
					continue
				}
				b.WriteString(section)
				curRunes += secRunes
				included = append(included, rf.Rel)
			}
			if len(included) == 0 {
				b.WriteString(noFilesFooter)
			}

			if out == "" {
				out = filepath.Join(workdir, ".loopexec", "context.md")
			}
			// Confine --out to workdir: an out path must not escape the tree.
			if absOut, oerr := filepath.Abs(out); oerr == nil {
				if r, rerr := filepath.Rel(absWork, absOut); rerr != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
					return failResponse(cmd, "", exitWorkspaceInvalid, "workspace_invalid",
						"--out must stay within --workdir")
				}
			}
			if mkerr := os.MkdirAll(filepath.Dir(out), 0o755); mkerr != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot create context dir", Cause: mkerr}
			}
			if werr := os.WriteFile(out, []byte(b.String()), 0o644); werr != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot write context", Cause: werr}
			}

			r := response{
				Tool: toolName, Version: toolVersion, Status: "ok", Errors: []string{},
				Context: &contextReport{
					Path:            out,
					TokensEstimated: estimateTokens(b.String()),
					BudgetTokens:    budgetTokens,
					FilesIncluded:   included,
					FilesDropped:    dropped,
				},
			}
			if perr := printResponse(cmd, r); perr != nil {
				return perr
			}
			if !jsonOutput {
				fmt.Fprintf(cmd.OutOrStdout(), "context: %s (~%d/%d tokens, %d files, %d dropped)\n",
					out, r.Context.TokensEstimated, budgetTokens, len(included), len(dropped))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&failureArg, "failure", "", "Failure / stack-trace source: a file path, '-' for stdin, or inline text")
	cmd.Flags().IntVar(&budgetTokens, "budget-tokens", 8000, "Context token ceiling (code-calibrated ~3.3 chars/token)")
	cmd.Flags().StringVar(&strategy, "strategy", "stacktrace_last_diff", "Relevance strategy (import_closure / dep_graph are Planned)")
	cmd.Flags().StringVar(&diffBase, "diff-base", "HEAD", "Git ref for the last-diff relevance signal")
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory to resolve files against (default: current directory)")
	cmd.Flags().StringVar(&out, "out", "", "Output path within --workdir (default: <workdir>/.loopexec/context.md)")
	return cmd
}

// readFailure resolves --failure as a regular file, stdin ('-'), inline text,
// or empty -- bounded so a giant input cannot exhaust memory.
func readFailure(cmd *cobra.Command, arg, workdir string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", nil
	}
	if arg == "-" {
		data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxFailureBytes))
		return string(data), err
	}
	p := arg
	if !filepath.IsAbs(p) {
		p = filepath.Join(workdir, arg)
	}
	if info, serr := os.Stat(p); serr == nil && info.Mode().IsRegular() {
		f, oerr := os.Open(p)
		if oerr != nil {
			return arg, nil
		}
		defer f.Close()
		data, rerr := io.ReadAll(io.LimitReader(f, maxFailureBytes))
		if rerr != nil {
			return arg, nil
		}
		return string(data), nil
	}
	// Not a readable regular file: treat the argument as inline failure text.
	return arg, nil
}
