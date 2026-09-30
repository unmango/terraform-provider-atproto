package provider

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

const (
	testDID    = "did:plc:alice"
	testHandle = "alice.test"
	testKnot   = "knot.test"
)

type fakeRecord struct {
	CID   string
	Value map[string]any
}

// fakePDS serves the XRPC methods the provider calls on a PDS, for one account.
type fakePDS struct {
	mu      sync.Mutex
	server  *httptest.Server
	seq     int
	records map[string]map[string]fakeRecord
	handles map[string]string
}

func newFakePDS() *fakePDS {
	p := &fakePDS{
		records: map[string]map[string]fakeRecord{},
		handles: map[string]string{testHandle: testDID, "bob.test": "did:plc:bob"},
	}
	p.server = httptest.NewServer(http.HandlerFunc(p.serve))

	return p
}

func (p *fakePDS) Close() { p.server.Close() }

func (p *fakePDS) URL() string { return p.server.URL }

func (p *fakePDS) record(collection, rkey string) (fakeRecord, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.records[collection][rkey]

	return r, ok
}

func (p *fakePDS) keys(collection string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Sorted(maps.Keys(p.records[collection]))
}

// put stores a record directly, as if another client wrote it.
func (p *fakePDS) put(collection, rkey string, value map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.store(collection, rkey, value)
}

func (p *fakePDS) remove(collection, rkey string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.records[collection], rkey)
}

func (p *fakePDS) store(collection, rkey string, value map[string]any) map[string]any {
	p.seq++
	if p.records[collection] == nil {
		p.records[collection] = map[string]fakeRecord{}
	}
	cid := fmt.Sprintf("bafycid%d", p.seq)
	p.records[collection][rkey] = fakeRecord{CID: cid, Value: value}

	return map[string]any{"uri": uri(collection, rkey), "cid": cid}
}

func uri(collection, rkey string) string {
	return "at://" + testDID + "/" + collection + "/" + rkey
}

func (p *fakePDS) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var body map[string]any
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	q := r.URL.Query()
	str := func(k string) string { s, _ := body[k].(string); return s }

	switch strings.TrimPrefix(r.URL.Path, "/xrpc/") {
	case "com.atproto.server.createSession":
		if str("password") != "app-password" {
			xrpcError(w, http.StatusUnauthorized, "AuthenticationRequired")
			return
		}
		writeJSON(w, map[string]any{"accessJwt": "access", "refreshJwt": "refresh", "did": testDID, "handle": testHandle})

	case "com.atproto.identity.resolveHandle":
		did, ok := p.handles[q.Get("handle")]
		if !ok {
			xrpcError(w, http.StatusBadRequest, "HandleNotFound")
			return
		}
		writeJSON(w, map[string]any{"did": did})

	case "com.atproto.server.getServiceAuth":
		writeJSON(w, map[string]any{"token": serviceToken(q.Get("aud"), q.Get("lxm"))})

	case "com.atproto.repo.getRecord":
		rec, ok := p.records[q.Get("collection")][q.Get("rkey")]
		if !ok {
			xrpcError(w, http.StatusBadRequest, "RecordNotFound")
			return
		}
		writeJSON(w, map[string]any{"uri": uri(q.Get("collection"), q.Get("rkey")), "cid": rec.CID, "value": rec.Value})

	case "com.atproto.repo.createRecord":
		collection, rkey := str("collection"), str("rkey")
		if rkey == "" {
			rkey = syntax.NewTIDNow(0).String()
		}
		if _, ok := p.records[collection][rkey]; ok {
			xrpcError(w, http.StatusBadRequest, "InvalidRequest")
			return
		}
		writeJSON(w, p.store(collection, rkey, body["record"].(map[string]any)))

	case "com.atproto.repo.putRecord":
		collection, rkey := str("collection"), str("rkey")
		if swap := str("swapRecord"); swap != "" && swap != p.records[collection][rkey].CID {
			xrpcError(w, http.StatusBadRequest, "InvalidSwap")
			return
		}
		writeJSON(w, p.store(collection, rkey, body["record"].(map[string]any)))

	case "com.atproto.repo.deleteRecord":
		delete(p.records[str("collection")], str("rkey"))
		writeJSON(w, map[string]any{})

	default:
		xrpcError(w, http.StatusNotImplemented, "MethodNotImplemented")
	}
}

