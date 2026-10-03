package mod

// CompanionKind is data/companions.yml and MountKind data/mounts.yml
// (proposal 0002, review decision 25). Core's own files add the core
// records with plain names, such as falcon and peco_peco, which mods
// change by name. server/gamedata reads the fields.
var (
	CompanionKind = Kind{Name: "companions", File: "data/companions.yml", Fixed: []string{"id"}, CoreNames: true}
	MountKind     = Kind{Name: "mounts", File: "data/mounts.yml", Fixed: []string{"id"}, CoreNames: true}
)
