package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/store"
)

// The MCP tool handlers (SPEC-008 FR-2, FR-3). Each calls the SAME gated,
// audited service method the web UI and the CLI call, in one transaction with
// its audit row (O-3, SD-4), as the configured MCP actor (SD-3). None
// re-implements logic, and none issues HTTP to /api/* (NFR-2).
//
// Errors are written for a person: the chat agent relays them, so they say what
// went wrong and what to do about it, never a raw database fault (NFR-5). A
// collision fails cleanly and re-runnably, with no partial write, because each
// mutation is one transaction (NFR-7).

// --- Writes (FR-2) ---

// mcpCreateInitiative implements FR-2.1 over store.CreateInitiative.
func (s *Server) mcpCreateInitiative(r *http.Request, args map[string]any) (any, error) {
	slug, ok := argString(args, "slug")
	if !ok {
		return nil, errors.New("a slug is required — a short lower-case identifier such as \"auth\"")
	}
	name, ok := argString(args, "name")
	if !ok {
		return nil, errors.New("a name is required — the human-readable title, such as \"Authentication\"")
	}
	description, _ := argString(args, "description")

	ctx := r.Context()
	var parentID *uuid.UUID
	parentPath, hasParent := argString(args, "parent_path")
	if hasParent {
		parent, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(parentPath, "/"))
		if err != nil {
			return nil, fmt.Errorf("there is no initiative at %q, so nothing can be created under it; "+
				"call get_tree to see what exists", parentPath)
		}
		parentID = &parent.ID
	}

	var in *store.Initiative
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		in, e = store.CreateInitiative(ctx, tx, parentID, slug, name, description, s.mcpActor())
		return e
	})
	if err != nil {
		return nil, initiativeCreateError(err, slug, parentPath)
	}
	path := slug
	if hasParent {
		path = parentPath + "/" + slug
	}
	// Publishing nothing here would leave an open UI page stale; the UI's own
	// create path publishes no bus event either, and both surfaces refresh
	// through the SSE hub, so signal the change presentation-only (FR-5.1).
	s.notifyEntityChanged("initiative", in.ID)
	return map[string]any{
		"path": path, "name": in.Name, "description": in.Description,
		"url": "/ui/i/" + path,
	}, nil
}

// initiativeCreateError turns a constraint violation into the plain-language
// explanation the chat agent relays (FR-2.1 AC, NFR-5/NFR-7).
func initiativeCreateError(err error, slug, parentPath string) error {
	if isUniqueViolation(err) {
		where := "at the top level"
		if parentPath != "" {
			where = "under " + parentPath
		}
		return fmt.Errorf("an initiative with the slug %q already exists %s; "+
			"choose a different slug, or update the existing one instead", slug, where)
	}
	return err
}

// mcpCreateFeature implements FR-2.2 over store.CreateFeature. The feature is
// created as an idea — its normal starting state. There is no parameter and no
// tool that advances it into development (DEC-004).
func (s *Server) mcpCreateFeature(r *http.Request, args map[string]any) (any, error) {
	initPath, ok := argString(args, "initiative_path")
	if !ok {
		return nil, errors.New("the path of the owning initiative is required, for example \"auth\"")
	}
	slug, ok := argString(args, "slug")
	if !ok {
		return nil, errors.New("a slug is required — a short lower-case identifier such as \"login\"")
	}
	name, ok := argString(args, "name")
	if !ok {
		return nil, errors.New("a name is required — the human-readable title, such as \"Login form\"")
	}
	description, _ := argString(args, "description")

	ctx := r.Context()
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(initPath, "/"))
	if err != nil {
		return nil, fmt.Errorf("there is no initiative at %q; call get_tree to see what exists", initPath)
	}
	var f *store.Feature
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		f, e = store.CreateFeature(ctx, tx, in.ID, slug, name, description, s.mcpActor())
		return e
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("a feature with the slug %q already exists under %s; "+
				"choose a different slug, or update the existing one instead", slug, initPath)
		}
		return nil, err
	}
	s.notifyEntityChanged("feature", f.ID)
	s.Bus.Publish(bus.FeatureCreated{FeatureID: f.ID})
	path := initPath + "/" + slug
	return map[string]any{
		"path": path, "name": f.Name, "description": f.Description,
		"state": string(f.State), "url": "/ui/f/" + path,
	}, nil
}

// mcpUpdateInitiative and mcpUpdateFeature implement FR-2.3 over the additive
// UpdateEntityFields — the same method the web UI's in-place edit calls, so the
// two authoring surfaces write one field and cannot diverge (FR-6.1).
func (s *Server) mcpUpdateInitiative(r *http.Request, args map[string]any) (any, error) {
	path, ok := argString(args, "path")
	if !ok {
		return nil, errors.New("the path of the initiative to update is required, for example \"auth\"")
	}
	ctx := r.Context()
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("there is no initiative at %q; call get_tree to see what exists", path)
	}
	name, description := argOptionalString(args, "name"), argOptionalString(args, "description")
	if name == nil && description == nil {
		return nil, errors.New("give a new name, a new description, or both — otherwise there is nothing to change")
	}
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.UpdateEntityFields(ctx, tx, "initiative", in.ID, name, description, s.mcpActor())
	}); err != nil {
		return nil, err
	}
	s.notifyEntityChanged("initiative", in.ID)
	updated, err := store.GetInitiative(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path": path, "name": updated.Name, "description": updated.Description,
		"url": "/ui/i/" + path,
	}, nil
}

