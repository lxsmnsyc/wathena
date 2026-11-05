package modscript

// rathenaJobNames are the names of rAthena's job_name (pc.cpp), from
// conf/msg_conf/map_msg.conf, by job ID: what jobname gives for core's
// jobs. A job ID that is not here is "Unknown Job" (msg 655).
var rathenaJobNames = map[int]string{
	0:    "Novice",                // JOB_NOVICE
	1:    "Swordsman",             // JOB_SWORDMAN
	2:    "Magician",              // JOB_MAGE
	3:    "Archer",                // JOB_ARCHER
	4:    "Acolyte",               // JOB_ACOLYTE
	5:    "Merchant",              // JOB_MERCHANT
	6:    "Thief",                 // JOB_THIEF
	7:    "Knight",                // JOB_KNIGHT
	8:    "Priest",                // JOB_PRIEST
	9:    "Wizard",                // JOB_WIZARD
	10:   "Blacksmith",            // JOB_BLACKSMITH
	11:   "Hunter",                // JOB_HUNTER
	12:   "Assassin",              // JOB_ASSASSIN
	13:   "Knight",                // JOB_KNIGHT2
	14:   "Crusader",              // JOB_CRUSADER
	15:   "Monk",                  // JOB_MONK
	16:   "Sage",                  // JOB_SAGE
	17:   "Rogue",                 // JOB_ROGUE
	18:   "Alchemist",             // JOB_ALCHEMIST
	19:   "Bard",                  // JOB_BARD
	20:   "Dancer",                // JOB_DANCER
	21:   "Crusader",              // JOB_CRUSADER2
	22:   "Wedding",               // JOB_WEDDING
	23:   "Super Novice",          // JOB_SUPER_NOVICE
	24:   "Gunslinger",            // JOB_GUNSLINGER
	25:   "Ninja",                 // JOB_NINJA
	26:   "Christmas",             // JOB_XMAS
	27:   "Summer",                // JOB_SUMMER
	28:   "Hanbok",                // JOB_HANBOK
	29:   "Oktoberfest",           // JOB_OKTOBERFEST
	30:   "Summer",                // JOB_SUMMER2
	4001: "High Novice",           // JOB_NOVICE_HIGH
	4002: "High Swordsman",        // JOB_SWORDMAN_HIGH
	4003: "High Magician",         // JOB_MAGE_HIGH
	4004: "High Archer",           // JOB_ARCHER_HIGH
	4005: "High Acolyte",          // JOB_ACOLYTE_HIGH
	4006: "High Merchant",         // JOB_MERCHANT_HIGH
	4007: "High Thief",            // JOB_THIEF_HIGH
	4008: "Lord Knight",           // JOB_LORD_KNIGHT
	4009: "High Priest",           // JOB_HIGH_PRIEST
	4010: "High Wizard",           // JOB_HIGH_WIZARD
	4011: "Whitesmith",            // JOB_WHITESMITH
	4012: "Sniper",                // JOB_SNIPER
	4013: "Assassin Cross",        // JOB_ASSASSIN_CROSS
	4014: "Lord Knight",           // JOB_LORD_KNIGHT2
	4015: "Paladin",               // JOB_PALADIN
	4016: "Champion",              // JOB_CHAMPION
	4017: "Professor",             // JOB_PROFESSOR
	4018: "Stalker",               // JOB_STALKER
	4019: "Creator",               // JOB_CREATOR
	4020: "Clown",                 // JOB_CLOWN
	4021: "Gypsy",                 // JOB_GYPSY
	4022: "Paladin",               // JOB_PALADIN2
	4023: "Baby Novice",           // JOB_BABY
	4024: "Baby Swordsman",        // JOB_BABY_SWORDMAN
	4025: "Baby Magician",         // JOB_BABY_MAGE
	4026: "Baby Archer",           // JOB_BABY_ARCHER
	4027: "Baby Acolyte",          // JOB_BABY_ACOLYTE
	4028: "Baby Merchant",         // JOB_BABY_MERCHANT
	4029: "Baby Thief",            // JOB_BABY_THIEF
	4030: "Baby Knight",           // JOB_BABY_KNIGHT
	4031: "Baby Priest",           // JOB_BABY_PRIEST
	4032: "Baby Wizard",           // JOB_BABY_WIZARD
	4033: "Baby Blacksmith",       // JOB_BABY_BLACKSMITH
	4034: "Baby Hunter",           // JOB_BABY_HUNTER
	4035: "Baby Assassin",         // JOB_BABY_ASSASSIN
	4036: "Baby Knight",           // JOB_BABY_KNIGHT2
	4037: "Baby Crusader",         // JOB_BABY_CRUSADER
	4038: "Baby Monk",             // JOB_BABY_MONK
	4039: "Baby Sage",             // JOB_BABY_SAGE
	4040: "Baby Rogue",            // JOB_BABY_ROGUE
	4041: "Baby Alchemist",        // JOB_BABY_ALCHEMIST
	4042: "Baby Bard",             // JOB_BABY_BARD
	4043: "Baby Dancer",           // JOB_BABY_DANCER
	4044: "Baby Crusader",         // JOB_BABY_CRUSADER2
	4045: "Super Baby",            // JOB_SUPER_BABY
	4046: "Taekwon",               // JOB_TAEKWON
	4047: "Star Gladiator",        // JOB_STAR_GLADIATOR
	4048: "Star Gladiator",        // JOB_STAR_GLADIATOR2
	4049: "Soul Linker",           // JOB_SOUL_LINKER
	4050: "Gangsi",                // JOB_GANGSI
	4051: "Death Knight",          // JOB_DEATH_KNIGHT
	4052: "Dark Collector",        // JOB_DARK_COLLECTOR
	4054: "Rune Knight",           // JOB_RUNE_KNIGHT
	4055: "Warlock",               // JOB_WARLOCK
	4056: "Ranger",                // JOB_RANGER
	4057: "Arch Bishop",           // JOB_ARCH_BISHOP
	4058: "Mechanic",              // JOB_MECHANIC
	4059: "Guillotine Cross",      // JOB_GUILLOTINE_CROSS
	4060: "Rune Knight T",         // JOB_RUNE_KNIGHT_T
	4061: "Warlock T",             // JOB_WARLOCK_T
	4062: "Ranger T",              // JOB_RANGER_T
	4063: "Arch Bishop T",         // JOB_ARCH_BISHOP_T
	4064: "Mechanic T",            // JOB_MECHANIC_T
	4065: "Guillotine Cross T",    // JOB_GUILLOTINE_CROSS_T
	4066: "Royal Guard",           // JOB_ROYAL_GUARD
	4067: "Sorcerer",              // JOB_SORCERER
	4068: "Minstrel",              // JOB_MINSTREL
	4069: "Wanderer",              // JOB_WANDERER
	4070: "Sura",                  // JOB_SURA
	4071: "Genetic",               // JOB_GENETIC
	4072: "Shadow Chaser",         // JOB_SHADOW_CHASER
	4073: "Royal Guard T",         // JOB_ROYAL_GUARD_T
	4074: "Sorcerer T",            // JOB_SORCERER_T
	4075: "Minstrel T",            // JOB_MINSTREL_T
	4076: "Wanderer T",            // JOB_WANDERER_T
	4077: "Sura T",                // JOB_SURA_T
	4078: "Genetic T",             // JOB_GENETIC_T
	4079: "Shadow Chaser T",       // JOB_SHADOW_CHASER_T
	4080: "Rune Knight",           // JOB_RUNE_KNIGHT2
	4081: "Rune Knight",           // JOB_RUNE_KNIGHT_T2
	4082: "Royal Guard",           // JOB_ROYAL_GUARD2
	4083: "Royal Guard",           // JOB_ROYAL_GUARD_T2
	4084: "Ranger",                // JOB_RANGER2
	4085: "Ranger",                // JOB_RANGER_T2
	4086: "Mechanic",              // JOB_MECHANIC2
	4087: "Mechanic",              // JOB_MECHANIC_T2
	4096: "Baby Rune Knight",      // JOB_BABY_RUNE_KNIGHT
	4097: "Baby Warlock",          // JOB_BABY_WARLOCK
	4098: "Baby Ranger",           // JOB_BABY_RANGER
	4099: "Baby Arch Bishop",      // JOB_BABY_ARCH_BISHOP
	4100: "Baby Mechanic",         // JOB_BABY_MECHANIC
	4101: "Baby Guillotine Cross", // JOB_BABY_GUILLOTINE_CROSS
	4102: "Baby Royal Guard",      // JOB_BABY_ROYAL_GUARD
	4103: "Baby Sorcerer",         // JOB_BABY_SORCERER
	4104: "Baby Minstrel",         // JOB_BABY_MINSTREL
	4105: "Baby Wanderer",         // JOB_BABY_WANDERER
	4106: "Baby Sura",             // JOB_BABY_SURA
	4107: "Baby Genetic",          // JOB_BABY_GENETIC
	4108: "Baby Shadow Chaser",    // JOB_BABY_SHADOW_CHASER
	4109: "Baby Rune Knight",      // JOB_BABY_RUNE_KNIGHT2
	4110: "Baby Royal Guard",      // JOB_BABY_ROYAL_GUARD2
	4111: "Baby Ranger",           // JOB_BABY_RANGER2
	4112: "Baby Mechanic",         // JOB_BABY_MECHANIC2
	4190: "Expanded Super Novice", // JOB_SUPER_NOVICE_E
	4191: "Expanded Super Baby",   // JOB_SUPER_BABY_E
	4211: "Kagerou",               // JOB_KAGEROU
	4212: "Oboro",                 // JOB_OBORO
	4215: "Rebellion",             // JOB_REBELLION
	4218: "Summoner",              // JOB_SUMMONER
	4220: "Baby Summoner",         // JOB_BABY_SUMMONER
	4222: "Baby Ninja",            // JOB_BABY_NINJA
	4223: "Baby Kagerou",          // JOB_BABY_KAGEROU
	4224: "Baby Oboro",            // JOB_BABY_OBORO
	4225: "Baby Taekwon",          // JOB_BABY_TAEKWON
	4226: "Baby Star Gladiator",   // JOB_BABY_STAR_GLADIATOR
	4227: "Baby Soul Linker",      // JOB_BABY_SOUL_LINKER
	4228: "Baby Gunslinger",       // JOB_BABY_GUNSLINGER
	4229: "Baby Rebellion",        // JOB_BABY_REBELLION
	4238: "Baby Star Gladiator",   // JOB_BABY_STAR_GLADIATOR2
	4239: "Star Emperor",          // JOB_STAR_EMPEROR
	4240: "Soul Reaper",           // JOB_SOUL_REAPER
	4241: "Baby Star Emperor",     // JOB_BABY_STAR_EMPEROR
	4242: "Baby Soul Reaper",      // JOB_BABY_SOUL_REAPER
	4243: "Star Emperor",          // JOB_STAR_EMPEROR2
	4244: "Baby Star Emperor",     // JOB_BABY_STAR_EMPEROR2
	4252: "Dragon Knight",         // JOB_DRAGON_KNIGHT
	4253: "Meister",               // JOB_MEISTER
	4254: "Shadow Cross",          // JOB_SHADOW_CROSS
	4255: "Arch Mage",             // JOB_ARCH_MAGE
	4256: "Cardinal",              // JOB_CARDINAL
	4257: "Windhawk",              // JOB_WINDHAWK
	4258: "Imperial Guard",        // JOB_IMPERIAL_GUARD
	4259: "Biolo",                 // JOB_BIOLO
	4260: "Abyss Chaser",          // JOB_ABYSS_CHASER
	4261: "Elemental Master",      // JOB_ELEMENTAL_MASTER
	4262: "Inquisitor",            // JOB_INQUISITOR
	4263: "Troubadour",            // JOB_TROUBADOUR
	4264: "Trouvere",              // JOB_TROUVERE
	4278: "Windhawk",              // JOB_WINDHAWK2
	4279: "Meister",               // JOB_MEISTER2
	4280: "Dragon Knight",         // JOB_DRAGON_KNIGHT2
	4281: "Imperial Guard",        // JOB_IMPERIAL_GUARD2
	4302: "Sky Emperor",           // JOB_SKY_EMPEROR
	4303: "Soul Ascetic",          // JOB_SOUL_ASCETIC
	4304: "Shinkiro",              // JOB_SHINKIRO
	4305: "Shiranui",              // JOB_SHIRANUI
	4306: "Night Watch",           // JOB_NIGHT_WATCH
	4307: "Hyper Novice",          // JOB_HYPER_NOVICE
	4308: "Spirit Handler",        // JOB_SPIRIT_HANDLER
	4316: "Sky Emperor",           // JOB_SKY_EMPEROR2
}
