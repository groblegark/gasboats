package model

import "testing"

func TestKind_IsValid(t *testing.T) {
	for _, tc := range []struct {
		kind Kind
		want bool
	}{
		{KindIssue, true},
		{KindData, true},
		{KindConfig, true},
		{Kind(""), false},
		{Kind("bogus"), false},
	} {
		if got := tc.kind.IsValid(); got != tc.want {
			t.Errorf("Kind(%q).IsValid() = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestKind_String(t *testing.T) {
	for _, tc := range []struct {
		kind Kind
		want string
	}{
		{KindIssue, "issue"},
		{KindData, "data"},
		{KindConfig, "config"},
	} {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("Kind(%q).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestBeadType_IsValid(t *testing.T) {
	for _, tc := range []struct {
		typ  BeadType
		want bool
	}{
		{TypeEpic, true},
		{TypeTask, true},
		{TypeFeature, true},
		{TypeChore, true},
		{TypeBug, true},
		{BeadType("workflow"), true},
		{BeadType(""), false},
	} {
		if got := tc.typ.IsValid(); got != tc.want {
			t.Errorf("BeadType(%q).IsValid() = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

func TestBeadType_String(t *testing.T) {
	for _, tc := range []struct {
		typ  BeadType
		want string
	}{
		{TypeTask, "task"},
		{TypeBug, "bug"},
		{BeadType("custom"), "custom"},
	} {
		if got := tc.typ.String(); got != tc.want {
			t.Errorf("BeadType(%q).String() = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	for _, tc := range []struct {
		status Status
		want   bool
	}{
		{StatusOpen, true},
		{StatusInProgress, true},
		{StatusDeferred, true},
		{StatusClosed, true},
		{StatusBlocked, true},
		{Status(""), false},
		{Status("deleted"), false},
	} {
		if got := tc.status.IsValid(); got != tc.want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestStatus_String(t *testing.T) {
	for _, tc := range []struct {
		status Status
		want   string
	}{
		{StatusOpen, "open"},
		{StatusClosed, "closed"},
		{StatusInProgress, "in_progress"},
	} {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("Status(%q).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestKindFor(t *testing.T) {
	for _, tc := range []struct {
		typ  BeadType
		want Kind
	}{
		{TypeEpic, KindIssue},
		{TypeTask, KindIssue},
		{TypeFeature, KindIssue},
		{TypeChore, KindIssue},
		{TypeBug, KindIssue},
		{TypeMolecule, KindIssue},
		{TypeAdvice, KindData},
		{TypeJack, KindData},
		{TypeFormula, KindData},
		{TypeDecision, Kind("")},
		{TypeReport, Kind("")},
		{BeadType("workflow"), Kind("")},
		{BeadType(""), Kind("")},
	} {
		if got := KindFor(tc.typ); got != tc.want {
			t.Errorf("KindFor(%q) = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

func TestTypeAliases(t *testing.T) {
	for _, tc := range []struct {
		deprecated BeadType
		canonical  BeadType
	}{
		{"template", TypeFormula},
		{"bundle", TypeMolecule},
	} {
		got, ok := TypeAliases[tc.deprecated]
		if !ok {
			t.Errorf("TypeAliases[%q] not found", tc.deprecated)
			continue
		}
		if got != tc.canonical {
			t.Errorf("TypeAliases[%q] = %q, want %q", tc.deprecated, got, tc.canonical)
		}
	}

	// Canonical types should not be aliased.
	for _, canonical := range []BeadType{TypeFormula, TypeMolecule} {
		if _, ok := TypeAliases[canonical]; ok {
			t.Errorf("TypeAliases[%q] should not exist; canonical types must not alias", canonical)
		}
	}
}

func TestDependencyType_IsValid(t *testing.T) {
	for _, tc := range []struct {
		dep  DependencyType
		want bool
	}{
		{DepBlocks, true},
		{DepParentChild, true},
		{DepRelated, true},
		{DepDuplicates, true},
		{DepSupersedes, true},
		{DependencyType("custom-dep"), true},
		{DependencyType(""), false},
		{DependencyType("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), false}, // 51 chars
	} {
		if got := tc.dep.IsValid(); got != tc.want {
			t.Errorf("DependencyType(%q).IsValid() = %v, want %v", tc.dep, got, tc.want)
		}
	}
}

func TestIsValidJackAction(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   bool
	}{
		{"edit", true},
		{"exec", true},
		{"patch", true},
		{"delete", true},
		{"create", true},
		{"", false},
		{"drop", false},
		{"Edit", false}, // case-sensitive
	} {
		if got := IsValidJackAction(tc.action); got != tc.want {
			t.Errorf("IsValidJackAction(%q) = %v, want %v", tc.action, got, tc.want)
		}
	}
}
