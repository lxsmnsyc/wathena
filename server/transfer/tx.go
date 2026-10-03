package transfer

import (
	"fmt"
	"slices"
	"time"

	"github.com/lxsmnsyc/wathena/server/item"
)

// Reason is why a Tx failed.
type Reason int

// Reasons.
const (
	// ReasonNotOwner is a slot that does not hold the serial the move
	// names, or an empty slot.
	ReasonNotOwner Reason = iota + 1
	// ReasonAmount is an amount below 1 or above what the slot holds.
	ReasonAmount
	// ReasonOverweight, ReasonFull, ReasonOverAmount and ReasonStackLimit
	// are the box's limits, as item.AddResult.
	ReasonOverweight
	ReasonFull
	ReasonOverAmount
	ReasonStackLimit
	// ReasonInvalid is an item the box cannot take, such as ID 0.
	ReasonInvalid
	// ReasonZeny is a wallet with too little zeny, a negative amount, or
	// zeny that would pass MaxZeny when the move may not drop it.
	ReasonZeny
)

var reasonNames = map[Reason]string{
	ReasonNotOwner:   "the slot does not hold that item",
	ReasonAmount:     "the amount is not valid",
	ReasonOverweight: "too heavy",
	ReasonFull:       "no free slot",
	ReasonOverAmount: "the stack would pass its limit",
	ReasonStackLimit: "the amount passes the item's stack limit",
	ReasonInvalid:    "not a valid item",
	ReasonZeny:       "not a valid zeny amount",
}

// Error is a failed Tx. Nothing changed.
type Error struct {
	// Op is the index of the move that failed, in the order of the Tx.
	Op     int
	Reason Reason
}

func (e *Error) Error() string {
	return fmt.Sprintf("transfer: move %d: %s", e.Op, reasonNames[e.Reason])
}

// AddResult is the reason as pc_additem's result, for the messages that
// show it, such as PickUpFailed.
func (e *Error) AddResult() item.AddResult {
	switch e.Reason {
	case ReasonOverweight:
		return item.AddOverweight
	case ReasonFull:
		return item.AddOverItem
	case ReasonOverAmount:
		return item.AddOverAmount
	case ReasonStackLimit:
		return item.AddStackLimit
	}
	return item.AddInvalid
}

func addReason(r item.AddResult) Reason {
	switch r {
	case item.AddOverweight:
		return ReasonOverweight
	case item.AddOverItem:
		return ReasonFull
	case item.AddOverAmount:
		return ReasonOverAmount
	case item.AddStackLimit:
		return ReasonStackLimit
	}
	return ReasonInvalid
}

type opKind int

const (
	opMove opKind = iota
	opCreate
	opDestroy
	opZeny
)

type op struct {
	kind     opKind
	from, to Box
	slot     int
	serial   item.Serial
	amount   int
	// copy is what Create puts, and end the end it comes from or goes to.
	copy item.Slot
	end  Owner
	// source is the serial a Create's copy came from, or zero.
	source item.Serial
	// Zeny moves: from or to may be nil for an end. drop lets zeny over
	// MaxZeny be lost, as pc_getzeny.
	wfrom, wto Wallet
	zeny       int64
	drop       bool
}

// Tx is a list of moves that happen together or not at all.
type Tx struct {
	Action Action
	// Now is the time the log lines get. Apply uses time.Now when it is
	// zero.
	Now time.Time
	ops []op
}

// New starts a Tx.
func New(a Action) *Tx { return &Tx{Action: a} }

// Move moves an amount from a slot of one box to another. The slot must
// hold serial.
func (t *Tx) Move(from Box, slot int, serial item.Serial, amount int, to Box) *Tx {
	t.ops = append(t.ops, op{kind: opMove, from: from, to: to, slot: slot, serial: serial, amount: amount})
	return t
}

// Create puts an amount of a copy in a box, from an end. source is the
// serial the copy came from, such as the stack a ground item was dropped
// from, or zero.
func (t *Tx) Create(from Owner, copy item.Slot, amount int, to Box, source item.Serial) *Tx {
	copy.Amount, copy.Serial, copy.Equip = 0, item.Serial{}, 0
	t.ops = append(t.ops, op{kind: opCreate, end: from, copy: copy, amount: amount, to: to, source: source})
	return t
}

