package statuschange

// Status IDs that wAthena reads, as rAthena's sc_type. The values are
// the same in both modes. ids_test.go checks them against
// script_constants.json.
const (
	SCStone                = 0    // SC_STONE
	SCFreeze               = 1    // SC_FREEZE
	SCStun                 = 2    // SC_STUN
	SCSleep                = 3    // SC_SLEEP
	SCPoison               = 4    // SC_POISON
	SCCurse                = 5    // SC_CURSE
	SCSilence              = 6    // SC_SILENCE
	SCConfusion            = 7    // SC_CONFUSION
	SCBlind                = 8    // SC_BLIND
	SCBleeding             = 9    // SC_BLEEDING
	SCDpoison              = 10   // SC_DPOISON
	SCStonewait            = 11   // SC_STONEWAIT
	SCProvoke              = 20   // SC_PROVOKE
	SCEndure               = 21   // SC_ENDURE
	SCHiding               = 24   // SC_HIDING
	SCCloaking             = 25   // SC_CLOAKING
	SCAngelus              = 29   // SC_ANGELUS
	SCBlessing             = 30   // SC_BLESSING
	SCSignumcrucis         = 31   // SC_SIGNUMCRUCIS
	SCIncreaseagi          = 32   // SC_INCREASEAGI
	SCDecreaseagi          = 33   // SC_DECREASEAGI
	SCSlowpoison           = 34   // SC_SLOWPOISON
	SCImpositio            = 35   // SC_IMPOSITIO
	SCAspersio             = 37   // SC_ASPERSIO
	SCBenedictio           = 38   // SC_BENEDICTIO
	SCKyrie                = 39   // SC_KYRIE
	SCAeterna              = 42   // SC_AETERNA
	SCAdrenaline           = 43   // SC_ADRENALINE
	SCWeaponperfection     = 44   // SC_WEAPONPERFECTION
	SCOverthrust           = 45   // SC_OVERTHRUST
	SCMaximizepower        = 46   // SC_MAXIMIZEPOWER
	SCTrickdead            = 47   // SC_TRICKDEAD
	SCBrokenarmor          = 50   // SC_BROKENARMOR
	SCBrokenweapon         = 51   // SC_BROKENWEAPON
	SCHallucination        = 52   // SC_HALLUCINATION
	SCWeight50             = 53   // SC_WEIGHT50
	SCWeight90             = 54   // SC_WEIGHT90
	SCAspdpotion0          = 55   // SC_ASPDPOTION0
	SCAspdpotion1          = 56   // SC_ASPDPOTION1
	SCAspdpotion2          = 57   // SC_ASPDPOTION2
	SCAspdpotion3          = 58   // SC_ASPDPOTION3
	SCSpeedup0             = 59   // SC_SPEEDUP0
	SCSpeedup1             = 60   // SC_SPEEDUP1
	SCAtkpotion            = 61   // SC_ATKPOTION
	SCMatkpotion           = 62   // SC_MATKPOTION
	SCWedding              = 63   // SC_WEDDING
	SCSlowdown             = 64   // SC_SLOWDOWN
	SCAnkle                = 65   // SC_ANKLE
	SCSight                = 88   // SC_SIGHT
	SCRuwach               = 90   // SC_RUWACH
	SCFireweapon           = 96   // SC_FIREWEAPON
	SCWaterweapon          = 97   // SC_WATERWEAPON
	SCWindweapon           = 98   // SC_WINDWEAPON
	SCEarthweapon          = 99   // SC_EARTHWEAPON
	SCNochat               = 106  // SC_NOCHAT
	SCConcentration        = 110  // SC_CONCENTRATION
	SCBerserk              = 112  // SC_BERSERK
	SCAssumptio            = 115  // SC_ASSUMPTIO
	SCMagicpower           = 118  // SC_MAGICPOWER
	SCTruesight            = 120  // SC_TRUESIGHT
	SCWindwalk             = 121  // SC_WINDWALK
	SCChangeundead         = 128  // SC_CHANGEUNDEAD
	SCJointbeat            = 129  // SC_JOINTBEAT
	SCOrcish               = 137  // SC_ORCISH
	SCReadystorm           = 138  // SC_READYSTORM
	SCReadydown            = 139  // SC_READYDOWN
	SCReadyturn            = 140  // SC_READYTURN
	SCReadycounter         = 141  // SC_READYCOUNTER
	SCDodge                = 142  // SC_DODGE
	SCShadowweapon         = 144  // SC_SHADOWWEAPON
	SCAdrenaline2          = 145  // SC_ADRENALINE2
	SCGhostweapon          = 146  // SC_GHOSTWEAPON
	SCSightblaster         = 160  // SC_SIGHTBLASTER
	SCWinkcharm            = 161  // SC_WINKCHARM
	SCDancing              = 164  // SC_DANCING
	SCElementalchange      = 165  // SC_ELEMENTALCHANGE
	SCSiegfried            = 172  // SC_SIEGFRIED
	SCModechange           = 177  // SC_MODECHANGE
	SCStop                 = 182  // SC_STOP
	SCSpirit               = 184  // SC_SPIRIT
	SCIntravision          = 186  // SC_INTRAVISION
	SCIncallstatus         = 187  // SC_INCALLSTATUS
	SCInchitrate           = 195  // SC_INCHITRATE
	SCIncflee              = 196  // SC_INCFLEE
	SCIncmhprate           = 198  // SC_INCMHPRATE
	SCIncmsprate           = 199  // SC_INCMSPRATE
	SCIncatkrate           = 200  // SC_INCATKRATE
	SCIncmatkrate          = 201  // SC_INCMATKRATE
	SCStrfood              = 203  // SC_STRFOOD
	SCAgifood              = 204  // SC_AGIFOOD
	SCVitfood              = 205  // SC_VITFOOD
	SCIntfood              = 206  // SC_INTFOOD
	SCDexfood              = 207  // SC_DEXFOOD
	SCLukfood              = 208  // SC_LUKFOOD
	SCScresist             = 214  // SC_SCRESIST
	SCXmas                 = 215  // SC_XMAS
	SCIncflee2             = 248  // SC_INCFLEE2
	SCJailed               = 249  // SC_JAILED
	SCCriticalwound        = 253  // SC_CRITICALWOUND
	SCSlowcast             = 255  // SC_SLOWCAST
	SCSummer               = 256  // SC_SUMMER
	SCExpboost             = 257  // SC_EXPBOOST
	SCItemboost            = 258  // SC_ITEMBOOST
	SCBossmapinfo          = 259  // SC_BOSSMAPINFO
	SCLifeinsurance        = 260  // SC_LIFEINSURANCE
	SCInccri               = 261  // SC_INCCRI
	SCMdefRate             = 265  // SC_MDEF_RATE
	SCInchealrate          = 267  // SC_INCHEALRATE
	SCDefRate              = 275  // SC_DEF_RATE
	SCItemscript           = 289  // SC_ITEMSCRIPT
	SCJexpboost            = 292  // SC_JEXPBOOST
	SCFoodStrCash          = 303  // SC_FOOD_STR_CASH
	SCFoodAgiCash          = 304  // SC_FOOD_AGI_CASH
	SCFoodVitCash          = 305  // SC_FOOD_VIT_CASH
	SCFoodDexCash          = 306  // SC_FOOD_DEX_CASH
	SCFoodIntCash          = 307  // SC_FOOD_INT_CASH
	SCFoodLukCash          = 308  // SC_FOOD_LUK_CASH
	SCFear                 = 309  // SC_FEAR
	SCBurning              = 310  // SC_BURNING
	SCFreezing             = 311  // SC_FREEZING
	SCRefresh              = 316  // SC_REFRESH
	SCWhiteimprison        = 333  // SC_WHITEIMPRISON
	SCMarshofabyss         = 334  // SC_MARSHOFABYSS
	SCRecognizedspell      = 335  // SC_RECOGNIZEDSPELL
	SCStasis               = 336  // SC_STASIS
	SCElectricshocker      = 345  // SC_ELECTRICSHOCKER
	SCBite                 = 347  // SC_BITE
	SCHovering             = 350  // SC_HOVERING
	SCWeaponblocking       = 363  // SC_WEAPONBLOCKING
	SCToxin                = 368  // SC_TOXIN
	SCParalyse             = 369  // SC_PARALYSE
	SCVenombleed           = 370  // SC_VENOMBLEED
	SCMagicmushroom        = 371  // SC_MAGICMUSHROOM
	SCDeathhurt            = 372  // SC_DEATHHURT
	SCPyrexia              = 373  // SC_PYREXIA
	SCOblivioncurse        = 374  // SC_OBLIVIONCURSE
	SCLeechesend           = 375  // SC_LEECHESEND
	SCShieldspellHp        = 378  // SC_SHIELDSPELL_HP
	SCShieldspellSp        = 379  // SC_SHIELDSPELL_SP
	SCCrystalize           = 388  // SC_CRYSTALIZE
	SCVacuumExtreme        = 391  // SC_VACUUM_EXTREME
	SCHarmonize            = 398  // SC_HARMONIZE
	SCVoiceofsiren         = 399  // SC_VOICEOFSIREN
	SCDeepsleep            = 400  // SC_DEEPSLEEP
	SCSaturdaynightfever   = 406  // SC_SATURDAYNIGHTFEVER
	SCNetherworld          = 412  // SC_NETHERWORLD
	SCStomachache          = 427  // SC_STOMACHACHE
	SCPushCart             = 518  // SC_PUSH_CART
	SCKyougaku             = 531  // SC_KYOUGAKU
	SCParalysis            = 553  // SC_PARALYSIS
	SCHanbok               = 555  // SC_HANBOK
	SCRebound              = 560  // SC_REBOUND
	SCUnlimit              = 561  // SC_UNLIMIT
	SCTelekinesisIntense   = 563  // SC_TELEKINESIS_INTENSE
	SCBTrap                = 578  // SC_B_TRAP
	SCOktoberfest          = 586  // SC_OKTOBERFEST
	SCChasewalk2           = 596  // SC_CHASEWALK2
	SCPromoteHealthReserch = 625  // SC_PROMOTE_HEALTH_RESERCH
	SCEnergyDrinkReserch   = 626  // SC_ENERGY_DRINK_RESERCH
	SCNorecoverState       = 627  // SC_NORECOVER_STATE
	SCSpritemable          = 630  // SC_SPRITEMABLE
	SCTunaparty            = 635  // SC_TUNAPARTY
	SCGvgGiant             = 649  // SC_GVG_GIANT
	SCGvgGolem             = 650  // SC_GVG_GOLEM
	SCGvgStun              = 651  // SC_GVG_STUN
	SCGvgStone             = 652  // SC_GVG_STONE
	SCGvgFreez             = 653  // SC_GVG_FREEZ
	SCGvgSleep             = 654  // SC_GVG_SLEEP
	SCGvgCurse             = 655  // SC_GVG_CURSE
	SCGvgSilence           = 656  // SC_GVG_SILENCE
	SCGvgBlind             = 657  // SC_GVG_BLIND
	SCClanInfo             = 658  // SC_CLAN_INFO
	SCDailysendmailcnt     = 672  // SC_DAILYSENDMAILCNT
	SCDressup              = 686  // SC_DRESSUP
	SCAncilla              = 698  // SC_ANCILLA
	SCGravitycontrol       = 721  // SC_GRAVITYCONTROL
	SCHellsPlant           = 735  // SC_HELLS_PLANT
	SCIncreaseMaxhp        = 736  // SC_INCREASE_MAXHP
	SCIncreaseMaxsp        = 737  // SC_INCREASE_MAXSP
	SCMadogear             = 757  // SC_MADOGEAR
	SCSoulattack           = 769  // SC_SOULATTACK
	SCGradualGravity       = 913  // SC_GRADUAL_GRAVITY
	SCKillingAura          = 915  // SC_KILLING_AURA
	SCPeriodReceiveitem2nd = 948  // SC_PERIOD_RECEIVEITEM_2ND
	SCPeriodPlusexp2nd     = 949  // SC_PERIOD_PLUSEXP_2ND
	SCStarBurst            = 1018 // SC_STAR_BURST
)