func (s *Server) mcpUpdateFeature(r *http.Request, args map[string]any) (any, error) {
	path, ok := argString(args, "path")
	if !ok {
		return nil, errors.New("the path of the feature to update is required, for example \"auth/login\"")
	}
	ctx := r.Context()
	f, err := s.featureByPath(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("there is no feature at %q; call get_tree to see what exists", path)
	}
	name, description := argOptionalString(args, "name"), argOptionalString(args, "description")
	if name == nil && description == nil {
		return nil, errors.New("give a new name, a new description, or both — otherwise there is nothing to change")
	}
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.UpdateEntityFields(ctx, tx, "feature", f.ID, name, description, s.mcpActor())
	}); err != nil {
		return nil, err
	}
	s.notifyEntityChanged("feature", f.ID)
	// Describing a feature is an authoring trigger (SPEC-009 FR-4.3).
	if description != nil && *description != "" {
		s.Bus.Publish(bus.FeatureDescribed{FeatureID: f.ID})
	}
	updated, err := store.GetFeature(ctx, s.Store.Pool, f.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path": path, "name": updated.Name, "description": updated.Description,
		"state": string(updated.State), "url": "/ui/f/" + path,
	}, nil
}

// mcpAttachDocument implements FR-2.4 over the shared RegisterDocForOwner path —
// the same capability as the UI's "attach a document" and the CLI's `doc add`
// (DEC-003). It registers and indexes a file that already exists; it never
// writes content (git owns content, vision §12).
func (s *Server) mcpAttachDocument(r *http.Request, args map[string]any) (any, error) {
	path, ok := argString(args, "path")
	if !ok {
		return nil, errors.New("the file's path within the repository is required, for example \"docs/design/login.md\"")
	}
	ownerType, ok := argString(args, "owner_type")
	if !ok {
		return nil, errors.New("say what owns the document: \"project\", \"initiative\" or \"feature\"")
	}
	docType, hasType := argString(args, "doc_type")
	if !hasType {
		docType = "design"
	}
	ctx := r.Context()
	ownerPath, _ := argString(args, "owner_path")
	ownerID, err := s.mcpResolveOwner(ctx, ownerType, ownerPath)
	if err != nil {
		return nil, err
	}
	doc, err := s.RegisterDocForOwner(ctx, path, docType, ownerType, ownerID, s.mcpActor())
	if err != nil {
		return nil, attachError(err, path)
	}
	s.notifyEntityChanged("document", doc.ID)
	return map[string]any{
		"id": doc.ID.String(), "path": doc.Path, "title": doc.Title,
		"type": doc.Type, "state": string(doc.State), "url": "/ui/d/" + doc.Path,
	}, nil
}

// attachError explains the two failures a chat agent actually hits: the file is
// not there yet, or it is already registered (NFR-5, NFR-7).
func attachError(err error, path string) error {
	if isUniqueViolation(err) {
		return fmt.Errorf("the file %q is already registered as a document; "+
			"call list_documents to see what is attached", path)
	}
	if strings.Contains(err.Error(), "no such file") {
		return fmt.Errorf("there is no file at %q in the repository. Write the file and commit it "+
			"first, then attach it — this tool records a file, it does not create one", path)
	}
	return err
}

// mcpResolveOwner maps an owner kind + path to an owner id, in the plain
// language of the tool's arguments.
func (s *Server) mcpResolveOwner(ctx context.Context, ownerType, ownerPath string) (*uuid.UUID, error) {
	switch ownerType {
	case "project":
		return nil, nil
	case "initiative":
		if ownerPath == "" {
			return nil, errors.New("owner_path is required when the owner is an initiative")
		}
		in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(ownerPath, "/"))
		if err != nil {
			return nil, fmt.Errorf("there is no initiative at %q; call get_tree to see what exists", ownerPath)
		}
		return &in.ID, nil
	case "feature":
		if ownerPath == "" {
			return nil, errors.New("owner_path is required when the owner is a feature")
		}
		f, err := s.featureByPath(ctx, ownerPath)
		if err != nil {
			return nil, fmt.Errorf("there is no feature at %q; call get_tree to see what exists", ownerPath)
		}
		return &f.ID, nil
	}
	return nil, fmt.Errorf("owner_type must be \"project\", \"initiative\" or \"feature\", not %q", ownerType)
}

// --- Reads (FR-3) ---

