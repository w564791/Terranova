package manifestbundle

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"gorm.io/gorm"
)

// HCL static-check rule names (publish rules, see CheckHCL). Like every
// Problem they carry only a rule name, a path and a 1-based line.
const (
	RuleHCLParseError   = "hcl_parse_error"
	RuleHCLProvisioner  = "hcl_provisioner"
	RuleHCLExternalData = "hcl_external_data"
	RuleHCLHTTPData     = "hcl_http_data"
	RuleHCLModuleSource = "hcl_module_source"
)

func hclRuleMessage(rule string) string {
	switch rule {
	case RuleHCLParseError:
		return "Terraform configuration file could not be parsed"
	case RuleHCLProvisioner:
		return "provisioner blocks (local-exec, remote-exec, file, ...) are not allowed"
	case RuleHCLExternalData:
		return `the "external" data source / hashicorp/external provider is not allowed`
	case RuleHCLHTTPData:
		return `the "http" data source / hashicorp/http provider is not allowed`
	case RuleHCLModuleSource:
		return "module source is not allowed: use a relative path inside the bundle or a module registered in the platform module catalog"
	}
	return rule
}

// IsTerraformConfigFile reports files Terraform / OpenTofu load as
// configuration: *.tf, *.tf.json (override.tf, *_override.tf and their
// .json forms are covered by the suffixes) and OpenTofu's *.tofu,
// *.tofu.json. Matching is case-insensitive and also covers names Terraform
// itself would ignore (leading '.', '#', trailing '~'): over-checking is the
// safe direction.
func IsTerraformConfigFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	for _, s := range []string{".tf", ".tf.json", ".tofu", ".tofu.json"} {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	return false
}

func isJSONConfig(p string) bool {
	base := strings.ToLower(path.Base(p))
	return strings.HasSuffix(base, ".tf.json") || strings.HasSuffix(base, ".tofu.json")
}

// ModuleSourcePolicy decides whether a non-local module source string is
// allowed. Relative local paths are decided by CheckHCL itself and never
// reach the policy. nil = local paths only.
type ModuleSourcePolicy func(source string) (bool, error)

// LocalModulesOnly allows no remote/registry module at all (only relative
// paths inside the bundle). Untrusted-bundle checks use it: remote modules
// are not part of the bundle and cannot be inspected.
var LocalModulesOnly ModuleSourcePolicy = nil

// ModuleSourceAllowlist is an exact-match allowlist; an entry also allows
// "<entry>//<subdir>" (Terraform's module sub-directory form).
func ModuleSourceAllowlist(entries []string) ModuleSourcePolicy {
	set := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e = strings.TrimSpace(e); e != "" {
			set[e] = true
		}
	}
	return func(source string) (bool, error) { return allowlisted(set, source), nil }
}

func allowlisted(set map[string]bool, source string) bool {
	if set[source] {
		return true
	}
	// "<entry>//<subdir>[?query]": split on the first "//" that is not the
	// "://" of a URL scheme
	rest, off := source, 0
	if j := strings.Index(rest, "://"); j >= 0 {
		off = j + 3
		rest = source[off:]
	}
	if k := strings.Index(rest, "//"); k > 0 {
		return set[source[:off+k]]
	}
	return false
}

// PublishModuleSourcePolicy is THE module-source allowlist of native publish
// (single place; step 8 git sources extend it here). Besides relative local
// paths (handled by CheckHCL) a module source is allowed when it is the
// module_source of an active module of the platform module catalog
// (modules.module_source, or a module_versions.module_source of an active
// module): exactly the sources the manifest editor offers. The repo has no
// other module-source / registry allowlist configuration. The catalog is
// loaded lazily on the first non-local source, so bundles with only local
// modules never touch the database.
func PublishModuleSourcePolicy(ctx context.Context, db *gorm.DB) ModuleSourcePolicy {
	var (
		once sync.Once
		set  map[string]bool
		err  error
	)
	return func(source string) (bool, error) {
		once.Do(func() {
			var rows []string
			err = db.WithContext(ctx).Raw(`
				SELECT m.module_source FROM modules m
				 WHERE m.status = 'active' AND COALESCE(m.module_source, '') <> ''
				UNION
				SELECT mv.module_source FROM module_versions mv
				  JOIN modules m ON m.id = mv.module_id
				 WHERE m.status = 'active' AND COALESCE(mv.module_source, '') <> ''`).Scan(&rows).Error
			if err != nil {
				err = fmt.Errorf("load module source allowlist: %w", err)
				return
			}
			set = make(map[string]bool, len(rows))
			for _, r := range rows {
				if r = strings.TrimSpace(r); r != "" {
					set[r] = true
				}
			}
		})
		if err != nil {
			return false, err
		}
		return allowlisted(set, source), nil
	}
}

