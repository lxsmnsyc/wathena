package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/retention"
)

// configWith parses a config file with the assets filled in, and
// validates it.
func configWith(t *testing.T, text string, args ...string) (settings, error) {
	t.Helper()
	p := writeConfig(t, "listen: 127.0.0.1:8300\nassets: "+assetDir(t)+"\n"+text)
	o, _, err := parseOptions("waserver all", append([]string{"--config", p}, args...), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return validate(o, dbSecrets)
}

func hasWarning(st settings, part string) bool {
	return slices.ContainsFunc(st.Warnings, func(w string) bool { return strings.Contains(w, part) })
}

// TestRetentionConfig: the retention block's defaults, its range, the
// warnings above the defaults, and the deprecated chat.log_days
// (decision 0049).
func TestRetentionConfig(t *testing.T) {
	st, err := configWith(t, "")
	if err != nil || st.Retention != retention.Defaults || len(st.Warnings) != 0 {
		t.Fatalf("defaults: %+v %v %v", st.Retention, st.Warnings, err)
	}
	st, err = configWith(t, "retention:\n  login_ip_days: 1\n  chat_days: 1\n  item_log_days: 3650\n  audit_days: 30\n")
	if err != nil || st.Retention != (retention.Periods{LoginIPDays: 1, ChatDays: 1, ItemLogDays: 3650, AuditDays: 30}) {
		t.Fatalf("set: %+v %v", st.Retention, err)
	}
	// Longer than the default warns.
	if len(st.Warnings) != 1 || !hasWarning(st, "retention.item_log_days is 3650 days") {
		t.Errorf("warnings %v", st.Warnings)
	}
	// The item and zeny logs and the audit log keep at least 30 days,
	// the others at least 1.
	for bad, want := range map[string]string{
		"login_ip_days: 0": "is 1 to 3650 days", "chat_days: -1": "is 1 to 3650 days",
		"item_log_days: 3651": "is 30 to 3650 days", "item_log_days: 29": "is 30 to 3650 days",
		"audit_days: 0": "is 30 to 3650 days", "audit_days: 29": "is 30 to 3650 days",
	} {
		key := "retention." + strings.SplitN(bad, ":", 2)[0]
		if _, err := configWith(t, "retention:\n  "+bad+"\n"); err == nil || !strings.Contains(err.Error(), key+": "+want) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	if _, _, err := parseOptions("waserver all", []string{"--config", writeConfig(t, "retention:\n  chat_hours: 3\n")}, io.Discard); err == nil {
		t.Error("an unknown retention key was accepted")
	}

	// chat.log_days still works, with a warning.
	st, err = configWith(t, "chat:\n  log_days: 14\n")
	if err != nil || st.Retention.ChatDays != 14 || !hasWarning(st, "chat.log_days is deprecated") {
		t.Errorf("chat.log_days: %+v %v %v", st.Retention, st.Warnings, err)
	}
	// Both, the same: allowed, with the warning.
	st, err = configWith(t, "chat:\n  log_days: 14\nretention:\n  chat_days: 14\n")
	if err != nil || st.Retention.ChatDays != 14 || !hasWarning(st, "chat.log_days is deprecated") {
		t.Errorf("both the same: %+v %v %v", st.Retention, st.Warnings, err)
	}
	// Both, different: an error.
	if _, err := configWith(t, "chat:\n  log_days: 14\nretention:\n  chat_days: 30\n"); err == nil || !strings.Contains(err.Error(), "keep retention.chat_days only") {
		t.Errorf("both different: %v", err)
	}
	if _, err := configWith(t, "chat:\n  log_days: 4000\n"); err == nil || !strings.Contains(err.Error(), "chat.log_days: is 1 to 3650 days") {
		t.Errorf("chat.log_days out of range: %v", err)
	}
	// Above the default through chat.log_days warns too.
	if st, err := configWith(t, "chat:\n  log_days: 45\n"); err != nil || !hasWarning(st, "retention.chat_days is 45 days") {
		t.Errorf("chat.log_days 45: %v %v", st.Warnings, err)
	}
}

// TestClassicConfig: start_map and maps in the classic block, the
// deprecated top-level keys, and the flags that win over both (decision
// 0049).
func TestClassicConfig(t *testing.T) {
	st, err := configWith(t, "classic:\n  start_map: prt_fild08\n  maps: [prontera, prt_fild08]\n")
	if err != nil || len(st.Warnings) != 0 {
		t.Fatalf("classic block: %v %v", st.Warnings, err)
	}
	if start, maps := st.classicMaps(); start != "prt_fild08" || joinList(maps) != "prontera,prt_fild08" {
		t.Errorf("classic block: %s %v", start, maps)
	}
	// Procedural worlds ignore the block: the top level stays empty.
	if st.StartMap != "prontera" || len(st.Maps) != 0 {
		t.Errorf("the top level took the classic block: %s %v", st.StartMap, st.Maps)
	}
	// Flags win.
	st, err = configWith(t, "classic:\n  start_map: prt_fild08\n  maps: all\n", "--map", "prontera", "--maps", "prontera")
	if err != nil {
		t.Fatal(err)
	}
	if start, maps := st.classicMaps(); start != "prontera" || joinList(maps) != "prontera" {
		t.Errorf("flags: %s %v", start, maps)
	}
	// The old top-level keys still work, with warnings.
	st, err = configWith(t, "start_map: prt_fild08\nmaps: all\n")
	if err != nil || !hasWarning(st, "start_map at the top level is deprecated") || !hasWarning(st, "maps at the top level is deprecated") {
		t.Fatalf("top level: %v %v", st.Warnings, err)
	}
	if start, maps := st.classicMaps(); start != "prt_fild08" || joinList(maps) != "all" {
		t.Errorf("top level: %s %v", start, maps)
	}
	// Both, different: an error each.
	_, err = configWith(t, "start_map: prontera\nmaps: all\nclassic:\n  start_map: prt_fild08\n  maps: [prt_fild08]\n")
	if err == nil || !strings.Contains(err.Error(), "keep classic.start_map only") || !strings.Contains(err.Error(), "keep classic.maps only") {
		t.Errorf("both different: %v", err)
	}
	// Both, the same: allowed.
	if _, err := configWith(t, "maps: all\nclassic:\n  maps: all\n"); err != nil {
		t.Errorf("both the same: %v", err)
	}
}

// TestComposeConfig: deploy/compose/waserver.yml loads without warnings,
// and puts its maps in the classic block, so a procedural world runs it
// as it is.
func TestComposeConfig(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../deploy/compose/waserver.yml"))
	if err != nil {
		t.Fatal(err)
	}
	p := writeConfig(t, string(data))
	o, _, err := parseOptions("waserver all", []string{"--config", p, "--assets", assetDir(t), "--public-url", "https://game.example.com"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	st, err := validate(o, dbSecrets)
	if err != nil || len(st.Warnings) != 0 {
		t.Fatalf("%v %v", st.Warnings, err)
	}
	if len(st.Maps) != 0 || st.StartMap != "prontera" {
		t.Errorf("a procedural world would run %s %v", st.StartMap, st.Maps)
	}
	if start, maps := st.classicMaps(); start != "prontera" || joinList(maps) != "all" {
		t.Errorf("a classic world runs %s %v", start, maps)
	}
}