// mcpGetTree implements FR-3.1: the initiative → feature tree, enough for the
// agent to know where to create.
func (s *Server) mcpGetTree(r *http.Request, _ map[string]any) (any, error) {
	ctx := r.Context()
	roots, err := store.RootInitiatives(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(roots))
	for _, in := range roots {
		node, err := s.mcpTreeNode(ctx, in, in.Slug)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return map[string]any{"initiatives": out}, nil
}

func (s *Server) mcpTreeNode(ctx context.Context, in store.Initiative, path string) (map[string]any, error) {
	node := map[string]any{
		"path": path, "name": in.Name, "description": in.Description, "archived": in.Archived,
	}
	feats, err := store.FeaturesForInitiative(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return nil, err
	}
	if len(feats) > 0 {
		fs := make([]any, 0, len(feats))
		for _, f := range feats {
			fs = append(fs, map[string]any{
				"path": path + "/" + f.Slug, "name": f.Name,
				"description": f.Description, "state": string(f.State),
			})
		}
		node["features"] = fs
	}
	children, err := store.ChildInitiatives(ctx, s.Store.Pool, in.ID)
	if err != nil {
		return nil, err
	}
	if len(children) > 0 {
		cs := make([]any, 0, len(children))
		for _, c := range children {
			cn, err := s.mcpTreeNode(ctx, c, path+"/"+c.Slug)
			if err != nil {
				return nil, err
			}
			cs = append(cs, cn)
		}
		node["initiatives"] = cs
	}
	return node, nil
}

// mcpGetInitiative implements FR-3.2.
func (s *Server) mcpGetInitiative(r *http.Request, args map[string]any) (any, error) {
	path, ok := argString(args, "path")
	if !ok {
		return nil, errors.New("the path of the initiative is required, for example \"auth\"")
	}
	ctx := r.Context()
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("there is no initiative at %q; call get_tree to see what exists", path)
	}
	node, err := s.mcpTreeNode(ctx, *in, path)
	if err != nil {
		return nil, err
	}
	docs, err := store.DocumentsForOwner(ctx, s.Store.Pool, "initiative", &in.ID)
	if err != nil {
		return nil, err
	}
	node["documents"] = mcpDocList(docs)
	node["url"] = "/ui/i/" + path
	return node, nil
}

// mcpGetFeature implements FR-3.2.
func (s *Server) mcpGetFeature(r *http.Request, args map[string]any) (any, error) {
	path, ok := argString(args, "path")
	if !ok {
		return nil, errors.New("the path of the feature is required, for example \"auth/login\"")
	}
	ctx := r.Context()
	f, err := s.featureByPath(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("there is no feature at %q; call get_tree to see what exists", path)
	}
	docs, err := store.DocumentsForOwner(ctx, s.Store.Pool, "feature", &f.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path": path, "name": f.Name, "description": f.Description,
		"state": string(f.State), "documents": mcpDocList(docs), "url": "/ui/f/" + path,
	}, nil
}

// mcpListDocuments implements FR-3.3.
func (s *Server) mcpListDocuments(r *http.Request, args map[string]any) (any, error) {
	ownerType, ok := argString(args, "owner_type")
	if !ok {
		return nil, errors.New("say whose documents to list: \"project\", \"initiative\" or \"feature\"")
	}
	ctx := r.Context()
	ownerPath, _ := argString(args, "owner_path")
	ownerID, err := s.mcpResolveOwner(ctx, ownerType, ownerPath)
	if err != nil {
		return nil, err
	}
	docs, err := store.DocumentsForOwner(ctx, s.Store.Pool, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"documents": mcpDocList(docs)}, nil
}

func mcpDocList(docs []store.Document) []any {
	out := make([]any, 0, len(docs))
	for _, d := range docs {
		out = append(out, map[string]any{
			"id": d.ID.String(), "path": d.Path, "title": d.Title,
			"type": d.Type, "state": string(d.State), "is_primary": d.IsPrimary,
			"url": "/ui/d/" + d.Path,
		})
	}
	return out
}

// --- Shared plumbing ---

// notifyEntityChanged fans a presentation-only signal into the SSE hub so an
// open web UI page reflects an MCP authoring mutation live (FR-5.1). It carries
// no authority and never affects whether the mutation happened — exactly the
// pattern notifyCheckpointRaised uses (SD-5).
func (s *Server) notifyEntityChanged(refType string, refID uuid.UUID) {
	if s.Hub == nil {
		return
	}
	s.Hub.Broadcast(entityChanged{RefType: refType, RefID: refID})
}

// entityChanged is the presentation-only event an authoring mutation broadcasts.
type entityChanged struct {
	RefType string
	RefID   uuid.UUID
}

func (e entityChanged) EventKind() string { return e.RefType + ".changed" }

// isUniqueViolation reports whether an error is a Postgres unique-constraint
// violation, so a slug collision can be explained rather than leaked raw
// (NFR-5, NFR-7).
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