// Destroy takes an amount from a slot of a box to an end. The slot must
// hold serial.
func (t *Tx) Destroy(from Box, slot int, serial item.Serial, amount int, to Owner) *Tx {
	t.ops = append(t.ops, op{kind: opDestroy, from: from, slot: slot, serial: serial, amount: amount, end: to})
	return t
}

// Pay moves zeny from a wallet to an end. The wallet must hold it.
func (t *Tx) Pay(from Wallet, amount int64, to Owner) *Tx {
	t.ops = append(t.ops, op{kind: opZeny, wfrom: from, end: to, zeny: amount})
	return t
}

// Earn moves zeny from an end to a wallet. With drop, zeny over MaxZeny
// is lost, as pc_getzeny. Without it, the move fails.
func (t *Tx) Earn(from Owner, amount int64, to Wallet, drop bool) *Tx {
	t.ops = append(t.ops, op{kind: opZeny, wto: to, end: from, zeny: amount, drop: drop})
	return t
}

// MoveZeny moves zeny between two wallets. It fails when the receiver
// would pass MaxZeny.
func (t *Tx) MoveZeny(from Wallet, amount int64, to Wallet) *Tx {
	t.ops = append(t.ops, op{kind: opZeny, wfrom: from, wto: to, zeny: amount})
	return t
}

// Len is the number of moves.
func (t *Tx) Len() int { return len(t.ops) }

// Change is one change a move made, in the order of the moves.
type Change struct {
	// Box and Slot are a slot that got (Amount above 0) or lost (below
	// 0) items. Wallet and Zeny are a wallet whose zeny changed by Zeny.
	Box    Box
	Slot   int
	Amount int
	Wallet Wallet
	Zeny   int64
}

// Apply checks every move on copies, then makes them. each, when not
// nil, sees each change as it is made, so the caller can tell players
// in the order of the moves. On an error nothing changed.
func (t *Tx) Apply(each func(Change)) (Logs, error) {
	if err := t.check(); err != nil {
		return Logs{}, err
	}
	now := t.Now
	if now.IsZero() {
		now = time.Now()
	}
	var logs Logs
	for i := range t.ops {
		o := &t.ops[i]
		if err := t.run(o, now, &logs, each); err != nil {
			// The checks passed on identical copies, so this is a bug in
			// a box. Earlier moves stay made.
			panic(fmt.Sprintf("transfer: move %d failed after its checks passed: %v", i, err))
		}
	}
	return logs, nil
}

// check runs every move on copies of the boxes and wallets.
func (t *Tx) check() error {
	boxes := map[Box]Box{}
	wallets := map[Wallet]Wallet{}
	box := func(b Box) Box {
		if b == nil {
			return nil
		}
		c, ok := boxes[b]
		if !ok {
			c = b.Clone()
			boxes[b] = c
		}
		return c
	}
	wallet := func(w Wallet) Wallet {
		if w == nil {
			return nil
		}
		c, ok := wallets[w]
		if !ok {
			c = w.Clone()
			wallets[w] = c
		}
		return c
	}
	for i, o := range t.ops {
		c := o
		c.from, c.to, c.wfrom, c.wto = box(o.from), box(o.to), wallet(o.wfrom), wallet(o.wto)
		if r := t.step(&c, time.Time{}, nil, nil); r != 0 {
			return &Error{Op: i, Reason: r}
		}
	}
	return nil
}

func (t *Tx) run(o *op, now time.Time, logs *Logs, each func(Change)) error {
	if r := t.step(o, now, logs, each); r != 0 {
		return &Error{Reason: r}
	}
	return nil
}

