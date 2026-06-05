package services

import (
	"reflect"
	"testing"
)

func TestNormalizeManifestSubpath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"  ", "", false},
		{"/", "", false},
		{"modules/x", "modules/x", false},
		{"/modules/x/", "modules/x", false},
		{"a\\b", "a/b", false},
		{"a/../b", "", true},
		{"./a", "", true},
		{"a//b", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeManifestSubpath(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("NormalizeManifestSubpath(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("NormalizeManifestSubpath(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestListWorkdirs(t *testing.T) {
	files := map[string][]byte{
		"main.tf":              nil,
		"variables.tf":         nil,
		"modules/net/main.tf":  nil,
		"modules/net/vars.tf":  nil,
		"modules/db/main.tf":   nil,
		"README.md":            nil, // 非 .tf 忽略
		"envs/prod/deep/x.tf":  nil,
	}
	got := ListWorkdirs(files)
	want := []string{"", "envs/prod/deep", "modules/db", "modules/net"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListWorkdirs()=%v want %v", got, want)
	}
}

func TestListWorkdirsAlwaysIncludesRoot(t *testing.T) {
	// 根下无 .tf 时,根仍应可选
	got := ListWorkdirs(map[string][]byte{"modules/x/main.tf": nil})
	if len(got) == 0 || got[0] != "" {
		t.Errorf("ListWorkdirs should always include root \"\", got %v", got)
	}
}
