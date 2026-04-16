package botcommands

import (
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ValidateCommand (registration shape)
// ---------------------------------------------------------------------------

func TestValidateCommand_OK(t *testing.T) {
	req := RegisterCommandRequest{
		Name:        "weather",
		Description: "Get the weather",
		Options: []BotCommandOption{
			{Name: "city", Description: "City name", Type: OptionTypeString, Required: true},
		},
	}
	if err := ValidateCommand(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommand_BadName(t *testing.T) {
	cases := []string{"", "WITHUPPER", "has space", "toolong_name_toolong_name_toolong_name_xx", "punct!"}
	for _, n := range cases {
		req := RegisterCommandRequest{Name: n, Description: "x"}
		if err := ValidateCommand(req); err == nil {
			t.Errorf("expected error for name %q", n)
		} else if !errors.Is(err, ErrBadRequest) {
			t.Errorf("name %q: expected ErrBadRequest, got %v", n, err)
		}
	}
}

func TestValidateCommand_BadDescription(t *testing.T) {
	req := RegisterCommandRequest{Name: "ok", Description: ""}
	if err := ValidateCommand(req); err == nil {
		t.Error("empty description should fail")
	}
	req.Description = strings.Repeat("a", 101)
	if err := ValidateCommand(req); err == nil {
		t.Error("101-char description should fail")
	}
}

func TestValidateCommand_TooManyOptions(t *testing.T) {
	req := RegisterCommandRequest{Name: "cmd", Description: "ok"}
	for i := 0; i < MaxOptionsPerCommand+1; i++ {
		req.Options = append(req.Options, BotCommandOption{
			Name: "opt", Description: "d", Type: OptionTypeString,
		})
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("should reject more than MaxOptionsPerCommand options")
	}
}

func TestValidateCommand_DuplicateOption(t *testing.T) {
	req := RegisterCommandRequest{
		Name:        "cmd",
		Description: "ok",
		Options: []BotCommandOption{
			{Name: "a", Description: "d", Type: OptionTypeString},
			{Name: "a", Description: "d", Type: OptionTypeInteger},
		},
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("duplicate option names should fail")
	}
}

func TestValidateCommand_BadOptionType(t *testing.T) {
	req := RegisterCommandRequest{
		Name:        "cmd",
		Description: "ok",
		Options:     []BotCommandOption{{Name: "a", Description: "d", Type: "SUBCOMMAND"}},
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("unsupported option type should fail")
	}
}

func TestValidateCommand_StringChoicesMustBeStrings(t *testing.T) {
	req := RegisterCommandRequest{
		Name: "cmd", Description: "ok",
		Options: []BotCommandOption{{
			Name: "a", Description: "d", Type: OptionTypeString,
			Choices: []OptionChoice{{Name: "One", Value: 1.0}},
		}},
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("string option with integer choice should fail")
	}
}

func TestValidateCommand_BooleanCannotHaveChoices(t *testing.T) {
	req := RegisterCommandRequest{
		Name: "cmd", Description: "ok",
		Options: []BotCommandOption{{
			Name: "a", Description: "d", Type: OptionTypeBoolean,
			Choices: []OptionChoice{{Name: "Yes", Value: "yes"}},
		}},
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("boolean option with choices should fail")
	}
}

func TestValidateCommand_MinMaxValueRequiresInteger(t *testing.T) {
	mn, mx := 1.0, 10.0
	req := RegisterCommandRequest{
		Name: "cmd", Description: "ok",
		Options: []BotCommandOption{{
			Name: "a", Description: "d", Type: OptionTypeString,
			MinValue: &mn, MaxValue: &mx,
		}},
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("min/max_value on STRING should fail")
	}
}

func TestValidateCommand_MinMaxLengthRequiresString(t *testing.T) {
	mn, mx := 1, 10
	req := RegisterCommandRequest{
		Name: "cmd", Description: "ok",
		Options: []BotCommandOption{{
			Name: "a", Description: "d", Type: OptionTypeInteger,
			MinLength: &mn, MaxLength: &mx,
		}},
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("min/max_length on INTEGER should fail")
	}
}

func TestValidateCommand_MinGreaterThanMax(t *testing.T) {
	mn, mx := 10.0, 1.0
	req := RegisterCommandRequest{
		Name: "cmd", Description: "ok",
		Options: []BotCommandOption{{
			Name: "a", Description: "d", Type: OptionTypeInteger,
			MinValue: &mn, MaxValue: &mx,
		}},
	}
	if err := ValidateCommand(req); err == nil {
		t.Error("min_value > max_value should fail")
	}
}

// ---------------------------------------------------------------------------
// ValidateOptions (user-supplied values)
// ---------------------------------------------------------------------------

func TestValidateOptions_Required(t *testing.T) {
	schema := []BotCommandOption{
		{Name: "city", Description: "d", Type: OptionTypeString, Required: true},
	}
	_, err := ValidateOptions(schema, map[string]interface{}{})
	if err == nil {
		t.Fatal("missing required option should fail")
	}
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("expected ErrBadRequest, got %v", err)
	}
}

func TestValidateOptions_UnknownKey(t *testing.T) {
	schema := []BotCommandOption{
		{Name: "a", Description: "d", Type: OptionTypeString},
	}
	_, err := ValidateOptions(schema, map[string]interface{}{"b": "x"})
	if err == nil {
		t.Fatal("unknown option key should fail")
	}
}

func TestValidateOptions_WrongType(t *testing.T) {
	schema := []BotCommandOption{
		{Name: "n", Description: "d", Type: OptionTypeInteger},
	}
	_, err := ValidateOptions(schema, map[string]interface{}{"n": "abc"})
	if err == nil {
		t.Fatal("string value for INTEGER option should fail")
	}
}

func TestValidateOptions_StringLengthBounds(t *testing.T) {
	mn, mx := 2, 5
	schema := []BotCommandOption{
		{Name: "s", Description: "d", Type: OptionTypeString, MinLength: &mn, MaxLength: &mx},
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"s": "a"}); err == nil {
		t.Error("too short should fail")
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"s": "abcdef"}); err == nil {
		t.Error("too long should fail")
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"s": "abc"}); err != nil {
		t.Errorf("valid value failed: %v", err)
	}
}

