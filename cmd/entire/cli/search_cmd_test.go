package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/codesearch"
	"github.com/entireio/cli/cmd/entire/cli/search"
	"github.com/entireio/cli/internal/coreapi"
)

// TestSearchCmd_AccessibleModeRequiresQuery verifies that accessible mode
// is treated like --json: a query is required when ACCESSIBLE=1.
// Note: this test modifies process-global state (env var), so it must NOT
// use t.Parallel().
func TestSearchCmd_AccessibleModeRequiresQuery(t *testing.T) {
	t.Setenv("ACCESSIBLE", "1")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when no query with --json + ACCESSIBLE=1")
	}

	want := "query required when using --json, accessible mode, or piped output"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want containing %q", err.Error(), want)
	}
}

func TestSearchCmd_HelpMentionsRepoFlagAndInlineFilters(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"search", "-h"})

	if err := root.Execute(); err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	help := buf.String()
	if !strings.Contains(help, "--repo") {
		t.Fatalf("help missing --repo flag:\n%s", help)
	}
	if !strings.Contains(help, "inline filters") {
		t.Fatalf("help missing inline filter note:\n%s", help)
	}
	if !strings.Contains(help, "repo:*") {
		t.Fatalf("help missing repo:* inline example:\n%s", help)
	}
}

func TestWriteSearchJSON_ZeroLimitFallsBackToDefaultPageSize(t *testing.T) {
	t.Parallel()

	resp := &search.Response{
		Results: testResults(),
		Total:   2,
		Page:    1,
	}

	var buf bytes.Buffer
	if err := writeSearchJSON(&buf, resp, 0, 1); err != nil {
		t.Fatalf("writeSearchJSON returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"limit": 10`) {
		t.Fatalf("output missing default limit fallback:\n%s", output)
	}
	if !strings.Contains(output, `"total_pages": 1`) {
		t.Fatalf("output missing total_pages:\n%s", output)
	}
}

func TestCodeSearchEnabled_EnvGate(t *testing.T) {
	// Modifies process-global env, no t.Parallel().
	for _, tc := range []struct {
		val  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"true", false},
		{"1", true},
	} {
		t.Setenv("ENTIRE_CODE_SEARCH", tc.val)
		if got := codeSearchEnabled(); got != tc.want {
			t.Errorf("ENTIRE_CODE_SEARCH=%q: codeSearchEnabled() = %v, want %v", tc.val, got, tc.want)
		}
	}
}

func TestSearchCmd_CodeFlagGated(t *testing.T) {
	// --code without ENTIRE_CODE_SEARCH should fail with gate message.
	t.Setenv("ENTIRE_CODE_SEARCH", "")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--code", "test query"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --code used without ENTIRE_CODE_SEARCH")
	}
	if !strings.Contains(err.Error(), "not yet available") {
		t.Errorf("error = %q, want containing 'not yet available'", err.Error())
	}
	if strings.Contains(err.Error(), "ENTIRE_CODE_SEARCH") {
		t.Errorf("gate error should not mention env var, got: %q", err.Error())
	}
}

func TestSearchCmd_CodeFlagRequiresQuery(t *testing.T) {
	t.Setenv("ENTIRE_CODE_SEARCH", "1")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--code"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --code used without query")
	}
	if !strings.Contains(err.Error(), "query required for code search") {
		t.Errorf("error = %q, want containing 'query required'", err.Error())
	}
}

func TestSearchCmd_CaseSensitiveWithoutCode(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"search", "--case-sensitive", "--json", "test"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --case-sensitive used without --code")
	}
	if !strings.Contains(err.Error(), "--case-sensitive can only be used with --code") {
		t.Errorf("error = %q, want containing '--case-sensitive can only be used with --code'", err.Error())
	}
}

