package chat

import (
	"errors"
	"time"
)

// Limits are the chat rate limits, which are server settings (review
// decision 26). A player may say Burst lines at once, and one more every
// Refill after that. A character in its first NewPeriod after it was
// made has the lower NewBurst and NewRefill, which slows spam bots
// (strategy section 7.10). rAthena's min_chat_delay does the same with a
// burst of 1, and is off by default.
type Limits struct {
	Burst  int           `yaml:"burst"`
	Refill time.Duration `yaml:"refill"`
	// NewBurst and NewRefill apply for NewPeriod after the character was
	// made. NewPeriod 0 turns them off.
	NewBurst  int           `yaml:"new_character_burst"`
	NewRefill time.Duration `yaml:"new_character_refill"`
	NewPeriod time.Duration `yaml:"new_character_period"`
}

// DefaultLimits are the limits without settings: 5 lines at once and 1
// per second, and 3 lines at once and 1 per 3 seconds in a character's
// first hour.
func DefaultLimits() Limits {
	return Limits{Burst: 5, Refill: time.Second, NewBurst: 3, NewRefill: 3 * time.Second, NewPeriod: time.Hour}
}

// Check checks the limits' values.
func (l Limits) Check() error {
	var errs []error
	if l.Burst < 1 || l.Burst > 100 {
		errs = append(errs, errors.New("burst is 1 to 100"))
	}
	if l.Refill < 10*time.Millisecond || l.Refill > time.Minute {
		errs = append(errs, errors.New("refill is 10ms to 1m"))
	}
	if l.NewPeriod < 0 || l.NewPeriod > 30*24*time.Hour {
		errs = append(errs, errors.New("new_character_period is 0 to 720h"))
	}
	if l.NewPeriod > 0 {
		if l.NewBurst < 1 || l.NewBurst > l.Burst {
			errs = append(errs, errors.New("new_character_burst is 1 to burst"))
		}
		if l.NewRefill < l.Refill || l.NewRefill > time.Hour {
			errs = append(errs, errors.New("new_character_refill is refill to 1h"))
		}
	}
	return errors.Join(errs...)
}

// For returns the burst and refill for a character made at created, at
// now. A zero created is a character made now.
func (l Limits) For(created, now time.Time) (int, time.Duration) {
	if l.NewPeriod > 0 && (created.IsZero() || now.Sub(created) < l.NewPeriod) {
		return l.NewBurst, l.NewRefill
	}
	return l.Burst, l.Refill
}
