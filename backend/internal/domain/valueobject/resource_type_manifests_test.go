package valueobject

import "testing"

func TestResourceTypeManifests_IsOrganizationCatalogResource(t *testing.T) {
	if ResourceTypeManifests != "MANIFESTS" {
		t.Fatalf("ResourceTypeManifests = %q, want MANIFESTS", ResourceTypeManifests)
	}
	if !ResourceTypeManifests.IsValid() {
		t.Fatal("MANIFESTS must be a valid resource type")
	}
	if got := ResourceTypeManifests.GetScopeLevel(); got != ScopeTypeOrganization {
		t.Fatalf("MANIFESTS scope = %q, want ORGANIZATION", got)
	}
	for _, in := range []string{"MANIFESTS", "manifests"} {
		rt, err := ParseResourceType(in)
		if err != nil || rt != ResourceTypeManifests {
			t.Fatalf("ParseResourceType(%q) = %q, %v", in, rt, err)
		}
	}
}

// MANIFESTS is a catalog permission. It must never be satisfied by, or
// satisfy, a workspace capability: deploying still needs WORKSPACE_RESOURCES.
func TestResourceTypeManifests_DoesNotImplyWorkspaceCapabilities(t *testing.T) {
	if ResourceTypeWorkspaceResources.IsSatisfiedBy(ResourceTypeManifests) {
		t.Fatal("MANIFESTS must not satisfy WORKSPACE_RESOURCES")
	}
	if ResourceTypeManifests.IsSatisfiedBy(ResourceTypeWorkspaceManagement) {
		t.Fatal("WORKSPACE_MANAGEMENT must not satisfy MANIFESTS")
	}
	if ResourceTypeManifests.IsSatisfiedBy(ResourceTypeSystemSettings) {
		t.Fatal("SYSTEM_SETTINGS must not satisfy MANIFESTS")
	}
}
