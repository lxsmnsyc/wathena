// Package staff holds the staff roles and their permissions (strategy
// section 7.8, proposal 0002 review decision 33, decision 0028):
//   - Permission names come from a fixed list, Catalog. The config is
//     checked against it.
//   - The owner role has every permission. The world's owner account,
//     which `wactl world new` records by ID, has it (review decision
//     54). So do the accounts that the config's staff.owners names, for
//     worlds made before the owner ID.
//   - The config's staff.roles maps role names to permission lists. A
//     name such as moderation.* matches every permission of a group.
//   - Owners and admins give the other roles from the admin screens, and
//     the database keeps them. Registry holds them for the game.
//   - A role counts only for a login with a second factor: a TOTP code,
//     a recovery code or a passkey that verified its user, on an account
//     that still has TOTP or a passkey on.
package staff

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Owner is the role of the world's owner account and the config's
// staff.owners. It has every permission and cannot be given from the
// admin screens.
const Owner = "owner"

// Permission is one name of the fixed list.
type Permission struct {
	Name string
	// Step is the phase 6 step whose admin API or commands use it.
	// Step 1 uses its names now. Later steps reserve theirs, so the
	// default roles can list them and configs can name them already.
	Step int
	// Allows says what the permission allows, for the docs.
	Allows string
}

// Catalog is the fixed list of staff permissions. Its names are
// group.name. The config names permissions from it, or a whole group as
// group.*.
var Catalog = []Permission{
	{Name: "staff.assign", Step: 1, Allows: "list the staff, and give and remove roles"},
	{Name: "audit.read", Step: 1, Allows: "read the GM audit log"},
	{Name: "audit.review", Step: 2, Allows: "mark audit rows as reviewed"},
	{Name: "players.read", Step: 2, Allows: "find accounts and characters, and view their status, position, inventory and storage"},
	{Name: "players.read_ip", Step: 2, Allows: "find accounts by the IP addresses of their logins"},
	{Name: "items.history", Step: 2, Allows: "show every move of an item serial, and a character's item and zeny log"},
	{Name: "items.restore", Step: 2, Allows: "move items that still exist back to a character, with a reason, marked for review"},
	{Name: "items.create", Step: 2, Allows: "make a copy of an item that is gone, or zeny, in a restore, and make items with @item"},
	{Name: "moderation.reports", Step: 3, Allows: "work through the report queue, read reported chat and close reports"},
	{Name: "moderation.mute", Step: 3, Allows: "mute players and lift mutes, from the admin screens and with @mute"},
	{Name: "moderation.kick", Step: 3, Allows: "kick players, from the admin screens and with @kick"},
	{Name: "moderation.jail", Step: 3, Allows: "jail players and lift jails, from the admin screens and with @jail"},
	{Name: "moderation.ban", Step: 3, Allows: "ban accounts for a time and lift bans"},
	{Name: "moderation.ban_permanent", Step: 3, Allows: "ban accounts without an end"},
	{Name: "commands.warp", Step: 3, Allows: "@warp, @jumpto and @go"},
	{Name: "commands.recall", Step: 3, Allows: "@recall"},
	{Name: "commands.hide", Step: 3, Allows: "@hide"},
	{Name: "world.read", Step: 4, Allows: "view zones, player counts and tick times"},
	{Name: "world.announce", Step: 4, Allows: "announce messages"},
	{Name: "world.restart", Step: 4, Allows: "plan restarts with a countdown"},
	{Name: "world.rename", Step: 4, Allows: "rename towns with @renametown (review decision 43)"},
	{Name: "events.control", Step: 4, Allows: "start, stop and schedule world events"},
	{Name: "mods.read", Step: 4, Allows: "list installed mods, their permissions and error counts"},
	{Name: "mods.disable", Step: 4, Allows: "turn a mod off in a zone"},
}

// DefaultRoles are the roles when the config's staff.roles is not set.
// Review decision 34 gives players.read to every role, items.history to
// admin and game_master, and the rest of step 2 to admin alone. Review
// decision 35 gives moderation.ban_permanent to admin alone, so
// game_master lists its moderation permissions.
var DefaultRoles = map[string][]string{
	"admin": {"staff.*", "audit.*", "players.*", "items.*", "moderation.*", "commands.*", "world.*", "events.*", "mods.*"},
	"game_master": {"audit.read", "players.read", "items.history",
		"moderation.reports", "moderation.mute", "moderation.kick", "moderation.jail", "moderation.ban", "commands.*",
		"world.read", "world.announce", "events.control"},
	"moderator": {"players.read", "moderation.reports", "moderation.mute", "moderation.kick", "moderation.jail"},
}

// Known reports whether a permission name is in Catalog.
func Known(name string) bool {
	return slices.ContainsFunc(Catalog, func(p Permission) bool { return p.Name == name })
}

// Names are the names of Catalog, in its order.
func Names() []string {
	out := make([]string, len(Catalog))
	for i, p := range Catalog {
		out[i] = p.Name
	}
	return out
}

