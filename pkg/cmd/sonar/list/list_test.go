package list

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func newFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return nil, nil
		},
	}
}

func TestList_RejectsPositionalArgs(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"frobnicate"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for stray positional arg")
	}
	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"-o", "yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for -o yaml")
	}
	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList_RejectsPageZero(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"--page", "0"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --page 0")
	}
	if !strings.Contains(err.Error(), "--page must be >= 1") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList_RejectsPageSizeZero(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"--page-size", "0"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --page-size 0")
	}
	if !strings.Contains(err.Error(), "--page-size must be between 1 and 500") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList_RejectsPageSizeOverMax(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"--page-size", "1000"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--page-size must be between 1 and 500") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList_PageFooter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		paging portal.SonarPaging
		want   string
	}{
		{
			name:   "within range",
			paging: portal.SonarPaging{PageIndex: 1, PageSize: 50, Total: 27},
			want:   "27 projects, page 1 of 1 (page-size 50)",
		},
		{
			name:   "singular projects word",
			paging: portal.SonarPaging{PageIndex: 1, PageSize: 50, Total: 1},
			want:   "1 project, page 1 of 1 (page-size 50)",
		},
		{
			name:   "empty result",
			paging: portal.SonarPaging{PageIndex: 999, PageSize: 50, Total: 0},
			want:   "no projects (page-size 50)",
		},
		{
			name:   "beyond last page",
			paging: portal.SonarPaging{PageIndex: 200, PageSize: 50, Total: 27},
			want:   "27 projects — page 200 is beyond last page (1 of 1) (page-size 50)",
		},
		{
			name:   "multi-page within range",
			paging: portal.SonarPaging{PageIndex: 3, PageSize: 10, Total: 35},
			want:   "35 projects, page 3 of 4 (page-size 10)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pageFooter(tc.paging)
			if got != tc.want {
				t.Errorf("pageFooter(%+v) = %q, want %q", tc.paging, got, tc.want)
			}
		})
	}
}

func TestList_RenderTable_EmptyProjects(t *testing.T) {
	t.Parallel()

	result := &portal.SonarProjectList{
		Projects: nil,
		Paging:   portal.SonarPaging{PageIndex: 1, PageSize: 50, Total: 0},
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, result); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "no projects") {
		t.Errorf("expected 'no projects' footer for empty list, got:\n%s", out)
	}
}

func TestList_RenderTable_WithProjects(t *testing.T) {
	t.Parallel()

	result := &portal.SonarProjectList{
		Projects: []portal.SonarProject{
			{Key: "payments-api", Name: "Payments API", QualityGateStatus: portal.QualityGateOK},
			{Key: "auth-service", Name: "Auth Service", QualityGateStatus: portal.QualityGateError},
			{Key: "no-gate-proj", Name: "No Gate Project", QualityGateStatus: ""},
		},
		Paging: portal.SonarPaging{PageIndex: 1, PageSize: 50, Total: 3},
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, result); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()

	wantSubstrings := []string{
		"payments-api", "Payments API", "OK",
		"auth-service", "Auth Service", "ERROR",
		"no-gate-proj", "No Gate Project", string(portal.QualityGateNone),
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, not found in:\n%s", want, out)
		}
	}

	if !strings.Contains(out, "3 projects") {
		t.Errorf("expected '3 projects' in footer, got:\n%s", out)
	}
}

func TestList_RenderTable_HeaderPresent(t *testing.T) {
	t.Parallel()

	result := &portal.SonarProjectList{
		Projects: []portal.SonarProject{
			{Key: "my-proj", Name: "My Project", QualityGateStatus: portal.QualityGateOK},
		},
		Paging: portal.SonarPaging{PageIndex: 1, PageSize: 50, Total: 1},
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, result); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()
	for _, header := range []string{"KEY", "NAME", "GATE"} {
		if !strings.Contains(out, header) {
			t.Errorf("expected header %q in output, got:\n%s", header, out)
		}
	}
}

func TestList_ValidInvocationReachesRunF(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdList(newFactory(), func(opts *ListOptions) error {
		called = true

		if opts.Page != 2 {
			t.Errorf("Page = %d, want 2", opts.Page)
		}

		if opts.PageSize != 25 {
			t.Errorf("PageSize = %d, want 25", opts.PageSize)
		}

		if opts.Search != "payments" {
			t.Errorf("Search = %q, want %q", opts.Search, "payments")
		}

		if opts.OutputFormat != "json" {
			t.Errorf("OutputFormat = %q, want %q", opts.OutputFormat, "json")
		}

		return nil
	})

	cmd.SetArgs([]string{"--page", "2", "--page-size", "25", "--search", "payments", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not called")
	}
}

func TestList_PageFooter_TotalZeroAnyPageSize(t *testing.T) {
	t.Parallel()

	for _, ps := range []int{10, 25, 50, 100} {
		t.Run(fmt.Sprintf("page-size-%d", ps), func(t *testing.T) {
			t.Parallel()

			paging := portal.SonarPaging{PageIndex: 1, PageSize: ps, Total: 0}
			got := pageFooter(paging)
			want := fmt.Sprintf("no projects (page-size %d)", ps)

			if got != want {
				t.Errorf("pageFooter(%+v) = %q, want %q", paging, got, want)
			}
		})
	}
}

func TestList_PageFooter_SingularProject(t *testing.T) {
	t.Parallel()

	paging := portal.SonarPaging{PageIndex: 1, PageSize: 50, Total: 1}
	got := pageFooter(paging)

	if !strings.Contains(got, "1 project,") {
		t.Errorf("expected singular 'project', got: %s", got)
	}

	if strings.Contains(got, "projects") {
		t.Errorf("should not use plural 'projects' for total=1, got: %s", got)
	}
}
