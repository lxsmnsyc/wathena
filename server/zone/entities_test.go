package zone

import "testing"

func TestEntitySet(t *testing.T) {
	var s entitySet
	a, b, c := &entity{}, &entity{}, &entity{}
	for _, e := range []*entity{a, b, c} {
		s.add(e)
	}
	// Removing during a walk skips the removed entities not reached yet.
	var seen []*entity
	for e := range s.all() {
		seen = append(seen, e)
		if e == a {
			s.remove(b)
		}
	}
	if len(seen) != 2 || seen[1] != c || s.has(b) || s.len() != 2 {
		t.Fatalf("walked %v", seen)
	}
	// b keeps its slot until compact, then a new entity reuses it.
	slot := b.slot
	s.compact()
	d := &entity{}
	s.add(d)
	if d.slot != slot || !s.has(d) || !s.has(a) || !s.has(c) || s.len() != 3 {
		t.Fatalf("slot %d, want %d; set %v", d.slot, slot, s.list)
	}
}

func TestNetIDs(t *testing.T) {
	var s entitySet
	a, b, c := &entity{}, &entity{}, &entity{}
	for _, e := range []*entity{a, b, c} {
		s.add(e)
	}
	var n netIDs
	n.set(a, 1)
	n.set(b, 2)
	n.set(c, 3)
	n.del(a)
	if _, ok := n.get(a); ok || n.id(b) != 2 || n.id(c) != 3 || len(n.known) != 2 {
		t.Fatalf("after del: %v", n.known)
	}
	// An entry left for a removed entity does not match the entity that
	// reuses its slot, and gives way to it.
	s.remove(b)
	s.compact()
	d := &entity{}
	s.add(d)
	if _, ok := n.get(d); ok {
		t.Fatal("the new entity matched the old entry")
	}
	n.set(d, 4)
	got := map[*entity]uint32{}
	for e, id := range n.all() {
		got[e] = id
	}
	if len(got) != 2 || got[c] != 3 || got[d] != 4 {
		t.Errorf("known %v", got)
	}
}
