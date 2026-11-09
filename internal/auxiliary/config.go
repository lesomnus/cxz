package auxiliary

import "fmt"

var kinds = []string{"summary", "suggestion", "title"}

func (c *Config) profile(kind string) *Profile {
	switch kind {
	case "summary":
		return &c.Summary
	case "suggestion":
		return &c.Suggestion
	case "title":
		return &c.Title
	}
	return nil
}

// normalize migrates per-kind assignments deterministically. Prefer an enabled
// task, then any configured task, in summary/suggestion/title order. A different
// account's model cannot be assumed to work under the chosen identity.
func (c *Config) normalize() bool {
	before := *c
	identity := Profile{Account: c.Account, Agent: c.Agent, Backend: c.Backend}
	if identity.Account == "" {
		for _, enabledOnly := range []bool{true, false} {
			for _, kind := range kinds {
				p := c.profile(kind)
				if p.Account != "" && (!enabledOnly || p.Enabled) {
					identity = *p
					break
				}
			}
			if identity.Account != "" {
				break
			}
		}
	}
	c.useAccount(identity)
	return before != *c
}

func (c *Config) useAccount(identity Profile) {
	c.Account, c.Agent, c.Backend = identity.Account, identity.Agent, identity.Backend
	for _, kind := range kinds {
		p := c.profile(kind)
		if p.Account != "" && (p.Account != identity.Account || p.Agent != identity.Agent || p.Backend != identity.Backend) {
			p.Enabled, p.Model, p.Effort = false, "", ""
		}
		p.Account, p.Agent, p.Backend = identity.Account, identity.Agent, identity.Backend
	}
}

// withProfiles resolves one shared identity before applying all task settings.
// Mixed-account requests are refused as a whole, rather than saving a prefix.
func (c Config) withProfiles(updates map[string]Profile) (Config, error) {
	var identity *Profile
	for _, kind := range kinds {
		p, ok := updates[kind]
		if !ok || p.Account == "" {
			continue
		}
		if identity != nil && (identity.Account != p.Account || identity.Agent != p.Agent || identity.Backend != p.Backend) {
			return c, fmt.Errorf("auxiliary tasks must use one shared account")
		}
		identity = &p
	}
	if identity != nil {
		c.useAccount(*identity)
	}
	for kind, p := range updates {
		dst := c.profile(kind)
		if dst == nil {
			return c, fmt.Errorf("unknown auxiliary task %q", kind)
		}
		p.Account, p.Agent, p.Backend = c.Account, c.Agent, c.Backend
		if err := p.Validate(); err != nil {
			return c, err
		}
		*dst = p
	}
	return c, nil
}
