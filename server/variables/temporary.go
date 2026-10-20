package variables

import (
	"fmt"
	"sync"
)

// Values that are not saved (review decision 58): rAthena's temporary
// server variables ($@x) and NPC variables (.x). Every zone reads and
// writes the same value, as with server variables. They are lost when
// the server stops, as in rAthena.

// TempKey names one value that is not saved. NPC is "" for a temporary
// server variable, and the NPC's full record name for an NPC variable.
type TempKey struct {
	Mod, NPC, Name string
}

// Temporary holds the values that are not saved. Zones read and write
// them from their own goroutines. One mod's values have the limits of
// its server variables: MaxModValues values, which core does not have,
// and MaxModBytes bytes.
type Temporary struct {
	mu     sync.Mutex
	values map[TempKey]string
	count  map[string]int
	bytes  map[string]int
}

// NewTemporary makes an empty set of values.
func NewTemporary() *Temporary {
	return &Temporary{values: map[TempKey]string{}, count: map[string]int{}, bytes: map[string]int{}}
}

// Get returns a value as JSON, and false when it has none.
func (t *Temporary) Get(k TempKey) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.values[k]
	return v, ok
}

// Set changes a value. An empty value is the default, which is not
// kept.
func (t *Temporary) Set(k TempKey, value string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.put(k, value)
}

// Update changes a value in one step: f gets the current value, or ""
// without one, and returns the new value. Other zones wait while f
// runs, so f must be quick.
func (t *Temporary) Update(k TempKey, f func(old string) (string, error)) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	next, err := f(t.values[k])
	if err == nil {
		err = t.put(k, next)
	}
	if err != nil {
		return "", err
	}
	return next, nil
}

// DropNPC forgets every NPC variable of an NPC, when it is removed, as
// rAthena's unloadnpc frees the NPC's variables.
func (t *Temporary) DropNPC(npc string) {
	if npc == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, v := range t.values {
		if k.NPC == npc {
			delete(t.values, k)
			t.count[k.Mod]--
			t.bytes[k.Mod] -= len(v)
		}
	}
}

// put changes a value within the mod's limits. The lock must be held.
func (t *Temporary) put(k TempKey, value string) error {
	old, had := t.values[k]
	count, bytes := t.count[k.Mod], t.bytes[k.Mod]-len(old)
	if had {
		count--
	}
	if value != "" {
		count++
		bytes += len(value)
		if !had && count > MaxModValues && k.Mod != Core {
			return fmt.Errorf("%s holds %d temporary and NPC variables, the most a mod may", k.Mod, MaxModValues)
		}
		if len(value) > len(old) && bytes > MaxModBytes {
			return fmt.Errorf("%s's temporary and NPC variables would take %d bytes, more than the %d a mod may", k.Mod, bytes, MaxModBytes)
		}
		t.values[k] = value
	} else {
		delete(t.values, k)
	}
	t.count[k.Mod], t.bytes[k.Mod] = count, bytes
	return nil
}
