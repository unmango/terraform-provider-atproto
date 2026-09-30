package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/unmango/terraform-provider-atproto/internal/atproto"
)

// Knot members and repository collaborators live in the knot's ACL. Knots
// with the "knot-acl" capability manage them over XRPC and the appview
// writes no record for them, so neither does this provider.

var didPattern = regexp.MustCompile(`^did:[a-z]+:[a-zA-Z0-9._:%-]*[a-zA-Z0-9._-]$`)

// aclEntry is one subject in a knot-side list, such as a member of a knot or
// a collaborator on a repository.
type aclEntry struct {
	name        string
	description string

	// scope names the attribute, besides knot and subject, that identifies
	// the list. Empty when the list belongs to the knot itself.
	scope            string
	scopeDescription string

	add    func(ctx context.Context, k *atproto.Knot, scope, subject string) error
	remove func(ctx context.Context, k *atproto.Knot, scope, subject string) error
	list   func(ctx context.Context, k *atproto.Knot, scope string) ([]string, error)
}

type aclResource struct {
	entry  aclEntry
	client *atproto.Client
}

var (
	_ resource.Resource                = &aclResource{}
	_ resource.ResourceWithConfigure   = &aclResource{}
	_ resource.ResourceWithImportState = &aclResource{}
)

// aclModel is read attribute by attribute, since the scope attribute's name
// differs between resources.
type aclModel struct {
	ID      types.String
	Knot    types.String
	Scope   types.String
	Subject types.String
}

func NewTangledKnotMemberResource() resource.Resource {
	return &aclResource{entry: aclEntry{
		name:        "tangled_knot_member",
		description: "Adds an account to a knot, letting it create repositories there. The provider's account must own the knot.",
		add: func(ctx context.Context, k *atproto.Knot, _, subject string) error {
			return k.AddMember(ctx, subject)
		},
		remove: func(ctx context.Context, k *atproto.Knot, _, subject string) error {
			return k.RemoveMember(ctx, subject)
		},
		list: func(ctx context.Context, k *atproto.Knot, _ string) ([]string, error) {
			return k.Members(ctx)
		},
	}}
}

func NewTangledRepositoryCollaboratorResource() resource.Resource {
	return &aclResource{entry: aclEntry{
		name:             "tangled_repository_collaborator",
		description:      "Grants an account push access to a repository. The provider's account must own the repository.",
		scope:            "repo_did",
		scopeDescription: "DID of the repository, the `repo_did` of an `atproto_tangled_repository`.",
		add: func(ctx context.Context, k *atproto.Knot, repo, subject string) error {
			return k.AddCollaborator(ctx, repo, subject)
		},
		remove: func(ctx context.Context, k *atproto.Knot, repo, subject string) error {
			return k.RemoveCollaborator(ctx, repo, subject)
		},
		list: func(ctx context.Context, k *atproto.Knot, repo string) ([]string, error) {
			return k.Collaborators(ctx, repo)
		},
	}}
}

func (r *aclResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.entry.name
}

func (r *aclResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "The identifying attributes joined with `/`: " + r.idFormat() + ".",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"knot": schema.StringAttribute{
			MarkdownDescription: "Domain of the knot.",
			Required:            true,
			PlanModifiers:       replace,
		},
		"subject": func() schema.StringAttribute {
			a := didAttribute("DID of the account.")
			a.PlanModifiers = replace
			return a
		}(),
	}
	if r.entry.scope != "" {
		a := didAttribute(r.entry.scopeDescription)
		a.PlanModifiers = replace
		attrs[r.entry.scope] = a
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: r.entry.description + "\n\nThe knot must advertise the `knot-acl` capability.",
		Attributes:          attrs,
	}
}

func (r *aclResource) idFormat() string {
	if r.entry.scope == "" {
		return "`knot/subject`"
	}

	return "`knot/" + r.entry.scope + "/subject`"
}

