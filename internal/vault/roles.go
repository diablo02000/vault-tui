package vault

// roleKind describes one browsable collection inside an auth method.
type roleKind struct {
	label    string // shown in the UI
	listPath string // path under auth/<mount>/ to LIST
	readPath string // path under auth/<mount>/ whose child <name> is read
}

// roleKinds maps an auth method type to the collections that can be listed.
// Vault has no generic "list roles" endpoint, so each type is declared here.
var roleKinds = map[string][]roleKind{
	"approle":    {{"roles", "role", "role"}},
	"kubernetes": {{"roles", "role", "role"}},
	"jwt":        {{"roles", "role", "role"}},
	"oidc":       {{"roles", "role", "role"}},
	"aws":        {{"roles", "role", "role"}},
	"azure":      {{"roles", "role", "role"}},
	"gcp":        {{"roles", "roles", "role"}},
	"userpass":   {{"users", "users", "users"}},
	"radius":     {{"users", "users", "users"}},
	"ldap":       {{"users", "users", "users"}, {"groups", "groups", "groups"}},
	"okta":       {{"users", "users", "users"}, {"groups", "groups", "groups"}},
	"github":     {{"teams", "map/teams", "map/teams"}, {"users", "map/users", "map/users"}},
	"cert":       {{"certs", "certs", "certs"}},
	"token":      {{"roles", "roles", "roles"}},
}