func serviceToken(aud, lxm string) string {
	return "service:" + aud + ":" + lxm
}

// fakeKnot serves a knot's XRPC methods, checking each procedure carries a
// service-auth token scoped to it.
type fakeKnot struct {
	mu            sync.Mutex
	server        *httptest.Server
	capabilities  []string
	repos         map[string]string
	defaultBranch map[string]string
	members       []string
	collaborators map[string][]string
	calls         []string
}

func newFakeKnot() *fakeKnot {
	k := &fakeKnot{
		capabilities:  []string{"knot-acl"},
		repos:         map[string]string{},
		defaultBranch: map[string]string{},
		collaborators: map[string][]string{},
	}
	k.server = httptest.NewServer(http.HandlerFunc(k.serve))

	return k
}

func (k *fakeKnot) Close() { k.server.Close() }

func (k *fakeKnot) URL() string { return k.server.URL }

func (k *fakeKnot) Calls() []string {
	k.mu.Lock()
	defer k.mu.Unlock()

	return slices.Clone(k.calls)
}

func (k *fakeKnot) serve(w http.ResponseWriter, r *http.Request) {
	k.mu.Lock()
	defer k.mu.Unlock()

	nsid := strings.TrimPrefix(r.URL.Path, "/xrpc/")
	q := r.URL.Query()

	var body map[string]any
	if r.Method == http.MethodPost {
		want := "Bearer " + serviceToken("did:web:"+testKnot, nsid)
		if got := r.Header.Get("Authorization"); got != want {
			xrpcError(w, http.StatusUnauthorized, "AuthMissing")
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		k.calls = append(k.calls, nsid)
	}
	str := func(key string) string { s, _ := body[key].(string); return s }

	switch nsid {
	case "sh.tangled.knot.version":
		writeJSON(w, map[string]any{"version": "v1.16.1", "capabilities": k.capabilities})

	case "sh.tangled.repo.create":
		did := "did:plc:repo-" + str("rkey")
		k.repos[str("rkey")] = did
		k.defaultBranch[did] = cmp.Or(str("defaultBranch"), "main")
		writeJSON(w, map[string]any{"repoDid": did})

	case "sh.tangled.repo.delete":
		delete(k.repos, str("rkey"))
		writeJSON(w, map[string]any{})

	case "sh.tangled.repo.setDefaultBranch":
		// The repo is named by its record's AT URI.
		rkey := str("repo")[strings.LastIndex(str("repo"), "/")+1:]
		k.defaultBranch[k.repos[rkey]] = str("defaultBranch")
		writeJSON(w, map[string]any{})

	case "sh.tangled.knot.addMember":
		k.members = append(k.members, str("subject"))
		writeJSON(w, map[string]any{})

	case "sh.tangled.knot.removeMember":
		k.members = slices.DeleteFunc(k.members, func(s string) bool { return s == str("subject") })
		writeJSON(w, map[string]any{})

	case "sh.tangled.knot.listMembers":
		writeJSON(w, map[string]any{"items": items(k.members)})

	case "sh.tangled.repo.addCollaborator":
		k.collaborators[str("repo")] = append(k.collaborators[str("repo")], str("subject"))
		writeJSON(w, map[string]any{})

	case "sh.tangled.repo.removeCollaborator":
		k.collaborators[str("repo")] = slices.DeleteFunc(k.collaborators[str("repo")], func(s string) bool { return s == str("subject") })
		writeJSON(w, map[string]any{})

	case "sh.tangled.repo.listCollaborators":
		writeJSON(w, map[string]any{"items": items(k.collaborators[q.Get("subject")])})

	default:
		xrpcError(w, http.StatusNotImplemented, "MethodNotImplemented")
	}
}

func items(subjects []string) []map[string]any {
	out := []map[string]any{}
	for _, s := range subjects {
		out = append(out, map[string]any{"subject": s})
	}

	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func xrpcError(w http.ResponseWriter, status int, name string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": name, "message": name})
}