// Match returns the permissions of Catalog that a pattern names: the
// name itself, or every name of a group for group.*. It returns nil for
// a pattern that names none.
func Match(pattern string) []string {
	group, ok := strings.CutSuffix(pattern, ".*")
	var out []string
	for _, p := range Catalog {
		if ok && strings.HasPrefix(p.Name, group+".") || !ok && p.Name == pattern {
			out = append(out, p.Name)
		}
	}
	return out
}

var roleName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// ValidRoleName reports whether a role name is 1 to 32 lower-case
// letters, digits or underscores, starting with a letter.
func ValidRoleName(name string) bool { return roleName.MatchString(name) }

// Roles are the configured roles with their permissions. The zero value
// has the owner role only.
type Roles struct {
	perms map[string]map[string]bool
}

// ParseRoles checks a staff.roles mapping against Catalog. nil gives
// DefaultRoles. Unknown permissions, empty groups, bad names and the
// owner role are errors.
func ParseRoles(config map[string][]string) (*Roles, error) {
	if config == nil {
		config = DefaultRoles
	}
	r := &Roles{perms: map[string]map[string]bool{}}
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(config)) {
		switch {
		case name == Owner:
			errs = append(errs, fmt.Errorf("staff.roles: %s comes only from staff.owners, and has every permission", Owner))
			continue
		case !ValidRoleName(name):
			errs = append(errs, fmt.Errorf("staff.roles: %q is not a role name: 1 to 32 lower-case letters, digits or underscores, starting with a letter", name))
			continue
		}
		set := map[string]bool{}
		for _, pattern := range config[name] {
			m := Match(pattern)
			if m == nil {
				errs = append(errs, fmt.Errorf("staff.roles.%s: %q is not a permission; the permissions are %s, and group.* for a group", name, pattern, strings.Join(Names(), ", ")))
			}
			for _, p := range m {
				set[p] = true
			}
		}
		r.perms[name] = set
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return r, nil
}

// MustDefaultRoles returns DefaultRoles as Roles.
func MustDefaultRoles() *Roles {
	r, err := ParseRoles(nil)
	if err != nil {
		panic(err)
	}
	return r
}

// Exists reports whether a role is configured. The owner role always
// exists.
func (r *Roles) Exists(role string) bool {
	if role == Owner {
		return true
	}
	if r == nil {
		return false
	}
	_, ok := r.perms[role]
	return ok
}

// Names are the configured roles, sorted, without the owner role.
func (r *Roles) Names() []string {
	if r == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(r.perms))
}

// Has reports whether a role has a permission. The owner role has every
// permission of Catalog. "" has none.
func (r *Roles) Has(role, permission string) bool {
	if role == Owner {
		return Known(permission)
	}
	if r == nil {
		return false
	}
	return r.perms[role][permission]
}

// Permissions are a role's permissions, in Catalog's order.
func (r *Roles) Permissions(role string) []string {
	var out []string
	for _, p := range Catalog {
		if r.Has(role, p.Name) {
			out = append(out, p.Name)
		}
	}
	return out
}

// Covers reports whether role a has every permission of role b, so an
// account with a may give or take b.
func (r *Roles) Covers(a, b string) bool {
	for _, p := range r.Permissions(b) {
		if !r.Has(a, p) {
			return false
		}
	}
	return true
}

// Config is the config's staff section.
type Config struct {
	// Owners are account names with the owner role.
	Owners []string `yaml:"owners"`
	// Roles map role names to permissions. DefaultRoles when nil.
	Roles map[string][]string `yaml:"roles"`
}

// Build checks the config and returns its roles.
func (c Config) Build() (*Roles, error) {
	var errs []error
	seen := map[string]bool{}
	for _, o := range c.Owners {
		if !validAccountName(o) {
			errs = append(errs, fmt.Errorf("staff.owners: %q is not an account name", o))
		}
		if seen[strings.ToLower(o)] {
			errs = append(errs, fmt.Errorf("staff.owners: %q is listed twice", o))
		}
		seen[strings.ToLower(o)] = true
	}
	r, err := ParseRoles(c.Roles)
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return r, nil
}

