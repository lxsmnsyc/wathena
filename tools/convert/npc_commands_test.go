package convert

import (
	"strings"
	"testing"
)

// TestLoopAndUnitConvert checks freeloop, which converts to nothing, and
// the reason of setunitdata's UNPC_GROUP_ID (decision 0072).
func TestLoopAndUnitConvert(t *testing.T) {
	res := questResult(t)
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: `{
	freeloop(1);
	for (.@i = 0; .@i < 3; .@i++) mes "a";
	freeloop(0);
	end;
}`})
	lua, err := n.npcScript()
	if err != nil || strings.Count(lua, "-- freeloop: the call's CPU budget stays.") != 2 {
		t.Errorf("%v\n%s", err, lua)
	}
	n = res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: `{ end; OnInit: setunitdata getnpcid(0),UNPC_GROUP_ID,7; end; }`})
	if _, err := n.npcScript(); err == nil || !strings.Contains(err.Error(), "setunitdata: UNPC_GROUP_ID, the group mark") {
		t.Errorf("got %v", err)
	}
}