func TestWriteCodeSearchText(t *testing.T) {
	t.Parallel()

	resp := &codesearch.SearchResponse{
		Stats: codesearch.Stats{TotalMatches: 2, TotalFiles: 1, ReposSearched: 1, DurationMs: 15},
		Results: []codesearch.Result{
			{Repo: "entireio/cli", Path: "main.go", Line: 10, ContextLine: "func main() {"},
			{Repo: "entireio/cli", Path: "main.go", Line: 42, ContextLine: "\tfmt.Println(\"hello\")"},
		},
	}

	var buf bytes.Buffer
	writeCodeSearchText(&buf, resp)

	output := buf.String()
	if !strings.Contains(output, "entireio/cli:main.go:10: func main() {") {
		t.Errorf("output missing first result:\n%s", output)
	}
	if !strings.Contains(output, "2 matches across 1 files") {
		t.Errorf("output missing summary line:\n%s", output)
	}
}

func TestWriteCodeSearchJSON(t *testing.T) {
	t.Parallel()

	resp := &codesearch.SearchResponse{
		Query:     "handleRequest",
		Stats:     codesearch.Stats{TotalMatches: 1, TotalFiles: 1, ReposSearched: 1, DurationMs: 5},
		RepoStats: []codesearch.RepoStats{{Repo: "r", MatchCount: 1, FileCount: 1}},
		Results:   []codesearch.Result{{Repo: "r", Path: "f.go", Line: 1, ContextLine: "package main"}},
	}

	var buf bytes.Buffer
	if err := writeCodeSearchJSON(&buf, resp); err != nil {
		t.Fatalf("writeCodeSearchJSON error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"query": "handleRequest"`) {
		t.Errorf("output missing query echo:\n%s", output)
	}
	if !strings.Contains(output, `"total": 1`) {
		t.Errorf("output missing total:\n%s", output)
	}
	if !strings.Contains(output, `"path": "f.go"`) {
		t.Errorf("output missing result path:\n%s", output)
	}
	if !strings.Contains(output, `"repo_stats"`) {
		t.Errorf("output missing repo_stats:\n%s", output)
	}
}

func TestWriteCodeSearchText_TruncatesLongLines(t *testing.T) {
	t.Parallel()

	longLine := strings.Repeat("x", 300)
	resp := &codesearch.SearchResponse{
		Stats:   codesearch.Stats{TotalMatches: 1, TotalFiles: 1, ReposSearched: 1, DurationMs: 1},
		Results: []codesearch.Result{{Repo: "r", Path: "f.go", Line: 1, ContextLine: longLine}},
	}

	var buf bytes.Buffer
	writeCodeSearchText(&buf, resp)

	output := buf.String()
	if strings.Contains(output, longLine) {
		t.Error("expected long context_line to be truncated")
	}
	if !strings.Contains(output, "…") {
		t.Error("expected truncated line to end with ellipsis")
	}
	// The prefix + 200 chars + ellipsis should be present.
	truncated := strings.Repeat("x", maxContextLineLen)
	if !strings.Contains(output, truncated+"…") {
		t.Error("expected exactly maxContextLineLen characters before ellipsis")
	}
}

func TestWriteCodeSearchText_Empty(t *testing.T) {
	t.Parallel()

	resp := &codesearch.SearchResponse{
		Stats: codesearch.Stats{},
	}

	var buf bytes.Buffer
	writeCodeSearchText(&buf, resp)

	if !strings.Contains(buf.String(), "No code search results found") {
		t.Errorf("expected empty results message, got:\n%s", buf.String())
	}
}

func TestGroupReposByJurisdiction(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{Cell: "aws-us-east-2", Jurisdiction: "us", FullName: "acme/web"},
		{Cell: "aws-us-east-2", Jurisdiction: "us", FullName: "acme/api"},
		{Cell: "aws-eu-west-1", Jurisdiction: "eu", FullName: "acme/docs"},
		{Cell: "", Jurisdiction: "", FullName: "acme/empty"},
	}

	groups := groupReposByJurisdiction(repos)

	if len(groups) != 3 {
		t.Fatalf("groupReposByJurisdiction returned %d groups, want 3", len(groups))
	}

	jurisdictions := make(map[string]bool)
	for _, g := range groups {
		jurisdictions[g.jurisdiction] = true
	}

	if !jurisdictions["us"] {
		t.Error("missing jurisdiction 'us'")
	}
	if !jurisdictions["eu"] {
		t.Error("missing jurisdiction 'eu'")
	}
	if !jurisdictions[""] {
		t.Error("missing home jurisdiction (empty string) for repos without placement")
	}
}