var accountName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,23}$`)

// validAccountName is the account service's rule for account names.
func validAccountName(s string) bool { return accountName.MatchString(s) }

// Registry holds the staff roles for the game: the world's owner, the
// owners of the config and the roles the admin screens gave, by account
// ID. It is safe for use by many goroutines. In the all-in-one build the
// admin API tells it of each change through StaffChanged.
type Registry struct {
	roles  *Roles
	owners map[string]bool
	// ownerAccount is the world's owner by account ID, or 0. It is set
	// before the registry is shared.
	ownerAccount int64

	mu       sync.RWMutex
	assigned map[int64]string
}

// NewRegistry makes a registry. owners are account names.
func NewRegistry(roles *Roles, owners []string) *Registry {
	return &Registry{roles: roles, owners: OwnerNames(owners), assigned: map[int64]string{}}
}

// SetOwnerAccount records the world's owner by account ID, from the
// world row (review decision 54). 0 is none. Call it before the
// registry is shared.
func (r *Registry) SetOwnerAccount(account int64) { r.ownerAccount = account }

// Roles are the configured roles.
func (r *Registry) Roles() *Roles { return r.roles }

// IsOwner reports whether an account is an owner: the world's owner
// account, or an account that staff.owners names.
func (r *Registry) IsOwner(account int64, name string) bool {
	return IsOwner(r.ownerAccount, r.owners, account, name)
}

// IsOwner reports whether an account is an owner, given the world's
// owner account (0 for none) and the lowercase names of staff.owners.
// The account service and the admin API use it too.
func IsOwner(ownerAccount int64, ownerNames map[string]bool, account int64, name string) bool {
	if ownerAccount != 0 && account == ownerAccount {
		return true
	}
	return ownerNames[strings.ToLower(name)]
}

// OwnerNames returns the lowercase names of staff.owners as a set.
func OwnerNames(owners []string) map[string]bool {
	out := make(map[string]bool, len(owners))
	for _, o := range owners {
		out[strings.ToLower(o)] = true
	}
	return out
}

// OwnersByNameWarning is the warning at start when staff.owners is set
// (review decision 54). Owners by name still work for worlds made before
// the owner ID, but a name with no account can be taken at signup.
const OwnersByNameWarning = "staff.owners names owners by account name. New worlds record their owner by account ID; keep staff.owners only for owners that already have an account"

// Load replaces the given roles, such as with the database's at start.
func (r *Registry) Load(assigned map[int64]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assigned = maps.Clone(assigned)
	if r.assigned == nil {
		r.assigned = map[int64]string{}
	}
}

// RoleOf returns an account's role: owner for the world's owner account
// or an owner's name, else the given role while the config still has
// it, else "". It does not check the login's second factor; the caller
// does.
func (r *Registry) RoleOf(account int64, name string) string {
	if r == nil {
		return ""
	}
	if r.IsOwner(account, name) {
		return Owner
	}
	r.mu.RLock()
	role := r.assigned[account]
	r.mu.RUnlock()
	if !r.roles.Exists(role) {
		return ""
	}
	return role
}

// StaffChanged records an account's new role, or "" for none. It is
// what the admin API calls after a change.
func (r *Registry) StaffChanged(_ context.Context, account int64, role string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if role == "" {
		delete(r.assigned, account)
	} else {
		r.assigned[account] = role
	}
	return nil
}

// Login is how a game login logged in, for its staff role.
type Login struct {
	// AccountName is the account's name, for staff.owners.
	AccountName string
	// TwoFactor is set when the login's session used a second factor (a
	// TOTP code, a recovery code, or a passkey that verified its user)
	// and the account had TOTP or a passkey on.
	TwoFactor bool
	// Recent is set when the login's session proved who its player is
	// within the reauth window (10 minutes by default) when the game
	// ticket was made. A stolen session cookie cannot make it so.
	Recent bool
}

// recentPermissions are the permissions whose GM commands need a
// recent login (review decision 51): the commands that make items and
// the sanctions. The admin API asks the same of restores and bans.
var recentPermissions = map[string]bool{
	"items.create":             true,
	"moderation.mute":          true,
	"moderation.jail":          true,
	"moderation.kick":          true,
	"moderation.ban":           true,
	"moderation.ban_permanent": true,
}

// NeedsRecentLogin reports whether a GM command of a permission needs a
// game login whose ticket was made soon after the player proved who
// they are.
func NeedsRecentLogin(permission string) bool { return recentPermissions[permission] }

// Allows reports whether a game login may use a GM command of a
// permission as far as its login's age goes. The role's permissions are
// checked apart.
func (l Login) Allows(permission string) bool { return l.Recent || !NeedsRecentLogin(permission) }

// ReauthText is the answer to a GM command that needs a recent login.
const ReauthText = "This command needs a recent login. Log in again on the account page, then reconnect."

// RoleOfLogin returns the role of a game login: the account's role when
// the login used a second factor, else "".
func (r *Registry) RoleOfLogin(account int64, l Login) string {
	if !l.TwoFactor {
		return ""
	}
	return r.RoleOf(account, l.AccountName)
}

// AuditRow is one row of the GM audit log, as the game side writes it:
// in the commit of the change it records, such as a restore.
type AuditRow struct {
	AccountID   int64
	AccountName string
	Role        string
	// Action is what the staff member did, such as items.restore.
	Action string
	// Source is admin or game.
	Source string
	Target string
	// TargetAccount is the target's account ID, or 0.
	TargetAccount int64
	Arguments     map[string]any
	Reason        string
	// NeedsReview marks the row for review. It needs a reason.
	NeedsReview bool
}

// BanText is what a banned player sees at login and when the ban kicks
// it: the end time in UTC, or that it has none, and the reason
// (decision 0030).
func BanText(reason string, end time.Time, permanent bool) string {
	if permanent || end.IsZero() {
		return "Your account is banned permanently. Reason: " + reason
	}
	return "Your account is banned until " + end.UTC().Format("2006-01-02 15:04 MST") + ". Reason: " + reason
}

// KickText is what a kicked player sees.
func KickText(reason string) string { return "Staff kicked you from the game. Reason: " + reason }
