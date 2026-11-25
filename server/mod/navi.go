package mod

// NaviKind is data/navi.yml (proposal 0006, section 5): the extra edges
// of the client's navigation, which the warps do not give: airship,
// Kafra and scroll trips. Core ships none. A record has from, to, x, y,
// to_x, to_y and service. server/gamedata reads the fields.
var NaviKind = Kind{Name: "navigation edges", File: "data/navi.yml", CoreNames: true, LocalNames: true}