// step makes one move. logs is nil while checking: serials and log lines
// are only made for the real moves.
func (t *Tx) step(o *op, now time.Time, logs *Logs, each func(Change)) Reason {
	real := logs != nil
	emit := func(c Change) {
		if each != nil {
			each(c)
		}
	}
	switch o.kind {
	case opZeny:
		return t.zeny(o, now, logs, emit)
	case opCreate:
		if o.amount <= 0 {
			return ReasonAmount
		}
		slot, r := o.to.Put(&o.copy, o.amount)
		if r != item.AddSuccess {
			return addReason(r)
		}
		if !real {
			return 0
		}
		s := o.to.Slot(slot)
		if s.Serial.IsZero() {
			s.Serial = item.NewSerial()
		}
		emit(Change{Box: o.to, Slot: slot, Amount: o.amount})
		logs.Items = append(logs.Items, t.line(now, o.end, o.to.Owner(), s.Serial, o.source, &o.copy, o.amount))
		return 0
	}
	// Move and Destroy take from a slot that must hold the serial.
	s := o.from.Slot(o.slot)
	if s == nil || s.ID == 0 || s.Serial != o.serial {
		return ReasonNotOwner
	}
	if o.amount <= 0 || o.amount > s.Amount {
		return ReasonAmount
	}
	moved := *s
	moved.Cards, moved.Extra = slices.Clone(s.Cards), s.Clone()
	whole := o.amount == s.Amount
	if !o.from.Take(o.slot, o.amount) {
		return ReasonAmount
	}
	if real {
		emit(Change{Box: o.from, Slot: o.slot, Amount: -o.amount})
	}
	if o.kind == opDestroy {
		if real {
			logs.Items = append(logs.Items, t.line(now, o.from.Owner(), o.end, moved.Serial, item.Serial{}, &moved, o.amount))
		}
		return 0
	}
	slot, r := o.to.Put(&moved, o.amount)
	if r != item.AddSuccess {
		return addReason(r)
	}
	if !real {
		return 0
	}
	d := o.to.Slot(slot)
	var source item.Serial
	switch {
	case d.Serial.IsZero() && whole:
		// A whole stack keeps its serial.
		d.Serial = moved.Serial
	case d.Serial.IsZero():
		// A split gets a new serial.
		d.Serial, source = item.NewSerial(), moved.Serial
	default:
		// A merge takes the stack's serial.
		source = moved.Serial
	}
	emit(Change{Box: o.to, Slot: slot, Amount: o.amount})
	logs.Items = append(logs.Items, t.line(now, o.from.Owner(), o.to.Owner(), d.Serial, source, &moved, o.amount))
	return 0
}

func (t *Tx) zeny(o *op, now time.Time, logs *Logs, emit func(Change)) Reason {
	if o.zeny < 0 || o.zeny > MaxZeny {
		return ReasonZeny
	}
	got := o.zeny
	if o.wfrom != nil && o.wfrom.Zeny() < o.zeny {
		return ReasonZeny
	}
	if o.wto != nil {
		if room := MaxZeny - o.wto.Zeny(); got > room {
			if !o.drop {
				return ReasonZeny
			}
			got = max(room, 0)
		}
	}
	from, to := o.end, o.end
	if o.wfrom != nil {
		o.wfrom.SetZeny(o.wfrom.Zeny() - o.zeny)
		from = o.wfrom.Owner()
		if logs != nil && o.zeny != 0 {
			emit(Change{Wallet: o.wfrom, Zeny: -o.zeny})
		}
	}
	if o.wto != nil {
		o.wto.SetZeny(o.wto.Zeny() + got)
		to = o.wto.Owner()
		if logs != nil && got != 0 {
			emit(Change{Wallet: o.wto, Zeny: got})
		}
	}
	if logs != nil && got > 0 {
		logs.Zeny = append(logs.Zeny, ZenyLog{At: now, Action: t.Action, From: from, To: to, Amount: got})
	}
	return 0
}

func (t *Tx) line(now time.Time, from, to Owner, serial, source item.Serial, s *item.Slot, amount int) ItemLog {
	return ItemLog{At: now, Action: t.Action, From: from, To: to, Serial: serial, Source: source,
		ItemID: s.ID, Amount: amount, Refine: s.Refine, Cards: slices.Clone(s.Cards), Extra: s.Clone()}
}