// Status IDs that skills start or read, as rAthena's sc_type.
const (
	SCConcentrate    = 23  // SC_CONCENTRATE
	SCQuagmire       = 28  // SC_QUAGMIRE
	SCGravitation    = 155 // SC_GRAVITATION
	SCSuffragium     = 36  // SC_SUFFRAGIUM
	SCMagnificat     = 40  // SC_MAGNIFICAT
	SCGloria         = 41  // SC_GLORIA
	SCBasilica       = 116 // SC_BASILICA
	SCLoud           = 48  // SC_LOUD
	SCEnergycoat     = 49  // SC_ENERGYCOAT
	SCAutoberserk    = 85  // SC_AUTOBERSERK
	SCSafetywall     = 89  // SC_SAFETYWALL
	SCWatkElement    = 103 // SC_WATK_ELEMENT
	SCMemorize       = 131 // SC_MEMORIZE
	SCPoembragi      = 175 // SC_POEMBRAGI
	SCPneuma         = 268 // SC_PNEUMA
	SCCloakingexceed = 364 // SC_CLOAKINGEXCEED
	SCSubWeaponPro   = 885 // SC_SUB_WEAPONPROPERTY
	SCStripweapon    = 68  // SC_STRIPWEAPON
	SCStripshield    = 69  // SC_STRIPSHIELD
	SCStriparmor     = 70  // SC_STRIPARMOR
	SCStriphelm      = 71  // SC_STRIPHELM
	SCChasewalk      = 124 // SC_CHASEWALK
	SCRejectsword    = 125 // SC_REJECTSWORD
	SCPreserve       = 151 // SC_PRESERVE
	SCCloseconfine   = 162 // SC_CLOSECONFINE
	SCCloseconfine2  = 163 // SC_CLOSECONFINE2
	SCRaid           = 513 // SC_RAID
)

