package native

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultBoardValidates(t *testing.T) {
	if err := DefaultBoard().Validate(); err != nil {
		t.Fatalf("default board should validate: %v", err)
	}
}

func TestBoardValidate(t *testing.T) {
	cases := []struct {
		name    string
		b       *Board
		wantErr string
	}{
		{"empty states", &Board{}, "at least one state required"},
		{"empty state name", &Board{States: []State{{Name: ""}}}, "non-empty"},
		{"duplicate state", &Board{States: []State{{Name: "a"}, {Name: "a"}}}, "duplicate state name"},
		{"empty field name", &Board{
			States: []State{{Name: "ready"}},
			Fields: []Field{{Name: "", Type: FieldText}},
		}, "field name must be non-empty"},
		{"duplicate field", &Board{
			States: []State{{Name: "ready"}},
			Fields: []Field{{Name: "x", Type: FieldText}, {Name: "x", Type: FieldText}},
		}, "duplicate field name"},
		{"enum without values", &Board{
			States: []State{{Name: "ready"}},
			Fields: []Field{{Name: "sev", Type: FieldEnum}},
		}, "enum_values"},
		{"unknown field type", &Board{
			States: []State{{Name: "ready"}},
			Fields: []Field{{Name: "x", Type: "blob"}},
		}, "unknown type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.b.Validate()
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateFieldValues(t *testing.T) {
	b := &Board{
		States: []State{{Name: "ready"}},
		Fields: []Field{
			{Name: "severity", Type: FieldEnum, EnumValues: []string{"low", "high"}},
			{Name: "count", Type: FieldNumber},
			{Name: "due", Type: FieldDate},
			{Name: "urgent", Type: FieldBool},
			{Name: "note", Type: FieldText, Required: true},
		},
	}
	if err := b.Validate(); err != nil {
		t.Fatalf("board invalid: %v", err)
	}

	t.Run("happy path", func(t *testing.T) {
		err := b.ValidateFieldValues(map[string]any{
			"severity": "low",
			"count":    3,
			"due":      time.Now().UTC().Format(time.RFC3339),
			"urgent":   true,
			"note":     "hi",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		err := b.ValidateFieldValues(map[string]any{"note": "hi", "unknown": 1})
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("want unknown-field error, got %v", err)
		}
	})

	t.Run("enum out of range", func(t *testing.T) {
		err := b.ValidateFieldValues(map[string]any{"note": "hi", "severity": "nope"})
		if err == nil || !strings.Contains(err.Error(), "enum_values") {
			t.Fatalf("want enum error, got %v", err)
		}
	})

	t.Run("number wrong type", func(t *testing.T) {
		err := b.ValidateFieldValues(map[string]any{"note": "hi", "count": "three"})
		if err == nil || !strings.Contains(err.Error(), "expected number") {
			t.Fatalf("want number error, got %v", err)
		}
	})

	t.Run("date bad format", func(t *testing.T) {
		err := b.ValidateFieldValues(map[string]any{"note": "hi", "due": "yesterday"})
		if err == nil || !strings.Contains(err.Error(), "invalid date") {
			t.Fatalf("want date error, got %v", err)
		}
	})

	t.Run("required missing", func(t *testing.T) {
		err := b.ValidateFieldValues(map[string]any{"severity": "low"})
		if err == nil || !strings.Contains(err.Error(), `required field "note"`) {
			t.Fatalf("want required error, got %v", err)
		}
	})
}
