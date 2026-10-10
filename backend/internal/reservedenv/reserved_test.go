package reservedenv

import (
	"strings"
	"testing"
)

func TestMatchKey_Patterns(t *testing.T) {
	cases := map[string]string{
		"TF_CLI_ARGS":                  "TF_CLI_ARGS",
		"TF_CLI_ARGS_init":             "TF_CLI_ARGS",
		"tf_cli_args_INIT":             "TF_CLI_ARGS",
		"TF_CLI_CONFIG_FILE":           "TF_CLI_CONFIG_FILE",
		"TF_PLUGIN_CACHE_DIR":          "TF_PLUGIN_CACHE",
		"TF_HTTP_ADDRESS":              "TF_HTTP_",
		"TF_TOKEN_app.terraform.io":    "TF_TOKEN_",
		"TF_REGISTRY_CLIENT_TIMEOUT":   "TF_REGISTRY_",
		"GIT_SSL_NO_VERIFY":            "GIT_SSL_",
		"GIT_ASKPASS":                  "GIT_ASKPASS",
		"GIT_CONFIG_GLOBAL":            "GIT_CONFIG",
		"SSL_CERT_FILE":                "SSL_CERT_",
		"CURL_CA_BUNDLE":               "CURL_CA_BUNDLE",
		"NODE_TLS_REJECT_UNAUTHORIZED": "NODE_TLS_REJECT_UNAUTHORIZED",
		"http_proxy":                   "HTTP_PROXY",
		"HTTPS_PROXY":                  "HTTPS_PROXY",
		"No_Proxy":                     "NO_PROXY",
		"_TERRANOVA_CA_CERT":           "_TERRANOVA_",
		"IAC_EXEC_HTTP_PROXY":          "IAC_",
		"TF_IN_AUTOMATION":             "TF_IN_AUTOMATION",
		"TF_INPUT":                     "TF_INPUT",
	}
	for key, wantPrefix := range cases {
		h := MatchKey(key)
		if h == nil || h.ReservedPrefix != wantPrefix {
			t.Fatalf("%s: got %+v want prefix %s", key, h, wantPrefix)
		}
	}
	for _, ok := range []string{"AWS_REGION", "TF_LOG", "MY_TF_HTTP_X", "CUSTOM"} {
		if MatchKey(ok) != nil {
			t.Fatalf("%s must not be reserved", ok)
		}
	}
}

func TestFindHits_Multiple(t *testing.T) {
	hits := FindHits([]string{"AWS_REGION", "TF_CLI_ARGS_init", "HTTP_PROXY", "TF_CLI_ARGS_init"})
	if len(hits) != 2 {
		t.Fatalf("hits=%v", hits)
	}
}

func TestPrefixes_MatchesPatterns(t *testing.T) {
	p := Prefixes()
	if len(p) != len(Patterns()) {
		t.Fatal("length mismatch")
	}
	for i, pat := range Patterns() {
		if p[i] != pat.Pattern {
			t.Fatalf("[%d] %q != %q", i, p[i], pat.Pattern)
		}
	}
	joined := strings.Join(p, ",")
	if !strings.Contains(joined, "TF_CLI_ARGS") || !strings.Contains(joined, "HTTP_PROXY") {
		t.Fatal(joined)
	}
}