func TestGroupReposByJurisdiction_DeduplicatesSameJurisdiction(t *testing.T) {
	t.Parallel()

	// Two different cells in the same jurisdiction should produce one group.
	repos := []coreapi.RepoIndexEntry{
		{Cell: "aws-us-east-1", Jurisdiction: "us", FullName: "acme/web"},
		{Cell: "aws-us-east-2", Jurisdiction: "us", FullName: "acme/api"},
	}

	groups := groupReposByJurisdiction(repos)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group for same jurisdiction, got %d", len(groups))
	}
	if groups[0].jurisdiction != "us" {
		t.Errorf("jurisdiction = %q, want us", groups[0].jurisdiction)
	}
}

func TestGroupReposByJurisdiction_Empty(t *testing.T) {
	t.Parallel()

	groups := groupReposByJurisdiction(nil)
	if len(groups) != 0 {
		t.Fatalf("groupReposByJurisdiction(nil) returned %d groups, want 0", len(groups))
	}
}

func TestMergeSearchResults(t *testing.T) {
	t.Parallel()

	cells := []cellGroup{
		{jurisdiction: "us"},
		{jurisdiction: "eu"},
	}

	results := []codeSearchCellResult{
		{
			resp: &codesearch.SearchResponse{
				Query: "handleRequest",
				Stats: codesearch.Stats{TotalMatches: 3, TotalFiles: 2, ReposSearched: 1, DurationMs: 10},
				Results: []codesearch.Result{
					{Repo: "acme/web", Path: "main.go", Line: 1, Score: 0.5},
				},
				RepoStats: []codesearch.RepoStats{{Repo: "acme/web", MatchCount: 3}},
			},
		},
		{
			resp: &codesearch.SearchResponse{
				Query: "handleRequest",
				Stats: codesearch.Stats{TotalMatches: 1, TotalFiles: 1, ReposSearched: 1, DurationMs: 20},
				Results: []codesearch.Result{
					{Repo: "acme/docs", Path: "handler.go", Line: 5, Score: 0.9},
				},
				RepoStats: []codesearch.RepoStats{{Repo: "acme/docs", MatchCount: 1}},
			},
		},
	}

	merged, err := mergeSearchResults(context.Background(), 0, cells, results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stats are the sum of per-cell peregrine stats (not recomputed from results).
	if merged.Stats.TotalMatches != 4 {
		t.Errorf("TotalMatches = %d, want 4 (summed from cells)", merged.Stats.TotalMatches)
	}
	if merged.Stats.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3 (summed from cells)", merged.Stats.TotalFiles)
	}
	if merged.Stats.ReposSearched != 2 {
		t.Errorf("ReposSearched = %d, want 2", merged.Stats.ReposSearched)
	}
	if merged.Stats.DurationMs != 20 {
		t.Errorf("DurationMs = %v, want 20 (slowest cell)", merged.Stats.DurationMs)
	}
	if len(merged.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(merged.Results))
	}
	// Higher score should come first (sorted descending).
	if merged.Results[0].Repo != "acme/docs" {
		t.Errorf("Results[0].Repo = %q, want acme/docs (higher score)", merged.Results[0].Repo)
	}
	if len(merged.RepoStats) != 2 {
		t.Fatalf("len(RepoStats) = %d, want 2", len(merged.RepoStats))
	}
}

