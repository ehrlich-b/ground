package api

import "testing"

func TestValidRole(t *testing.T) {
	accepted := []string{"extractor", "auditor", "both", "observer"}
	for _, r := range accepted {
		r := r
		t.Run("accepts_"+r, func(t *testing.T) {
			if !validRole(r) {
				t.Errorf("validRole(%q) = false, want true", r)
			}
		})
	}

	t.Run("rejects_admin_despite_being_a_real_role", func(t *testing.T) {
		if validRole("admin") {
			t.Error(`validRole("admin") = true, want false — validRole deliberately excludes "admin" even though it is a real role used by model.Agent.Role / EnsureAdminAgent`)
		}
	})

	t.Run("rejects_empty", func(t *testing.T) {
		if validRole("") {
			t.Error(`validRole("") = true, want false`)
		}
	})

	t.Run("rejects_case_variant", func(t *testing.T) {
		if validRole("Extractor") {
			t.Error(`validRole("Extractor") = true, want false — matching is case-sensitive`)
		}
	})
}

func TestValidSemantic(t *testing.T) {
	accepted := []string{"confirm", "misquote", "out_of_context", "weak", "broken_link"}
	for _, v := range accepted {
		v := v
		t.Run("accepts_"+v, func(t *testing.T) {
			if !validSemantic(v) {
				t.Errorf("validSemantic(%q) = false, want true", v)
			}
		})
	}

	t.Run("rejects_empty", func(t *testing.T) {
		if validSemantic("") {
			t.Error(`validSemantic("") = true, want false`)
		}
	})

	t.Run("rejects_plausible_but_wrong", func(t *testing.T) {
		if validSemantic("invalid") {
			t.Error(`validSemantic("invalid") = true, want false`)
		}
	})
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain_lowercase_and_space_to_dash", "Hello World", "hello-world"},
		{"leading_trailing_whitespace_trimmed", "  spaced out  ", "spaced-out"},
		{"separator_run_collapsed", "already--_dashed", "already-dashed"},
		{"punctuation_vanishes_no_gap", "Weird!@#$%Chars", "weirdchars"},
		{"empty_input", "", ""},
		{"dashes_only_turned_to_empty", "---", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := slugify(tc.in); got != tc.want {
				t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNullableString(t *testing.T) {
	t.Run("empty_returns_nil_pointer", func(t *testing.T) {
		if got := nullableString(""); got != nil {
			t.Errorf("nullableString(\"\") = %v, want nil pointer", got)
		}
	})

	t.Run("non_empty_returns_matching_pointer", func(t *testing.T) {
		got := nullableString("persisted")
		if got == nil {
			t.Fatal("nullableString(\"persisted\") = nil, want non-nil pointer")
		}
		if *got != "persisted" {
			t.Errorf("nullableString(\"persisted\") dereferences to %q, want %q", *got, "persisted")
		}
	})
}