// Status IDs of the Knight and Lord Knight skills, as rAthena's sc_type.
const (
	SCTwohandquicken = 22  // SC_TWOHANDQUICKEN
	SCAutocounter    = 87  // SC_AUTOCOUNTER
	SCAurablade      = 108 // SC_AURABLADE
	SCParrying       = 109 // SC_PARRYING
	SCTensionrelax   = 111 // SC_TENSIONRELAX
	SCOnehand        = 150 // SC_ONEHAND
	SCBattleOrders   = 152 // SC_BATTLEORDERS
	SCRegeneration   = 153 // SC_REGENERATION
)

// Joint Beat's breaks, as rAthena's e_joint_break: SC_JOINTBEAT's val2
// holds them as bits.
const (
	BreakAnkle    = 0x01 // BREAK_ANKLE
	BreakWrist    = 0x02 // BREAK_WRIST
	BreakKnee     = 0x04 // BREAK_KNEE
	BreakShoulder = 0x08 // BREAK_SHOULDER
	BreakWaist    = 0x10 // BREAK_WAIST
	BreakNeck     = 0x20 // BREAK_NECK
	BreakFlags    = 0x3F // BREAK_FLAGS
)

// Status IDs of the Crusader and Paladin skills, as rAthena's sc_type.
const (
	SCAutoguard     = 76  // SC_AUTOGUARD
	SCReflectshield = 77  // SC_REFLECTSHIELD
	SCProvidence    = 79  // SC_PROVIDENCE
	SCDefender      = 80  // SC_DEFENDER
	SCSpearquicken  = 86  // SC_SPEARQUICKEN
	SCGospel        = 114 // SC_GOSPEL
	SCDevotion      = 134 // SC_DEVOTION
	SCSacrifice     = 135 // SC_SACRIFICE
	SCShrink        = 159 // SC_SHRINK
	SCInchit        = 194 // SC_INCHIT
	SCIncfleerate   = 197 // SC_INCFLEERATE
	SCIncdefrate    = 202 // SC_INCDEFRATE
)

