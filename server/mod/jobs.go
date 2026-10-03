package mod

// JobKind is the job overlay: data/jobs.yml (proposal 0002, review
// decision 24). A record's name is a job's name in wa.jobs, such as
// knight, or a mod's full name, such as example.jobs:Desperado.
// server/gamedata reads the fields.
var JobKind = Kind{Name: "jobs", File: "data/jobs.yml", Fixed: []string{"id"}}

// Jobs merges the mods' job overlays. core reports whether core has a
// job of a name.
func (s *Set) Jobs(core func(name string) bool) (*Overlay, error) {
	return s.Overlay(JobKind, core)
}
