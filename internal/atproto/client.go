// Package atproto is the small slice of the AT Protocol the provider needs:
// record CRUD on the account's PDS, handle resolution, and service-auth calls
// to Tangled knots.
package atproto

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// ErrNotFound is returned when a record or other object does not exist.
var ErrNotFound = errors.New("not found")

type Config struct {
	// Handle or DID of the account.
	Identifier string

	AppPassword string

	// PDSHost skips DID document resolution when set, e.g. "https://bsky.social".
	PDSHost string
}

type Client struct {
	api *atclient.APIClient

	// KnotURL maps a knot domain to its base URL. Tests point it at a local server.
	KnotURL func(domain string) string
}

func Login(ctx context.Context, cfg Config) (*Client, error) {
	var (
		api *atclient.APIClient
		err error
	)

	if cfg.PDSHost != "" {
		api, err = atclient.LoginWithPasswordHost(ctx, cfg.PDSHost, cfg.Identifier, cfg.AppPassword, "", nil)
	} else {
		var id syntax.AtIdentifier
		if id, err = syntax.ParseAtIdentifier(cfg.Identifier); err != nil {
			return nil, err
		}
		api, err = atclient.LoginWithPassword(ctx, identity.DefaultDirectory(), id, cfg.AppPassword, "", nil)
	}
	if err != nil {
		return nil, err
	}

	return &Client{api: api, KnotURL: httpsURL}, nil
}

func httpsURL(domain string) string {
	return "https://" + domain
}

// DID of the logged in account, which owns every record the client writes.
func (c *Client) DID() string {
	return c.api.AccountDID.String()
}

type Ref struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type Record struct {
	Ref
	Value map[string]any `json:"value"`
}

// CreateRecord writes a new record. An empty rkey lets the PDS mint a TID.
func (c *Client) CreateRecord(ctx context.Context, collection, rkey string, value map[string]any) (Ref, error) {
	body := map[string]any{
		"repo":       c.DID(),
		"collection": collection,
		"record":     withType(collection, value),
	}
	if rkey != "" {
		body["rkey"] = rkey
	}

	var out Ref
	err := c.api.Post(ctx, "com.atproto.repo.createRecord", body, &out)

	return out, err
}

// PutRecord creates or replaces a record. A non-empty swapCID makes the write
// fail if the record changed since it was read.
func (c *Client) PutRecord(ctx context.Context, collection, rkey string, value map[string]any, swapCID string) (Ref, error) {
	body := map[string]any{
		"repo":       c.DID(),
		"collection": collection,
		"rkey":       rkey,
		"record":     withType(collection, value),
	}
	if swapCID != "" {
		body["swapRecord"] = swapCID
	}

	var out Ref
	err := c.api.Post(ctx, "com.atproto.repo.putRecord", body, &out)

	return out, err
}

func (c *Client) GetRecord(ctx context.Context, collection, rkey string) (*Record, error) {
	params := map[string]any{
		"repo":       c.DID(),
		"collection": collection,
		"rkey":       rkey,
	}

	var out Record
	if err := c.api.Get(ctx, "com.atproto.repo.getRecord", params, &out); err != nil {
		if isRecordNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &out, nil
}

// DeleteRecord removes a record. Deleting a record that is already gone succeeds.
func (c *Client) DeleteRecord(ctx context.Context, collection, rkey string) error {
	body := map[string]any{
		"repo":       c.DID(),
		"collection": collection,
		"rkey":       rkey,
	}

	return c.api.Post(ctx, "com.atproto.repo.deleteRecord", body, nil)
}

func (c *Client) ResolveHandle(ctx context.Context, handle string) (string, error) {
	var out struct {
		DID string `json:"did"`
	}
	err := c.api.Get(ctx, "com.atproto.identity.resolveHandle", map[string]any{"handle": handle}, &out)

	return out.DID, err
}

// withType returns value with its $type set, leaving the caller's map untouched.
func withType(collection string, value map[string]any) map[string]any {
	out := make(map[string]any, len(value)+1)
	for k, v := range value {
		out[k] = v
	}
	out["$type"] = collection

	return out
}

func isRecordNotFound(err error) bool {
	var apiErr *atclient.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.Name == "RecordNotFound" || apiErr.StatusCode == http.StatusNotFound
}

// serviceAuth asks the PDS for a token that lets a service (aud) act for the
// account, scoped to a single method (lxm).
func (c *Client) serviceAuth(ctx context.Context, aud, lxm string) (string, error) {
	params := map[string]any{
		"aud": aud,
		"lxm": lxm,
		"exp": time.Now().Add(2 * time.Minute).Unix(),
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := c.api.Get(ctx, "com.atproto.server.getServiceAuth", params, &out); err != nil {
		return "", fmt.Errorf("getting service auth for %s: %w", lxm, err)
	}

	return out.Token, nil
}

// serviceDID is the did:web a knot or spindle at domain is addressed as.
func serviceDID(domain string) string {
	return "did:web:" + strings.ReplaceAll(domain, ":", "%3A")
}
