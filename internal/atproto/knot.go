package atproto

import (
	"context"
	"fmt"
	"slices"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// capabilityACL marks knots that manage members and collaborators through
// XRPC calls. Older knots read them from PDS records instead, which this
// provider does not support.
const capabilityACL = "knot-acl"

// Knot is a Tangled knot server. Procedures are authenticated with a
// service-auth token the account's PDS issues for that knot and method.
type Knot struct {
	client *Client
	domain string
}

func (c *Client) Knot(domain string) *Knot {
	return &Knot{client: c, domain: domain}
}

func (k *Knot) api() *atclient.APIClient {
	return atclient.NewAPIClient(k.client.KnotURL(k.domain))
}

func (k *Knot) procedure(ctx context.Context, nsid string, body, out any) error {
	token, err := k.client.serviceAuth(ctx, serviceDID(k.domain), nsid)
	if err != nil {
		return err
	}

	api := k.api()
	api.Headers.Set("Authorization", "Bearer "+token)

	if err := api.Post(ctx, syntax.NSID(nsid), body, out); err != nil {
		return fmt.Errorf("knot %s: %s: %w", k.domain, nsid, err)
	}

	return nil
}

type listItem struct {
	Subject string `json:"subject"`
}

// list pages through a knot list query and returns every subject.
func (k *Knot) list(ctx context.Context, nsid, subject string) ([]string, error) {
	var (
		subjects []string
		cursor   string
	)
	for {
		params := map[string]any{"subject": subject, "limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}

		var out struct {
			Items  []listItem `json:"items"`
			Cursor *string    `json:"cursor"`
		}
		if err := k.api().Get(ctx, syntax.NSID(nsid), params, &out); err != nil {
			return nil, fmt.Errorf("knot %s: %s: %w", k.domain, nsid, err)
		}

		for _, item := range out.Items {
			subjects = append(subjects, item.Subject)
		}
		if out.Cursor == nil || *out.Cursor == "" || len(out.Items) == 0 {
			return subjects, nil
		}
		cursor = *out.Cursor
	}
}

// RequireACL fails unless the knot manages members and collaborators over XRPC.
func (k *Knot) RequireACL(ctx context.Context) error {
	var out struct {
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
	if err := k.api().Get(ctx, "sh.tangled.knot.version", nil, &out); err != nil {
		return fmt.Errorf("knot %s: sh.tangled.knot.version: %w", k.domain, err)
	}

	if !slices.Contains(out.Capabilities, capabilityACL) {
		return fmt.Errorf("knot %s (version %s) lacks the %q capability; upgrade the knot", k.domain, out.Version, capabilityACL)
	}

	return nil
}

type CreateRepoInput struct {
	Name          string `json:"name"`
	Rkey          string `json:"rkey"`
	DefaultBranch string `json:"defaultBranch,omitempty"`
	Source        string `json:"source,omitempty"`
}

// CreateRepo creates the git repository and returns the repo DID the knot minted.
func (k *Knot) CreateRepo(ctx context.Context, in CreateRepoInput) (string, error) {
	var out struct {
		RepoDID string `json:"repoDid"`
	}
	if err := k.procedure(ctx, "sh.tangled.repo.create", in, &out); err != nil {
		return "", err
	}

	return out.RepoDID, nil
}

func (k *Knot) DeleteRepo(ctx context.Context, name, rkey string) error {
	return k.procedure(ctx, "sh.tangled.repo.delete", map[string]any{
		"did":  k.client.DID(),
		"name": name,
		"rkey": rkey,
	}, nil)
}

// SetDefaultBranch changes the default branch of the repo whose record is at repoURI.
func (k *Knot) SetDefaultBranch(ctx context.Context, repoURI, branch string) error {
	return k.procedure(ctx, "sh.tangled.repo.setDefaultBranch", map[string]any{
		"repo":          repoURI,
		"defaultBranch": branch,
	}, nil)
}

func (k *Knot) AddMember(ctx context.Context, subject string) error {
	return k.procedure(ctx, "sh.tangled.knot.addMember", map[string]any{"subject": subject}, nil)
}

func (k *Knot) RemoveMember(ctx context.Context, subject string) error {
	return k.procedure(ctx, "sh.tangled.knot.removeMember", map[string]any{"subject": subject}, nil)
}

func (k *Knot) Members(ctx context.Context) ([]string, error) {
	return k.list(ctx, "sh.tangled.knot.listMembers", k.domain)
}

func (k *Knot) AddCollaborator(ctx context.Context, repoDID, subject string) error {
	return k.procedure(ctx, "sh.tangled.repo.addCollaborator", map[string]any{"repo": repoDID, "subject": subject}, nil)
}

func (k *Knot) RemoveCollaborator(ctx context.Context, repoDID, subject string) error {
	return k.procedure(ctx, "sh.tangled.repo.removeCollaborator", map[string]any{"repo": repoDID, "subject": subject}, nil)
}

func (k *Knot) Collaborators(ctx context.Context, repoDID string) ([]string, error) {
	return k.list(ctx, "sh.tangled.repo.listCollaborators", repoDID)
}
