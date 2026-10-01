package zone

import "iter"

// entitySet holds a zone's entities in a slice, so a tick walks them
// without a map. Each entity in the set also has a slot: a small number
// that players' netIDs are indexed by.
//
// remove leaves a nil in the slice, so removing while walking the set is
// safe. compact drops the nils and frees the slots of removed entities.
// The zone calls it at the start of a tick, after every player saw the
// removals of the tick before.
type entitySet struct {
	list  []*entity
	count int
	// free are slots to reuse. freed are the slots of entities removed
	// since the last compact.
	free  []int32
	freed []int32
	next  int32
}

// has reports whether e is in the set.
func (s *entitySet) has(e *entity) bool {
	i := e.idx
	return i >= 0 && int(i) < len(s.list) && s.list[i] == e
}

// add puts e in the set with a new slot. An entity is added once: a
// removed entity is never added again, since players may still hold its
// old slot.
func (s *entitySet) add(e *entity) {
	if s.has(e) {
		return
	}
	if n := len(s.free); n > 0 {
		e.slot = s.free[n-1]
		s.free = s.free[:n-1]
	} else {
		s.next++
		e.slot = s.next
	}
	e.idx = int32(len(s.list)) // #nosec G115 -- far fewer entities than 2^31.
	s.list = append(s.list, e)
	s.count++
}

// remove takes e out of the set. e keeps its slot until compact, so
// players still find their ID for it this tick.
func (s *entitySet) remove(e *entity) {
	if !s.has(e) {
		return
	}
	s.list[e.idx] = nil
	e.idx = -1
	s.count--
	s.freed = append(s.freed, e.slot)
}

// compact drops removed entities from the slice and frees their slots.
// It must not run while the set is walked.
func (s *entitySet) compact() {
	kept := s.list[:0]
	for _, e := range s.list {
		if e != nil {
			e.idx = int32(len(kept)) // #nosec G115 -- as in add.
			kept = append(kept, e)
		}
	}
	clear(s.list[len(kept):])
	s.list = kept
	s.free = append(s.free, s.freed...)
	s.freed = s.freed[:0]
}

// len is the number of entities in the set.
func (s *entitySet) len() int { return s.count }

// all walks the set. Entities removed during the walk and not reached
// yet are skipped. Entities added during the walk are reached.
func (s *entitySet) all() iter.Seq[*entity] {
	return func(yield func(*entity) bool) {
		for i := 0; i < len(s.list); i++ {
			if e := s.list[i]; e != nil && !yield(e) {
				return
			}
		}
	}
}

// netIDs are one connection's IDs for the entities it knows, indexed by
// slot. Each entry keeps its entity, so an entry left from a removed
// entity never matches the entity that reuses its slot. known lists the
// same entities densely, so a tick can walk them quickly.
type netIDs struct {
	bySlot []netEntry
	known  []*entity
}

type netEntry struct {
	e  *entity
	id uint32
	// pos is the entity's index in known.
	pos int32
}

// get returns the ID for e, and whether the connection knows e.
func (n *netIDs) get(e *entity) (uint32, bool) {
	if i := int(e.slot); i < len(n.bySlot) {
		if en := n.bySlot[i]; en.e == e {
			return en.id, true
		}
	}
	return 0, false
}

// id returns the ID for e, or 0 when the connection does not know e.
func (n *netIDs) id(e *entity) uint32 {
	id, _ := n.get(e)
	return id
}

// set records the ID for e.
func (n *netIDs) set(e *entity, id uint32) {
	i := int(e.slot)
	if i >= len(n.bySlot) {
		n.bySlot = append(n.bySlot, make([]netEntry, i+1-len(n.bySlot))...)
	}
	en := &n.bySlot[i]
	switch en.e {
	case e:
		en.id = id
		return
	case nil:
	default:
		// An entry left from a removed entity whose slot e reuses.
		n.del(en.e)
	}
	*en = netEntry{e: e, id: id, pos: int32(len(n.known))} // #nosec G115 -- far fewer entities than 2^31.
	n.known = append(n.known, e)
}

// del forgets e.
func (n *netIDs) del(e *entity) {
	i := int(e.slot)
	if i >= len(n.bySlot) || n.bySlot[i].e != e {
		return
	}
	pos := n.bySlot[i].pos
	last := len(n.known) - 1
	moved := n.known[last]
	n.known[pos] = moved
	n.bySlot[moved.slot].pos = pos
	n.known[last] = nil
	n.known = n.known[:last]
	n.bySlot[i] = netEntry{}
}

// all walks the known entities and their IDs. Forgetting an entity
// during the walk is not allowed.
func (n *netIDs) all() iter.Seq2[*entity, uint32] {
	return func(yield func(*entity, uint32) bool) {
		for _, e := range n.known {
			if !yield(e, n.bySlot[e.slot].id) {
				return
			}
		}
	}
}

// blockSize is the side of a block of cells in blockIndex, as rAthena's
// BLOCK_SIZE.
const blockSize = 8

// blockIndex finds the entities near a cell. The zone builds it again
// at the start of each tick's updates, while no entity moves, so no
// code that moves an entity has to keep it current.
type blockIndex struct {
	w, h   int
	blocks [][]*entity
	// used are the blocks that hold entities.
	used []int32
}

// build puts every entity of the set into its block.
func (b *blockIndex) build(width, height int, s *entitySet) {
	if b.blocks == nil {
		b.w, b.h = max(1, (width+blockSize-1)/blockSize), max(1, (height+blockSize-1)/blockSize)
		b.blocks = make([][]*entity, b.w*b.h)
	}
	for _, i := range b.used {
		clear(b.blocks[i])
		b.blocks[i] = b.blocks[i][:0]
	}
	b.used = b.used[:0]
	for _, e := range s.list {
		if e == nil {
			continue
		}
		i := b.block(e.x/blockSize, e.y/blockSize)
		if len(b.blocks[i]) == 0 {
			b.used = append(b.used, int32(i)) // #nosec G115 -- a block of a map at most 1024 cells wide.
		}
		b.blocks[i] = append(b.blocks[i], e)
	}
}

// block is the index of a block, with block coordinates clamped to the
// map.
func (b *blockIndex) block(bx, by int) int {
	return min(max(by, 0), b.h-1)*b.w + min(max(bx, 0), b.w-1)
}

// span is the range of block coordinates that cells c-r to c+r touch,
// clamped to n blocks.
func span(c, r, n int) (int, int) {
	lo, hi := c-r, c+r
	if lo < 0 {
		lo = 0
	}
	return min(lo/blockSize, n-1), min(max(hi, 0)/blockSize, n-1)
}