// Battle check flags that SC_GOSPEL's val4 holds, as rAthena's BCT_SELF
// and BCT_ENEMY: the caster's own Battle Chant, or its effect on an
// enemy.
const (
	BCTSelf  = 0x010000
	BCTEnemy = 0x020000
)

// Status IDs of the Assassin and Assassin Cross skills, as rAthena's
// sc_type.
const (
	SCEncpoison   = 26  // SC_ENCPOISON
	SCPoisonreact = 27  // SC_POISONREACT
	SCSplasher    = 78  // SC_SPLASHER
	SCEdp         = 119 // SC_EDP
)

// Status IDs of the Monk and Champion skills, as rAthena's sc_type.
const (
	SCExtremityfist     = 91  // SC_EXTREMITYFIST
	SCExplosionspirits  = 92  // SC_EXPLOSIONSPIRITS
	SCCombo             = 93  // SC_COMBO
	SCBladestopWait     = 94  // SC_BLADESTOP_WAIT
	SCBladestop         = 95  // SC_BLADESTOP
	SCFogwall           = 132 // SC_FOGWALL
	SCSteelbody         = 136 // SC_STEELBODY
	SCSkillrateUp       = 221 // SC_SKILLRATE_UP
	SCCursedcircleAtker = 414 // SC_CURSEDCIRCLE_ATKER
	SCRaisingdragon     = 417 // SC_RAISINGDRAGON
	SCGtEnergygain      = 418 // SC_GT_ENERGYGAIN
)

