package server

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"cromwell/internal/store"
)

// The project-structure rail (DESIGN-008 §8, design round 5). It does not nest;
// it *scopes*. A nested tree cost ~29px of indent per level, so a name had 187px
// at the root and 71px four levels down — unusable on a project of any real
// depth. Scoped, every name gets the full width at any depth, and depth is free.
//
// The one rule that keeps this simple: **the rail has no location of its own.
// The scope follows the page.** It always shows the nearest ancestor initiative
// of whatever is being viewed, plus that initiative's children. There is no
// independent rail state to persist, reconcile, or "reveal current item" into.
//
// The scope is therefore derived from the page's breadcrumbs — which are built
// from the entity's path — and never from client-side state. Anything held in
// JavaScript would be lost by the first HTMX swap that replaced the rail, and a
// shared link would not reproduce it.

// navNode is one row in the rail: a child of the current scope.
type navNode struct {
	Kind        string // "initiative" | "feature" — selects the icon
	Name        string
	URL         string
	State       string // feature lifecycle state; empty for initiatives
	Here        bool   // this row is the page being viewed
	HasChildren bool   // an initiative, so it gets the drill arrow
}

// navScope is the whole rail: the trail of ancestors above the current scope,
// the scope itself, and its children one level deep.
type navScope struct {
	Path     []crumb // ancestors above the scope; empty at the root
	Scope    crumb   // where we are: the project, or an initiative
	AtRoot   bool    // the scope is the project itself
	Children []navNode
}

// navTree builds the rail for the page described by these breadcrumbs. The scope
// is the last crumb that names an initiative; failing that, the project.
func (s *Server) navTree(ctx context.Context, crumbs []crumb) (*navScope, error) {
	// Find the nearest initiative in the trail. For an initiative page that is
	// the page itself; for a feature or a document under one, it is the parent.
	scopeIdx := -1
	for i, c := range crumbs {
		if c.Kind == "initiative" {
			scopeIdx = i
		}
	}

	out := &navScope{}
	if scopeIdx < 0 {
		// At (or effectively at) the project root: the scope is the project and
		// the children are the top-level initiatives.
		out.AtRoot = true
		out.Scope = crumb{Label: "Project", URL: "/ui/project", Kind: "project"}
		roots, err := store.RootInitiatives(ctx, s.Store.Pool)
		if err != nil {
			return nil, err
		}
		here := currentURL(crumbs)
		for i := range roots {
			node, err := s.navChildFor(ctx, roots[i], here)
			if err != nil {
				return nil, err
			}
			out.Children = append(out.Children, node)
		}
		return out, nil
	}

	out.Scope = crumbs[scopeIdx]
	out.Path = append(out.Path, crumbs[:scopeIdx]...)
	// The trail's own crumbs are links; the scope is rendered separately.
	for i := range out.Path {
		out.Path[i].Here = false
	}

	in, err := s.initiativeByURL(ctx, out.Scope.URL)
	if err != nil || in == nil {
		return out, err
	}
	here := currentURL(crumbs)

	kids, err := store.ChildInitiatives(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return nil, err
	}
	for i := range kids {
		node, err := s.navChildFor(ctx, kids[i], here)
		if err != nil {
			return nil, err
		}
		out.Children = append(out.Children, node)
	}

	path := strings.TrimPrefix(out.Scope.URL, "/ui/i/")
	feats, err := store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return nil, err
	}
	for i := range feats {
		u := "/ui/f/" + path + "/" + feats[i].Slug
		out.Children = append(out.Children, navNode{
			Kind:  "feature",
			Name:  feats[i].Name,
			URL:   u,
			State: string(feats[i].State),
			Here:  u == here,
		})
	}
	return out, nil
}

// navChildFor builds one child-initiative row. It carries the drill arrow,
// because an initiative always has an inside to go to.
func (s *Server) navChildFor(ctx context.Context, in store.Initiative, here string) (navNode, error) {
	path, err := s.initiativePath(ctx, in.ID)
	if err != nil {
		return navNode{}, err
	}
	u := "/ui/i/" + path
	return navNode{Kind: "initiative", Name: in.Name, URL: u, Here: u == here, HasChildren: true}, nil
}

// initiativeByURL resolves a rail scope back to its initiative. The URL was
// built from the entity's own slug path, so this is a lookup, not a parse of
// anything a user typed.
func (s *Server) initiativeByURL(ctx context.Context, url string) (*store.Initiative, error) {
	p := strings.TrimPrefix(url, "/ui/i/")
	if p == "" || p == url {
		return nil, nil
	}
	return s.Store.InitiativeBySlugPath(ctx, strings.Split(p, "/"))
}

// currentURL is the address of the page being viewed, so the rail can mark it.
func currentURL(crumbs []crumb) string {
	for i := len(crumbs) - 1; i >= 0; i-- {
		if crumbs[i].Here {
			return crumbs[i].URL
		}
	}
	return ""
}

// --- headed: views telling the shell their title and breadcrumb trail --------
//
// Value receivers on purpose. Handlers pass some of these views by value and
// entityPage by pointer; a value's method set excludes pointer-receiver methods,
// so pointer receivers here would make the shell's type assertion fail silently
// and the page would render with no title and no breadcrumbs. Value receivers
// satisfy the interface either way.

func (p *entityPage) headTitle() string   { return p.Title }
func (p *entityPage) headCrumbs() []crumb { return p.Breadcrumbs }

func (p entityDocPage) headTitle() string { return p.Document.Title }
func (p entityDocPage) headCrumbs() []crumb {
	return p.Breadcrumbs
}

func (p milestonePage) headTitle() string { return p.Milestone.Name }
func (p milestonePage) headCrumbs() []crumb {
	return []crumb{p.Owner, {Label: p.Milestone.Name, Kind: "milestone", Here: true}}
}

func (p roadmapPage) headTitle() string { return p.Roadmap.Name }
func (p roadmapPage) headCrumbs() []crumb {
	return []crumb{p.Owner, {Label: p.Roadmap.Name, Kind: "roadmap", Here: true}}
}

func (p taskPage) headTitle() string { return p.Task.Title }
func (p taskPage) headCrumbs() []crumb {
	c := []crumb{{Label: "Project", URL: "/ui/project", Kind: "project"}}
	if p.FeatureURL != "" {
		c = append(c, crumb{Label: p.FeatureName, URL: p.FeatureURL, Kind: "feature"})
	}
	return append(c, crumb{Label: p.Task.Title, Kind: "task", Here: true})
}

// milestoneURL and roadmapURL are the id-to-URL helpers for click-through.
func milestoneURL(id uuid.UUID) string { return "/ui/m/" + id.String() }
func roadmapURL(id uuid.UUID) string   { return "/ui/r/" + id.String() }
