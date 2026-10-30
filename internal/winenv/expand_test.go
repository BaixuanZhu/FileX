package winenv_test

import (
	"testing"

	"filex/internal/winenv"
)

func TestExpandSimple(t *testing.T) {
	vars := winenv.Vars{
		"FOO": "hello",
		"BAR": "world",
	}

	result := winenv.ExpandEnv("%FOO% %BAR%", vars)
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func TestExpandNested(t *testing.T) {
	vars := winenv.Vars{
		"A": "%B%",
		"B": "resolved",
	}

	result := winenv.ExpandEnv("%A%", vars)
	if result != "resolved" {
		t.Errorf("got %q, want %q", result, "resolved")
	}
}

func TestExpandUndefined(t *testing.T) {
	vars := winenv.Vars{}

	result := winenv.ExpandEnv("%UNDEFINED%", vars)
	if result != "%UNDEFINED%" {
		t.Errorf("got %q, want %%UNDEFINED%%", result)
	}
}

func TestExpandCycleProtection(t *testing.T) {
	vars := winenv.Vars{
		"A": "%B%",
		"B": "%A%",
	}

	result := winenv.ExpandEnv("%A%", vars)
	if result == "%A%" || result == "%B%" {
		t.Logf("cycle resolved to %q (acceptable)", result)
	}
}

func TestComposeVars(t *testing.T) {
	system := map[string]string{
		"SYS":    "system_val",
		"SHARED": "from_system",
	}
	user := map[string]string{
		"USR":    "user_val",
		"SHARED": "from_user",
	}

	composed := winenv.ComposeVars(system, user)

	if composed["SYS"] != "system_val" {
		t.Errorf("SYS = %q, want system_val", composed["SYS"])
	}
	if composed["USR"] != "user_val" {
		t.Errorf("USR = %q, want user_val", composed["USR"])
	}
	if composed["SHARED"] != "from_user" {
		t.Errorf("SHARED = %q, want from_user (user overrides system)", composed["SHARED"])
	}
}

func TestComposeVarsExpansion(t *testing.T) {
	system := map[string]string{
		"BASE": "C:\\Windows",
	}
	user := map[string]string{
		"MYDIR": "%BASE%\\MyApp",
	}

	composed := winenv.ComposeVars(system, user)

	if composed["MYDIR"] != "C:\\Windows\\MyApp" {
		t.Errorf("MYDIR = %q, want C:\\Windows\\MyApp", composed["MYDIR"])
	}
}
