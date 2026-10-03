package social

// Item share, as party_share_loot. The zone gives the item; these pick
// the member in rAthena's order, so a test can replay a recording.

// Item share types, as party_item_share_type's bits.
const (
	// ShareFirstOnly shares only items that have a first looter.
	ShareFirstOnly = 1
	// ShareRoundRobin gives items in turn instead of at random.
	ShareRoundRobin = 2
)

// SharesLoot reports whether party_share_loot gives an item to a
// member: the party shares items evenly, and with ShareFirstOnly the
// item has a first looter.
func SharesLoot(o Options, shareType int, hasFirst bool) bool {
	return o.ItemEven && (hasFirst || shareType&ShareFirstOnly == 0)
}

// RoundRobin is the round robin pick: from the slot after itemc, every
// slot once, wrapping at size, until give takes the item. eligible
// reports whether a slot holds a member on the map who is alive. It
// returns the slot that took the item, and false when none did.
func RoundRobin(itemc, size int, eligible func(slot int) bool, give func(slot int) bool) (int, bool) {
	if size <= 0 {
		return 0, false
	}
	i := itemc
	for {
		i++
		if i >= size {
			i = 0
		}
		if eligible(i) && give(i) {
			return i, true
		}
		if i == itemc || itemc >= size && i == size-1 {
			return 0, false
		}
	}
}

// RandomPick is the random pick: draw(n) returns 0 to n-1, as
// rnd_value(0, count-1). A member whose give fails is swapped with the
// last and dropped. candidates are slots in slot order. It returns the
// slot that took the item, and false when none did.
func RandomPick(candidates []int, draw func(n int) int, give func(slot int) bool) (int, bool) {
	c := append([]int(nil), candidates...)
	for len(c) > 0 {
		i := draw(len(c))
		if give(c[i]) {
			return c[i], true
		}
		c[i] = c[len(c)-1]
		c = c[:len(c)-1]
	}
	return 0, false
}