func TestMergeSearchResults_Truncation(t *testing.T) {
	t.Parallel()

	cells := []cellGroup{
		{jurisdiction: "us"},
		{jurisdiction: "eu"},
	}

	results := []codeSearchCellResult{
		{resp: &codesearch.SearchResponse{
			Results: []codesearch.Result{
				{Repo: "a", Path: "1.go", Score: 0.9},
				{Repo: "a", Path: "2.go", Score: 0.7},
			},
			Stats: codesearch.Stats{TotalMatches: 2},
		}},
		{resp: &codesearch.SearchResponse{
			Results: []codesearch.Result{
				{Repo: "b", Path: "3.go", Score: 0.8},
				{Repo: "b", Path: "4.go", Score: 0.6},
			},
			Stats: codesearch.Stats{TotalMatches: 2},
		}},
	}

	merged, err := mergeSearchResults(context.Background(), 3, cells, results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(merged.Results) != 3 {
		t.Fatalf("len(Results) = %d, want 3 (truncated to limit)", len(merged.Results))
	}
	// Top 3 by score: 0.9, 0.8, 0.7
	if merged.Results[0].Score != 0.9 || merged.Results[1].Score != 0.8 || merged.Results[2].Score != 0.7 {
		t.Errorf("results not sorted by score: %v, %v, %v",
			merged.Results[0].Score, merged.Results[1].Score, merged.Results[2].Score)
	}
}

func TestMergeSearchResults_PartialCellError(t *testing.T) {
	t.Parallel()

	cells := []cellGroup{
		{jurisdiction: "us"},
		{jurisdiction: "eu"},
	}

	results := []codeSearchCellResult{
		{
			resp: &codesearch.SearchResponse{
				Query:   "test",
				Stats:   codesearch.Stats{TotalMatches: 2, TotalFiles: 1, ReposSearched: 1, DurationMs: 5},
				Results: []codesearch.Result{{Repo: "acme/web", Path: "f.go", Line: 1}},
			},
		},
		{
			err: errors.New("cell timed out"),
		},
	}

	merged, err := mergeSearchResults(context.Background(), 0, cells, results)
	if err != nil {
		t.Fatalf("partial failure should not error: %v", err)
	}

	// Stats are summed from successful cells only.
	if merged.Stats.TotalMatches != 2 {
		t.Errorf("TotalMatches = %d, want 2 (from successful cell)", merged.Stats.TotalMatches)
	}
	if len(merged.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1 (failed cell skipped)", len(merged.Results))
	}
	if len(merged.FailedJurisdictions) != 1 || merged.FailedJurisdictions[0] != "eu" {
		t.Errorf("FailedJurisdictions = %v, want [eu]", merged.FailedJurisdictions)
	}
}

func TestMergeSearchResults_DeduplicatesOverlappingCells(t *testing.T) {
	t.Parallel()

	cells := []cellGroup{
		{jurisdiction: ""},
		{jurisdiction: "us"},
	}

	// Same result returned by both home cell and explicit "us" cell.
	dup := codesearch.Result{Repo: "acme/web", Path: "main.go", Line: 10, Column: 5, Score: 0.9}
	results := []codeSearchCellResult{
		{resp: &codesearch.SearchResponse{
			Results: []codesearch.Result{dup},
			Stats:   codesearch.Stats{TotalMatches: 1, TotalFiles: 1, ReposSearched: 1},
		}},
		{resp: &codesearch.SearchResponse{
			Results: []codesearch.Result{dup},
			Stats:   codesearch.Stats{TotalMatches: 1, TotalFiles: 1, ReposSearched: 1},
		}},
	}

	merged, err := mergeSearchResults(context.Background(), 0, cells, results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(merged.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1 (duplicate removed)", len(merged.Results))
	}
}

func TestResolveRepoFilters_GhPrefix(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{ID: "01ABC", FullName: "entirehq/entire.io"},
	}
	ids, matched := resolveRepoFilters([]string{"gh/entirehq/entire.io"}, repos)
	if len(ids) != 1 || ids[0] != "01ABC" {
		t.Fatalf("gh/ prefix: ids = %v, want [01ABC]", ids)
	}
	if len(matched) != 1 {
		t.Fatalf("gh/ prefix: matched = %d, want 1", len(matched))
	}
}

func TestResolveRepoFilters_EtPrefix(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{ID: "02DEF", FullName: "myproj/backend"},
	}
	ids, matched := resolveRepoFilters([]string{"et/myproj/backend"}, repos)
	if len(ids) != 1 || ids[0] != "02DEF" {
		t.Fatalf("et/ prefix: ids = %v, want [02DEF]", ids)
	}
	if len(matched) != 1 {
		t.Fatalf("et/ prefix: matched = %d, want 1", len(matched))
	}
}

