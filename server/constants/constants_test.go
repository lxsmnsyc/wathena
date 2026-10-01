package constants

import "testing"

func TestModules(t *testing.T) {
	d := &Data{Constants: map[string]int64{"RC_DEMIHUMAN": 7, "SC_STUN": 2, "SC_2011RWC_SCROLL": 400, "W_2HSWORD": 3, "RC2_NONE": 0}}
	m := d.Modules()
	if m["races"].Values["demi_human"] != 7 || m["races"].Names[7] != "demi_human" {
		t.Errorf("races %+v", m["races"])
	}
	if m["statuses"].Values["stun"] != 2 || m["statuses"].Values["n2011rwc_scroll"] != 400 {
		t.Errorf("statuses %+v", m["statuses"])
	}
	if m["weapon_types"].Values["two_handed_sword"] != 3 || len(m["monster_groups"].Values) != 0 {
		t.Errorf("modules %+v %+v", m["weapon_types"], m["monster_groups"])
	}
	if v, param, ok := d.Lookup("rc_demihuman"); !ok || param || v != 7 {
		t.Errorf("lookup %d %v %v", v, param, ok)
	}
}
