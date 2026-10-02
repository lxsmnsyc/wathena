package skill

// The skills that Plagiarism copies: those whose CopyFlags in rAthena's
// skill_db.yml name Plagiarism. skills.json does not carry CopyFlags, so
// the list is here. It is the same in both modes, and no skill has
// RemoveRequirement.
var plagiarismCopyable = map[int]bool{
	5:    true, // SM_BASH
	7:    true, // SM_MAGNUM
	11:   true, // MG_NAPALMBEAT
	13:   true, // MG_SOULSTRIKE
	14:   true, // MG_COLDBOLT
	15:   true, // MG_FROSTDIVER
	17:   true, // MG_FIREBALL
	18:   true, // MG_FIREWALL
	19:   true, // MG_FIREBOLT
	20:   true, // MG_LIGHTNINGBOLT
	21:   true, // MG_THUNDERSTORM
	24:   true, // AL_RUWACH
	28:   true, // AL_HEAL
	42:   true, // MC_MAMMONITE
	46:   true, // AC_DOUBLE
	47:   true, // AC_SHOWER
	52:   true, // TF_POISON
	54:   true, // ALL_RESURRECTION
	62:   true, // KN_BOWLINGBASH
	68:   true, // PR_ASPERSIO
	69:   true, // PR_BENEDICTIO
	70:   true, // PR_SANCTUARY
	77:   true, // PR_TURNUNDEAD
	79:   true, // PR_MAGNUS
	80:   true, // WZ_FIREPILLAR
	81:   true, // WZ_SIGHTRASHER
	83:   true, // WZ_METEOR
	84:   true, // WZ_JUPITEL
	85:   true, // WZ_VERMILION
	86:   true, // WZ_WATERBALL
	88:   true, // WZ_FROSTNOVA
	89:   true, // WZ_STORMGUST
	90:   true, // WZ_EARTHSPIKE
	91:   true, // WZ_HEAVENDRIVE
	116:  true, // HT_LANDMINE
	121:  true, // HT_FREEZINGTRAP
	122:  true, // HT_BLASTMINE
	123:  true, // HT_CLAYMORETRAP
	141:  true, // AS_SPLASHER
	148:  true, // AC_CHARGEARROW
	152:  true, // TF_THROWSTONE
	156:  true, // AL_HOLYLIGHT
	212:  true, // RG_BACKSTAP
	229:  true, // AM_DEMONSTRATION
	230:  true, // AM_ACIDTERROR
	250:  true, // CR_SHIELDCHARGE
	251:  true, // CR_SHIELDBOOMERANG
	253:  true, // CR_HOLYCROSS
	254:  true, // CR_GRANDCROSS
	263:  true, // MO_TRIPLEATTACK
	266:  true, // MO_INVESTIGATE
	267:  true, // MO_FINGEROFFENSIVE
	271:  true, // MO_EXTREMITYFIST
	272:  true, // MO_CHAINCOMBO
	337:  true, // ITM_TOMAHAWK
	421:  true, // TK_JUMPKICK
	526:  true, // NJ_ZENYNAGE
	527:  true, // NJ_TATAMIGAESHI
	528:  true, // NJ_KASUMIKIRI
	529:  true, // NJ_SHADOWJUMP
	530:  true, // NJ_KIRIKAGE
	531:  true, // NJ_UTSUSEMI
	534:  true, // NJ_KOUENKA
	535:  true, // NJ_KAENSIN
	536:  true, // NJ_BAKUENRYU
	537:  true, // NJ_HYOUSENSOU
	539:  true, // NJ_HYOUSYOURAKU
	540:  true, // NJ_HUUJIN
	541:  true, // NJ_RAIGEKISAI
	542:  true, // NJ_KAMAITACHI
	1001: true, // KN_CHARGEATK
	1004: true, // AS_VENOMKNIFE
	1006: true, // WZ_SIGHTBLASTER
	1009: true, // HT_PHANTASMIC
	1015: true, // MO_KITRANSLATION
	3008: true, // KO_MUCHANAGE
}

// CopyableByPlagiarism reports whether a skill's CopyFlags name
// Plagiarism (SKILL_COPY_PLAGIARISM).
func CopyableByPlagiarism(id int) bool { return plagiarismCopyable[id] }
