package handlers

import "iac-platform/internal/manifestbundle"

// BundleRulesViolatedResponse 422 body of publish when the draft breaks the
// bundle rules. Problems carry only rule names and paths, never file content.
type BundleRulesViolatedResponse struct {
	Error    string                   `json:"error" example:"draft violates the bundle rules"`
	Code     string                   `json:"code" example:"bundle_rules_violated"`
	Problems []manifestbundle.Problem `json:"problems"`
}

// BundleRepublishRequiredResponse 409 body of install / upgrade / previews
// when the version has no valid bundle (bundle_hash NULL). Reason holds only
// rule names and paths.
type BundleRepublishRequiredResponse struct {
	Error     string `json:"error" example:"this version has no valid bundle; please republish it"`
	Code      string `json:"code" example:"bundle_republish_required"`
	VersionID string `json:"version_id" example:"mfv-01hxyz"`
	Reason    string `json:"reason" example:"denylisted_file @ prod.tfvars"`
}
