package provider

import "testing"

func TestValidateRuleInputs_GlobalRejectsGroupArtifact(t *testing.T) {
	if err := validateRuleInputs("global", "g", "a"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateRuleInputs_GlobalAcceptsEmpty(t *testing.T) {
	if err := validateRuleInputs("global", "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty scope defaults to global.
	if err := validateRuleInputs("", "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRuleInputs_ArtifactRequiresGroupArtifact(t *testing.T) {
	if err := validateRuleInputs("artifact", "", "a"); err == nil {
		t.Fatalf("expected error")
	}
	if err := validateRuleInputs("artifact", "g", ""); err == nil {
		t.Fatalf("expected error")
	}
	if err := validateRuleInputs("artifact", "g", "a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRuleInputs_ScopeMustBeKnown(t *testing.T) {
	if err := validateRuleInputs("nope", "", ""); err == nil {
		t.Fatalf("expected error")
	}
}
