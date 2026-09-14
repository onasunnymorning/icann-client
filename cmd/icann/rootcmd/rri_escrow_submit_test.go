package rootcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates a file with placeholder content, creating parents as needed.
func writeFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<report/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExpandReportPaths(t *testing.T) {
	root := t.TempDir()
	reports := filepath.Join(root, "reports")
	// Deliberately created out of order: the directory listing must come back
	// lexically, which for these names is chronological.
	c := writeFile(t, filepath.Join(reports, "example-20250103-diff.xml"))
	a := writeFile(t, filepath.Join(reports, "example-20250101-full.xml"))
	b := writeFile(t, filepath.Join(reports, "example-20250102-diff.xml"))
	writeFile(t, filepath.Join(reports, "notes.txt"))
	nested := writeFile(t, filepath.Join(reports, "archive", "example-20241231-full.xml"))
	loose := writeFile(t, filepath.Join(root, "single.xml"))
	bracket := writeFile(t, filepath.Join(root, "odd[1].xml"))

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "directory yields its xml entries in lexical order, non-recursively",
			args: []string{reports},
			want: []string{a, b, c},
		},
		{
			name: "explicit files keep the given order",
			args: []string{c, a},
			want: []string{c, a},
		},
		{
			name: "a quoted glob is expanded",
			args: []string{filepath.Join(reports, "*.xml")},
			want: []string{a, b, c},
		},
		{
			name: "duplicates are collapsed, preserving first-seen order",
			args: []string{a, reports, a},
			want: []string{a, b, c},
		},
		{
			name: "a literal path wins over glob metacharacters",
			args: []string{bracket},
			want: []string{bracket},
		},
		{
			name: "a nested report is only reached explicitly",
			args: []string{nested, loose},
			want: []string{nested, loose},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandReportPaths(tt.args)
			if err != nil {
				t.Fatalf("expandReportPaths() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("expandReportPaths() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != filepath.Clean(tt.want[i]) {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExpandReportPathsErrors(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "missing path",
			args:    []string{filepath.Join(root, "nope.xml")},
			wantMsg: "nope.xml",
		},
		{
			// filepath.Glob returns (nil, nil) for no matches, which would
			// otherwise silently submit nothing.
			name:    "glob matching nothing",
			args:    []string{filepath.Join(root, "*.xml")},
			wantMsg: "matched no files",
		},
		{
			name:    "directory with no reports",
			args:    []string{empty},
			wantMsg: "no .xml reports",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandReportPaths(tt.args)
			if err == nil {
				t.Fatalf("expandReportPaths() = %v, want an error", got)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantMsg)
			}
		})
	}
}

func TestProgressLine(t *testing.T) {
	got := progressLine(submissionResult{
		File: "reports/a.xml", Status: statusAccepted, ID: "an-id", ResultCode: 1000,
	})
	for _, want := range []string{"accepted", "reports/a.xml", "id=an-id", "code=1000"} {
		if !strings.Contains(got, want) {
			t.Errorf("progressLine() = %q, want it to contain %q", got, want)
		}
	}
}
