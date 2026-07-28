package server

import (
	"context"

	"github.com/google/uuid"

	"cromwell/internal/store"
)

// The navigation tree in the rail (DESIGN-008 §8): the project's initiatives and
// features, as *navigation furniture* rather than a work surface (D-7). Browsing
// proper is one page at a time, by breadcrumb and by a page's own list of
// children; this exists so you can jump sideways without going up first.
//
// It is deliberately shallow work: the roll-up tree — sizes, tiers, the honest
// "?" for unestimated work — is the analytical view, and that belongs to Stage B.

// navNode is one line in the rail's tree. A node with children renders as a
// native <details> so disclosure survives an HTMX swap with no JavaScript; a
// leaf renders as a link carrying its lifecycle state as a dot.
type navNode struct {
	Kind     string // "initiative" | "feature" — also selects the icon
	Name     string
	URL      string
	State    string // feature lifecycle state; empty for initiatives
	Here     bool   // this node is the page being viewed
	Open     bool   // an ancestor of the current page, so it starts expanded
	Children []navNode
}

// navTree builds the rail's tree. The crumbs of the current page tell it which
// branches to open and which leaf to mark, so the tree always shows where you
// are without a second lookup.
func (s *Server) navTree(ctx context.Context, crumbs []crumb) ([]navNode, error) {
	onPath := make(map[string]bool, len(crumbs))
	for _, c := range crumbs {
		if c.URL != "" {
			onPath[c.URL] = true
		}
		if c.Here && c.URL != "" {
			onPath[c.URL] = true
		}
	}
	here := ""
	if n := len(crumbs); n > 0 && crumbs[n-1].Here {
		here = crumbs[n-1].URL
	}

	roots, err := store.RootInitiatives(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	out := make([]navNode, 0, len(roots))
	for i := range roots {
		node, err := s.navNodeFor(ctx, roots[i], onPath, here)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, nil
}

// navNodeFor builds one initiative's node and, recursively, everything under it.
func (s *Server) navNodeFor(ctx context.Context, in store.Initiative, onPath map[string]bool, here string) (navNode, error) {
	path, err := s.initiativePath(ctx, in.ID)
	if err != nil {
		return navNode{}, err
	}
	url := "/ui/i/" + path
	node := navNode{Kind: "initiative", Name: in.Name, URL: url}

	kids, err := store.ChildInitiatives(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return navNode{}, err
	}
	for i := range kids {
		child, err := s.navNodeFor(ctx, kids[i], onPath, here)
		if err != nil {
			return navNode{}, err
		}
		node.Children = append(node.Children, child)
	}

	feats, err := store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return navNode{}, err
	}
	for i := range feats {
		fURL := "/ui/f/" + path + "/" + feats[i].Slug
		node.Children = append(node.Children, navNode{
			Kind:  "feature",
			Name:  feats[i].Name,
			URL:   fURL,
			State: string(feats[i].State),
			Here:  fURL == here,
		})
	}

	node.Here = url == here
	// Open this branch when the current page is inside it, so the tree arrives
	// already showing where you are.
	node.Open = onPath[url] || node.Here || anyOpen(node.Children)
	return node, nil
}

func anyOpen(nodes []navNode) bool {
	for _, n := range nodes {
		if n.Here || n.Open || anyOpen(n.Children) {
			return true
		}
	}
	return false
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

// milestoneRef is the id-to-URL helper the templates use for click-through.
func milestoneURL(id uuid.UUID) string { return "/ui/m/" + id.String() }
func roadmapURL(id uuid.UUID) string   { return "/ui/r/" + id.String() }