func TestResolveRepoFilters_ULID(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{ID: "01JXYZ123ABC", FullName: "entirehq/cli"},
	}
	ids, _ := resolveRepoFilters([]string{"01JXYZ123ABC"}, repos)
	if len(ids) != 1 || ids[0] != "01JXYZ123ABC" {
		t.Fatalf("ULID: ids = %v, want [01JXYZ123ABC]", ids)
	}
}

func TestResolveRepoFilters_BareSlug(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{ID: "01ABC", FullName: "entirehq/entire.io"},
	}
	ids, _ := resolveRepoFilters([]string{"entirehq/entire.io"}, repos)
	if len(ids) != 1 || ids[0] != "01ABC" {
		t.Fatalf("bare slug: ids = %v, want [01ABC]", ids)
	}
}

func TestResolveRepoFilters_NoMatch(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{ID: "01ABC", FullName: "entirehq/entire.io"},
	}
	ids, matched := resolveRepoFilters([]string{"gh/nonexistent/repo"}, repos)
	if len(ids) != 0 {
		t.Fatalf("no match: ids = %v, want empty", ids)
	}
	if len(matched) != 0 {
		t.Fatalf("no match: matched = %d, want 0", len(matched))
	}
}

func TestResolveRepoFilters_DeduplicatesSameRepo(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{ID: "01ABC", FullName: "entirehq/entire.io"},
	}
	// Same repo via three different formats — should produce one result.
	ids, _ := resolveRepoFilters([]string{"gh/entirehq/entire.io", "entirehq/entire.io", "01ABC"}, repos)
	if len(ids) != 1 {
		t.Fatalf("dedup: len(ids) = %d, want 1", len(ids))
	}
}

func TestResolveRepoFilters_MultipleReposMixed(t *testing.T) {
	t.Parallel()

	repos := []coreapi.RepoIndexEntry{
		{ID: "01ABC", FullName: "entirehq/entire.io"},
		{ID: "02DEF", FullName: "myproj/backend"},
	}
	ids, matched := resolveRepoFilters([]string{"gh/entirehq/entire.io", "et/myproj/backend"}, repos)
	if len(ids) != 2 {
		t.Fatalf("multiple: len(ids) = %d, want 2", len(ids))
	}
	if len(matched) != 2 {
		t.Fatalf("multiple: len(matched) = %d, want 2", len(matched))
	}
}

func TestSearchCmd_CaseSensitiveWithCodeFlagParsesCorrectly(t *testing.T) {
	// --case-sensitive with --code should be accepted (fails later at auth, not at validation).
	t.Setenv("ENTIRE_CODE_SEARCH", "1")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--code", "--case-sensitive", "HandleRequest"})

	err := root.Execute()
	// Will fail at auth, but should NOT fail at flag validation.
	if err != nil && strings.Contains(err.Error(), "--case-sensitive can only be used with --code") {
		t.Errorf("--case-sensitive with --code should be accepted, got: %v", err)
	}
}

func TestSearchCmd_LimitFlagAccepted(t *testing.T) {
	// --limit with --code should parse correctly.
	t.Setenv("ENTIRE_CODE_SEARCH", "1")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--code", "--limit", "50", "handleRequest"})

	err := root.Execute()
	// Will fail at auth, but should NOT fail at flag parsing.
	if err != nil && strings.Contains(err.Error(), "invalid") {
		t.Errorf("--limit 50 should be accepted, got: %v", err)
	}
}