func (r *aclResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	resp.Diagnostics.Append(configureClient(req.ProviderData, &r.client)...)
}

func (r *aclResource) get(ctx context.Context, getAttr func(context.Context, path.Path, any) diag.Diagnostics) (aclModel, diag.Diagnostics) {
	var (
		m     aclModel
		diags diag.Diagnostics
	)
	diags.Append(getAttr(ctx, path.Root("knot"), &m.Knot)...)
	diags.Append(getAttr(ctx, path.Root("subject"), &m.Subject)...)
	diags.Append(getAttr(ctx, path.Root("id"), &m.ID)...)
	if r.entry.scope != "" {
		diags.Append(getAttr(ctx, path.Root(r.entry.scope), &m.Scope)...)
	}

	return m, diags
}

func (r *aclResource) set(ctx context.Context, setAttr func(context.Context, path.Path, any) diag.Diagnostics, m aclModel) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(setAttr(ctx, path.Root("id"), m.ID)...)
	diags.Append(setAttr(ctx, path.Root("knot"), m.Knot)...)
	diags.Append(setAttr(ctx, path.Root("subject"), m.Subject)...)
	if r.entry.scope != "" {
		diags.Append(setAttr(ctx, path.Root(r.entry.scope), m.Scope)...)
	}

	return diags
}

func (r *aclResource) id(m aclModel) string {
	parts := []string{knotDomain(m.Knot.ValueString())}
	if r.entry.scope != "" {
		parts = append(parts, m.Scope.ValueString())
	}

	return strings.Join(append(parts, m.Subject.ValueString()), "/")
}

func (r *aclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	m, diags := r.get(ctx, req.Plan.GetAttribute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	knot := r.client.Knot(knotDomain(m.Knot.ValueString()))
	if err := knot.RequireACL(ctx); err != nil {
		resp.Diagnostics.AddError("Unsupported Knot", err.Error())
		return
	}
	if err := r.entry.add(ctx, knot, m.Scope.ValueString(), m.Subject.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Add "+m.Subject.ValueString(), err.Error())
		return
	}

	m.ID = types.StringValue(r.id(m))
	resp.Diagnostics.Append(r.set(ctx, resp.State.SetAttribute, m)...)
}

func (r *aclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	m, diags := r.get(ctx, req.State.GetAttribute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	knot := r.client.Knot(knotDomain(m.Knot.ValueString()))
	subjects, err := r.entry.list(ctx, knot, m.Scope.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to List "+r.entry.name, err.Error())
		return
	}

	if !slices.Contains(subjects, m.Subject.ValueString()) {
		resp.State.RemoveResource(ctx)
		return
	}

	m.ID = types.StringValue(r.id(m))
	resp.Diagnostics.Append(r.set(ctx, resp.State.SetAttribute, m)...)
}

// Update is never called: every attribute forces replacement.
func (r *aclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Unexpected Update", r.entry.name+" has no attributes that can change in place.")
}

func (r *aclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	m, diags := r.get(ctx, req.State.GetAttribute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	knot := r.client.Knot(knotDomain(m.Knot.ValueString()))
	if err := r.entry.remove(ctx, knot, m.Scope.ValueString(), m.Subject.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Remove "+m.Subject.ValueString(), err.Error())
	}
}

func (r *aclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	want := 2
	if r.entry.scope != "" {
		want = 3
	}

	parts := strings.Split(req.ID, "/")
	if len(parts) != want {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("expected %s, got %q", r.idFormat(), req.ID))
		return
	}

	m := aclModel{
		ID:      types.StringValue(req.ID),
		Knot:    types.StringValue(parts[0]),
		Subject: types.StringValue(parts[len(parts)-1]),
	}
	if r.entry.scope != "" {
		m.Scope = types.StringValue(parts[1])
	}

	resp.Diagnostics.Append(r.set(ctx, resp.State.SetAttribute, m)...)
}