// localModuleSource reports a Terraform local path source ("./x", "../x")
// that stays inside the bundle when resolved against the directory of the
// declaring file. Escaping paths are rejected (inBundle false).
func localModuleSource(file, source string) (local, inBundle bool) {
	if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") {
		return false, false
	}
	if strings.ContainsRune(source, '\\') {
		return true, false
	}
	resolved := path.Clean(path.Join(path.Dir(file), source))
	return true, resolved != ".." && !strings.HasPrefix(resolved, "../") && !strings.HasPrefix(resolved, "/")
}

var (
	topSchema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "resource", LabelNames: []string{"type", "name"}},
		{Type: "data", LabelNames: []string{"type", "name"}},
		{Type: "module", LabelNames: []string{"name"}},
		{Type: "check", LabelNames: []string{"name"}},
		{Type: "removed"},
		{Type: "terraform"},
	}}
	provisionerSchema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "provisioner", LabelNames: []string{"type"}},
	}}
	checkSchema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "data", LabelNames: []string{"type", "name"}},
	}}
	terraformSchema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "required_providers"},
	}}
	moduleSchema = &hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "source"}}}
)

// CheckHCL is the static exec-capability check of a bundle. Every Terraform
// configuration file (IsTerraformConfigFile, any depth: local modules are
// loaded too) is parsed with hclparse (native syntax or JSON) and inspected
// through body schemas, never with regular expressions. It reports:
//   - hcl_parse_error: the file (or a block we inspect) does not parse; a
//     parse failure is always a problem, never skipped;
//   - hcl_provisioner: any provisioner block (any type, any `when`) in any
//     resource (null_resource, terraform_data, ...) or removed block;
//   - hcl_external_data / hcl_http_data: data "external" / data "http"
//     (top level or nested in a check block), and required_providers entries
//     whose source is hashicorp/external / hashicorp/http (aliasing the
//     provider under another local name);
//   - hcl_module_source: a module whose source is not a static string, is not
//     a relative local path that stays inside the bundle, and is not allowed
//     by policy (nil = local paths only).
//
// Problems carry file and the 1-based line of the block / attribute. Sorted
// like Validate. A policy error aborts the check.
func CheckHCL(files []File, policy ModuleSourcePolicy) ([]Problem, error) {
	var out []Problem
	add := func(rule, file string, line int) {
		if line < 1 {
			line = 1
		}
		out = append(out, Problem{File: file, Line: line, Rule: rule, Message: hclRuleMessage(rule)})
	}
	parser := hclparse.NewParser()
	for _, f := range files {
		if !IsTerraformConfigFile(f.Path) {
			continue
		}
		var (
			file  *hcl.File
			diags hcl.Diagnostics
		)
		if isJSONConfig(f.Path) {
			file, diags = parser.ParseJSON(f.Content, f.Path)
		} else {
			file, diags = parser.ParseHCL(f.Content, f.Path)
		}
		if diags.HasErrors() || file == nil || file.Body == nil {
			add(RuleHCLParseError, f.Path, diagLine(diags))
			continue
		}
		if err := checkBody(f.Path, file.Body, policy, add); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Rule < out[j].Rule
	})
	return out, nil
}

func diagLine(diags hcl.Diagnostics) int {
	for _, d := range diags {
		if d.Severity == hcl.DiagError && d.Subject != nil {
			return d.Subject.Start.Line
		}
	}
	return 1
}