// Status IDs of the Sage and Professor skills, as rAthena's sc_type.
const (
	SCMagicrod    = 81  // SC_MAGICROD
	SCAutospell   = 83  // SC_AUTOSPELL
	SCVolcano     = 100 // SC_VOLCANO
	SCDeluge      = 101 // SC_DELUGE
	SCViolentgale = 102 // SC_VIOLENTGALE
	SCMindbreaker = 130 // SC_MINDBREAKER
	SCSpiderweb   = 133 // SC_SPIDERWEB
	SCDoublecast  = 154 // SC_DOUBLECAST
	SCSuiton      = 236 // SC_SUITON
)

// Status IDs of the Blacksmith and Whitesmith skills, as rAthena's
// sc_type.
const (
	SCMeltdown      = 122 // SC_MELTDOWN
	SCCartboost     = 123 // SC_CARTBOOST
	SCMaxoverthrust = 156 // SC_MAXOVERTHRUST
)

// Status IDs of the Bard, Dancer, Clown and Gypsy skills, as rAthena's
// sc_type.
const (
	SCComa            = 185 // SC_COMA
	SCMarionette      = 126 // SC_MARIONETTE
	SCMarionette2     = 127 // SC_MARIONETTE2
	SCLonging         = 157 // SC_LONGING
	SCHermode         = 158 // SC_HERMODE
	SCRichmankim      = 166 // SC_RICHMANKIM
	SCEternalchaos    = 167 // SC_ETERNALCHAOS
	SCDrumbattle      = 168 // SC_DRUMBATTLE
	SCNibelungen      = 169 // SC_NIBELUNGEN
	SCRokisweil       = 170 // SC_ROKISWEIL
	SCIntoabyss       = 171 // SC_INTOABYSS
	SCWhistle         = 173 // SC_WHISTLE
	SCAssncros        = 174 // SC_ASSNCROS
	SCAppleidun       = 176 // SC_APPLEIDUN
	SCHumming         = 178 // SC_HUMMING
	SCDontforgetme    = 179 // SC_DONTFORGETME
	SCFortune         = 180 // SC_FORTUNE
	SCService4u       = 181 // SC_SERVICE4U
	SCTarotcard       = 664 // SC_TAROTCARD
	SCAdaptation      = 702 // SC_ADAPTATION
	SCEnsemblefatigue = 745 // SC_ENSEMBLEFATIGUE
)

// Status IDs of monster skills, as rAthena's sc_type.
const (
	SCKeeping    = 66  // SC_KEEPING
	SCBarrier    = 67  // SC_BARRIER
	SCRebirth    = 284 // SC_REBIRTH
	SCInvincible = 295 // SC_INVINCIBLE
)