func TestSearchCmd_InlineRepoStarTreatedAsAllRepos(t *testing.T) {
	// repo:* inline should be treated as "all repos" (no filter).
	t.Setenv("ENTIRE_CODE_SEARCH", "1")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--code", "auth repo:*"})

	err := root.Execute()
	// Will fail at auth, but should NOT fail at query parsing.
	if err != nil && strings.Contains(err.Error(), "invalid") {
		t.Errorf("repo:* should be accepted, got: %v", err)
	}
}

func TestSearchCmd_MultipleInlineRepoFilters(t *testing.T) {
	// Multiple inline repo: filters should all be collected.
	t.Setenv("ENTIRE_CODE_SEARCH", "1")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--code", "auth repo:gh/entirehq/entire.io repo:gh/entirehq/cli"})

	err := root.Execute()
	// Will fail at auth, but should NOT fail at filter parsing.
	if err != nil && strings.Contains(err.Error(), "invalid") {
		t.Errorf("multiple repo: filters should be accepted, got: %v", err)
	}
}

func TestWriteCodeSearchJSON_RepoFilteredEmpty(t *testing.T) {
	t.Parallel()

	// When a repo filter matches nothing, we get an empty response.
	resp := &codesearch.SearchResponse{
		Query:   "handleRequest",
		Stats:   codesearch.Stats{},
		Results: nil,
	}

	var buf bytes.Buffer
	if err := writeCodeSearchJSON(&buf, resp); err != nil {
		t.Fatalf("writeCodeSearchJSON error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"results": []`) {
		t.Errorf("expected empty results array, got:\n%s", output)
	}
	if !strings.Contains(output, `"total": 0`) {
		t.Errorf("expected total 0, got:\n%s", output)
	}
}

func TestExtractInlineRepoFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input     string
		wantQuery string
		wantRepos []string
	}{
		{"auth", "auth", nil},
		{"auth repo:gh/entirehq/cli", "auth", []string{"gh/entirehq/cli"}},
		{"repo:gh/a/b repo:et/c/d handleRequest", "handleRequest", []string{"gh/a/b", "et/c/d"}},
		{"repo:*", "", []string{"*"}},
		// author: and branch: are NOT consumed — they stay in the query.
		{"author:foo TODO", "author:foo TODO", nil},
		{"branch:main auth repo:gh/a/b", "branch:main auth", []string{"gh/a/b"}},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			gotQuery, gotRepos := extractInlineRepoFilters(tc.input)
			if gotQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tc.wantQuery)
			}
			if len(gotRepos) != len(tc.wantRepos) {
				t.Fatalf("repos = %v, want %v", gotRepos, tc.wantRepos)
			}
			for i := range gotRepos {
				if gotRepos[i] != tc.wantRepos[i] {
					t.Errorf("repos[%d] = %q, want %q", i, gotRepos[i], tc.wantRepos[i])
				}
			}
		})
	}
}

func TestSearchCmd_CodePreservesNonRepoFiltersInQuery(t *testing.T) {
	// Ensure author:foo is NOT consumed by code search query parsing.
	t.Setenv("ENTIRE_CODE_SEARCH", "1")

	root := NewRootCmd()
	root.SetArgs([]string{"search", "--code", "author:foo TODO"})

	err := root.Execute()
	// Will fail at auth/git, but should NOT fail with empty query.
	if err != nil && strings.Contains(err.Error(), "query required") {
		t.Errorf("author:foo should be preserved in code query, got: %v", err)
	}
}

func TestMergeSearchResults_AllCellsFail(t *testing.T) {
	t.Parallel()

	cells := []cellGroup{
		{jurisdiction: "us"},
		{jurisdiction: "eu"},
	}

	results := []codeSearchCellResult{
		{err: errors.New("us cell timed out")},
		{err: errors.New("eu cell timed out")},
	}

	_, err := mergeSearchResults(context.Background(), 0, cells, results)
	if err == nil {
		t.Fatal("expected error when all cells fail")
	}
	if !strings.Contains(err.Error(), "code search failed") {
		t.Errorf("error = %q, want containing 'code search failed'", err.Error())
	}
}
