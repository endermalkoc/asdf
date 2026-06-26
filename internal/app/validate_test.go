package app

import (
	"strings"
	"testing"
)

func TestValidateEnumSoft(t *testing.T) {
	allowed := []string{"feature", "entity"}

	// empty and known values pass silently (no note, no error)
	for _, v := range []string{"", "feature", "entity"} {
		if note, err := ValidateEnumSoft("kind", v, allowed, false); note != "" || err != nil {
			t.Errorf("value %q: want silent pass, got note=%q err=%v", v, note, err)
		}
	}

	// unknown + lenient → accepted with a note, no error
	note, err := ValidateEnumSoft("kind", "bogus", allowed, false)
	if err != nil || note == "" || !strings.Contains(note, "bogus") {
		t.Errorf("unknown lenient: want note mentioning the value and no error, got note=%q err=%v", note, err)
	}

	// unknown + strict → hard error
	if _, err := ValidateEnumSoft("kind", "bogus", allowed, true); err == nil {
		t.Errorf("unknown strict: want error, got nil")
	}
}