func checkBody(file string, body hcl.Body, policy ModuleSourcePolicy, add func(rule, file string, line int)) error {
	content, _, diags := body.PartialContent(topSchema)
	if diags.HasErrors() {
		// malformed block header (wrong label count, ...): cannot be inspected
		add(RuleHCLParseError, file, diagLine(diags))
	}
	if content == nil {
		return nil
	}
	provisioners := func(b *hcl.Block) {
		pc, _, d := b.Body.PartialContent(provisionerSchema)
		if d.HasErrors() {
			add(RuleHCLParseError, file, diagLine(d))
		}
		if pc == nil {
			return
		}
		for _, p := range pc.Blocks {
			add(RuleHCLProvisioner, file, p.DefRange.Start.Line)
		}
	}
	dataSource := func(b *hcl.Block) {
		switch b.Labels[0] {
		case "external":
			add(RuleHCLExternalData, file, b.DefRange.Start.Line)
		case "http":
			add(RuleHCLHTTPData, file, b.DefRange.Start.Line)
		}
	}
	for _, b := range content.Blocks {
		switch b.Type {
		case "resource", "removed":
			provisioners(b)
		case "data":
			dataSource(b)
		case "check":
			cc, _, d := b.Body.PartialContent(checkSchema)
			if d.HasErrors() {
				add(RuleHCLParseError, file, diagLine(d))
			}
			if cc != nil {
				for _, db := range cc.Blocks {
					dataSource(db)
				}
			}
		case "terraform":
			checkRequiredProviders(file, b, add)
		case "module":
			if err := checkModuleSource(file, b, policy, add); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkModuleSource(file string, b *hcl.Block, policy ModuleSourcePolicy, add func(rule, file string, line int)) error {
	mc, _, d := b.Body.PartialContent(moduleSchema)
	if d.HasErrors() || mc == nil {
		add(RuleHCLModuleSource, file, b.DefRange.Start.Line)
		return nil
	}
	attr, ok := mc.Attributes["source"]
	if !ok {
		add(RuleHCLModuleSource, file, b.DefRange.Start.Line)
		return nil
	}
	line := attr.Range.Start.Line
	v, vd := attr.Expr.Value(nil)
	if vd.HasErrors() || !v.IsKnown() || v.IsNull() || v.Type() != cty.String {
		add(RuleHCLModuleSource, file, line) // only static strings are inspectable
		return nil
	}
	source := v.AsString()
	if local, inBundle := localModuleSource(file, source); local {
		if !inBundle {
			add(RuleHCLModuleSource, file, line)
		}
		return nil
	}
	if policy == nil {
		add(RuleHCLModuleSource, file, line)
		return nil
	}
	allowed, err := policy(source)
	if err != nil {
		return err
	}
	if !allowed {
		add(RuleHCLModuleSource, file, line)
	}
	return nil
}

// checkRequiredProviders flags hashicorp/external and hashicorp/http mapped
// under any local name (data "x" with local name x -> hashicorp/external
// would otherwise evade the data-source type check).
func checkRequiredProviders(file string, tb *hcl.Block, add func(rule, file string, line int)) {
	tc, _, d := tb.Body.PartialContent(terraformSchema)
	if d.HasErrors() {
		add(RuleHCLParseError, file, diagLine(d))
	}
	if tc == nil {
		return
	}
	for _, rp := range tc.Blocks {
		attrs, ad := rp.Body.JustAttributes()
		if ad.HasErrors() {
			add(RuleHCLParseError, file, diagLine(ad))
		}
		for _, a := range attrs {
			v, vd := a.Expr.Value(nil)
			if vd.HasErrors() || !v.IsKnown() || v.IsNull() || !(v.Type().IsObjectType() || v.Type().IsMapType()) {
				continue // legacy version-string form: provider resolved by local name (covered by data type check)
			}
			if v.Type().IsObjectType() && !v.Type().HasAttribute("source") {
				continue
			}
			src := v.GetAttr("source")
			if src.IsNull() || !src.IsKnown() || src.Type() != cty.String {
				continue
			}
			switch normalizeProviderSource(src.AsString()) {
			case "hashicorp/external":
				add(RuleHCLExternalData, file, a.Range.Start.Line)
			case "hashicorp/http":
				add(RuleHCLHTTPData, file, a.Range.Start.Line)
			}
		}
	}
}

func normalizeProviderSource(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"registry.terraform.io/", "registry.opentofu.org/"} {
		s = strings.TrimPrefix(s, p)
	}
	if !strings.Contains(s, "/") {
		s = "hashicorp/" + s
	}
	return s
}

// ValidateForPublish is the full native publish rule set: Validate plus
// CheckHCL with the given module-source policy (PublishModuleSourcePolicy).
// CheckHCL is publish-only: migrations and re-evaluation of stored versions
// keep using Validate, so existing valid-hash versions are not invalidated
// retroactively. With too_many_files only that single problem is returned.
func ValidateForPublish(files []File, policy ModuleSourcePolicy) ([]Problem, error) {
	problems := Validate(files)
	if len(problems) == 1 && problems[0].Rule == RuleTooManyFiles {
		return problems, nil
	}
	hclProblems, err := CheckHCL(files, policy)
	if err != nil {
		return nil, err
	}
	problems = append(problems, hclProblems...)
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].Rule < problems[j].Rule
	})
	return problems, nil
}

// PackFilesForPublish is PackFiles with ValidateForPublish.
func PackFilesForPublish(files []File, policy ModuleSourcePolicy) (*Bundle, []Problem, error) {
	problems, err := ValidateForPublish(files, policy)
	if err != nil {
		return nil, nil, err
	}
	if len(problems) > 0 {
		return nil, problems, nil
	}
	return PackFiles(files)
}
