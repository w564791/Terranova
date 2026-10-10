package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"
)

// Executor side of the approval double hash check (manifest_approval.go).

// verifyBundleInWorkDir checks that the bundle files in workDir are exactly
// the files of hand-off h: the archive is unpacked into a scratch directory
// (Unpack re-checks every entry and that it hashes to h.BundleHash; a failure
// there wraps manifestbundle.ErrIntegrity), then each bundle path is read
// back from workDir (regular files only, no symlinks) and the set re-hashed.
// A difference is an *ApprovalMismatchError (bundle_changed).
func verifyBundleInWorkDir(h *ManifestBundleHandoff, workDir string) error {
	scratch, err := os.MkdirTemp("", "bundle-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	want, err := manifestbundle.UnpackBytes(h.Archive, scratch, h.BundleHash, manifestbundle.DefaultLimits())
	if err != nil {
		return fmt.Errorf("verify manifest bundle (version %s): %w", h.VersionID, err)
	}
	onDisk := make([]manifestbundle.File, 0, len(want))
	for _, f := range want {
		p := filepath.Join(workDir, filepath.FromSlash(f.Path))
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			return &ApprovalMismatchError{Reason: ApprovalReasonBundleChanged, Detail: "bundle file " + f.Path + " is missing or not a regular file in the work directory"}
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read bundle file %s: %w", f.Path, err)
		}
		onDisk = append(onDisk, manifestbundle.File{Path: f.Path, Content: content, Mode: int(fi.Mode().Perm())})
	}
	got, err := manifestbundle.Hash(onDisk)
	if err != nil {
		return err
	}
	if got != h.BundleHash {
		return &ApprovalMismatchError{Reason: ApprovalReasonBundleChanged,
			Detail: fmt.Sprintf("work directory bundle hashes to %s, approved %s", got, h.BundleHash)}
	}
	return nil
}

// verifyManifestBundleBinding the deployment's hand-off bundle is the bundle
// expected (the run's bundle for the plan, approved_bundle_hash for the
// apply) and the files in workDir are that bundle.
func (s *TerraformExecutor) verifyManifestBundleBinding(workspace *models.Workspace, workDir, expected string) error {
	if expected == "" {
		return &ApprovalMismatchError{Reason: ApprovalReasonNotApproved, Detail: "no approved bundle hash"}
	}
	h, err := s.dataAccessor.GetManifestBundleByTag(*workspace.ManifestDeploymentID, *workspace.ManifestActiveTag)
	if err != nil {
		return fmt.Errorf("load manifest bundle: %w", err)
	}
	if h == nil {
		return fmt.Errorf("no manifest version found for deployment=%s tag=%s",
			*workspace.ManifestDeploymentID, *workspace.ManifestActiveTag)
	}
	if h.BundleHash != expected {
		return &ApprovalMismatchError{Reason: ApprovalReasonBundleChanged,
			Detail: fmt.Sprintf("deployment bundle is %s, expected %s", h.BundleHash, expected)}
	}
	return verifyBundleInWorkDir(h, workDir)
}

// verifyApprovedPlanFile the plan.out about to be applied is the approved one.
func verifyApprovedPlanFile(planFile, approvedPlanHash string) error {
	if approvedPlanHash == "" {
		return &ApprovalMismatchError{Reason: ApprovalReasonNotApproved, Detail: "no approved plan hash"}
	}
	data, err := os.ReadFile(planFile)
	if err != nil {
		return &ApprovalMismatchError{Reason: ApprovalReasonPlanChanged, Detail: "plan file unreadable"}
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != approvedPlanHash {
		return &ApprovalMismatchError{Reason: ApprovalReasonPlanChanged,
			Detail: fmt.Sprintf("plan.out hashes to %s, approved %s", got, approvedPlanHash)}
	}
	return nil
}

// verifyManifestApply the full pre-apply check of a manifest apply: recorded
// approval, bundle (hand-off and work directory) == approved_bundle_hash,
// plan.out == approved_plan_hash.
func (s *TerraformExecutor) verifyManifestApply(workspace *models.Workspace, workDir, planFile string, approval *ManifestApproval) error {
	if !approval.Approved() {
		return &ApprovalMismatchError{Reason: ApprovalReasonNotApproved, Detail: "the plan was not approved"}
	}
	if err := s.verifyManifestBundleBinding(workspace, workDir, approval.ApprovedBundleHash); err != nil {
		return err
	}
	return verifyApprovedPlanFile(planFile, approval.ApprovedPlanHash)
}

// isApprovalMismatch reports an approval refusal.
func isApprovalMismatch(err error) (*ApprovalMismatchError, bool) {
	var am *ApprovalMismatchError
	ok := errors.As(err, &am)
	return am, ok
}
