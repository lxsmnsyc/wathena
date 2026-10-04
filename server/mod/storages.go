package mod

// StorageKind is data/storages.yml (proposal 0002, review decision 29):
// extra storages, as rAthena's premium storages of inter_server.yml. A
// record has display_name, slots and owner (account or character).
// Core has no records: the account's storage is not a record.
// server/gamedata reads the fields.
var StorageKind = Kind{Name: "storages", File: "data/storages.yml", Fixed: []string{"id"}, CoreNames: true, LocalNames: true}
