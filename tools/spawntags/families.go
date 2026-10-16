package main

// Families maps rAthena's map families to wAthena's biomes: the
// generator's field biomes and the dungeon tilesets (decision 0042). A
// map belongs to the family whose name is the longest start of the map's
// name, when at most 4 characters follow, such as prt_fild08a or
// gl_chyard_. Maps of no family are left out: towns, guild and quest
// maps, and rAthena's later lands, whose monsters are far above the
// transcendent levels.
var Families = map[string][]string{
	// Field biomes of generator-v2.
	"grassland": {"prt_fild", "lhz_fild", "yuno_fild", "hu_fild", "ein_fild", "rockmi"},
	"forest": {"pay_fild", "gef_fild", "mjolnir_", "man_fild", "spl_fild", "bif_fild", "ama_fild", "lou_fild",
		"ecl_fild", "mosk_fild", "prt_maze", "yggdrasil"},
	"desert": {"moc_fild", "dic_fild", "ra_fild"},
	"snow":   {"ve_fild", "xmas_fild"},
	"swamp":  {"um_fild", "ayo_fild", "dew_fild", "nif_fild", "niflheim", "bra_fild"},
	"beach":  {"cmd_fild", "gon_fild", "harboro"},
	// Dungeon tilesets of generator-v3.
	"cave": {"pay_dun", "mjo_dun", "orcsdun", "anthell", "ein_dun", "ice_dun", "xmas_dun", "beach_dun", "iz_dun",
		"abyss_", "dic_dun", "ayo_dun", "dew_dun", "arug_dun", "mag_dun", "thor_v", "bra_dun"},
	"crypt": {"gef_dun", "gl_cas", "gl_church", "gl_chyard", "gl_dun", "gl_in", "gl_knt", "gl_prison", "gl_sew",
		"gl_step", "abbey", "tur_dun", "lou_dun", "ama_dun", "prt_sewb", "moc_pryd", "in_sphinx"},
	"tower": {"c_tower", "alde_dun", "tha_t", "gon_dun", "kh_dun", "ecl_tdun", "odin_tem"},
	"ruins": {"gefenia", "juperos_", "ra_san", "lhz_dun", "glast_", "ba_2whs", "ba_bath", "ba_lib", "ba_lost",
		"ba_maison", "ba_pw", "treasure"},
}

// FieldBiomes are the biomes of fields. The others are dungeons'.
var FieldBiomes = []string{"grassland", "forest", "desert", "snow", "swamp", "beach"}

// maxRest is the most characters a map's name may have after its
// family's.
const maxRest = 4
