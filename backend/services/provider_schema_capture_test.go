package services

import (
	"testing"

	"iac-platform/internal/models"
)

func TestParseProviderVersionsFromLock(t *testing.T) {
	lock := `
# This file is maintained automatically by "terraform init".

provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.100.0"
  constraints = ">= 5.0.0"
  hashes = [
    "h1:abc=",
  ]
}

provider "registry.terraform.io/hashicorp/random" {
  version = "3.6.0"
  hashes = [
    "h1:def=",
  ]
}
`
	refs, key, err := parseProviderVersionsFromLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("want 2 providers, got %d", len(refs))
	}
	// sorted by source
	if refs[0].Source != "registry.terraform.io/hashicorp/aws" || refs[0].Version != "5.100.0" {
		t.Fatalf("unexpected first: %+v", refs[0])
	}
	if refs[1].Source != "registry.terraform.io/hashicorp/random" || refs[1].Version != "3.6.0" {
		t.Fatalf("unexpected second: %+v", refs[1])
	}
	if key == "" || len(key) != 64 {
		t.Fatalf("bad versions key: %q", key)
	}

	// same content => same key
	_, key2, _ := parseProviderVersionsFromLock(lock)
	if key != key2 {
		t.Fatal("key not stable")
	}

	// version change => different key
	lock2 := `
provider "registry.terraform.io/hashicorp/aws" {
  version = "5.101.0"
}
provider "registry.terraform.io/hashicorp/random" {
  version = "3.6.0"
}
`
	_, key3, _ := parseProviderVersionsFromLock(lock2)
	if key3 == key {
		t.Fatal("expected key change on version bump")
	}
}

func TestProviderVersionsKeyEmpty(t *testing.T) {
	if providerVersionsKey(nil) != "" {
		t.Fatal("empty should be empty key")
	}
	if providerVersionsKey([]models.ProviderVersionRef{}) != "" {
		t.Fatal("empty slice should be empty key")
	}
}