func TestValidateOptions_IntegerBounds(t *testing.T) {
	mn, mx := 1.0, 10.0
	schema := []BotCommandOption{
		{Name: "n", Description: "d", Type: OptionTypeInteger, MinValue: &mn, MaxValue: &mx},
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"n": 0.0}); err == nil {
		t.Error("below min should fail")
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"n": 11.0}); err == nil {
		t.Error("above max should fail")
	}
	got, err := ValidateOptions(schema, map[string]interface{}{"n": 5.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := got["n"].(int64); !ok || v != 5 {
		t.Errorf("expected int64(5), got %T %v", got["n"], got["n"])
	}
}

func TestValidateOptions_IntegerRejectsFractional(t *testing.T) {
	schema := []BotCommandOption{
		{Name: "n", Description: "d", Type: OptionTypeInteger},
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"n": 3.14}); err == nil {
		t.Error("fractional INTEGER value should fail")
	}
}

func TestValidateOptions_Choices(t *testing.T) {
	schema := []BotCommandOption{
		{
			Name: "size", Description: "d", Type: OptionTypeString,
			Choices: []OptionChoice{
				{Name: "S", Value: "small"},
				{Name: "L", Value: "large"},
			},
		},
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"size": "medium"}); err == nil {
		t.Error("value outside choices should fail")
	}
	if _, err := ValidateOptions(schema, map[string]interface{}{"size": "small"}); err != nil {
		t.Errorf("valid choice should succeed: %v", err)
	}
}

func TestValidateOptions_BooleanOK(t *testing.T) {
	schema := []BotCommandOption{
		{Name: "flag", Description: "d", Type: OptionTypeBoolean},
	}
	out, err := ValidateOptions(schema, map[string]interface{}{"flag": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := out["flag"].(bool); !ok || !v {
		t.Errorf("expected flag=true, got %v", out["flag"])
	}
}

func TestValidateOptions_OptionalMissingAllowed(t *testing.T) {
	schema := []BotCommandOption{
		{Name: "x", Description: "d", Type: OptionTypeString, Required: false},
	}
	out, err := ValidateOptions(schema, map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, set := out["x"]; set {
		t.Error("optional missing value should not be set in output")
	}
}

// ---------------------------------------------------------------------------
// newToken
// ---------------------------------------------------------------------------

func TestNewToken_Is64CharHex(t *testing.T) {
	tok, err := newToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tok) != 64 {
		t.Errorf("expected 64 chars, got %d", len(tok))
	}
	for _, c := range tok {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("non-hex char %q in token %q", c, tok)
		}
	}
}
