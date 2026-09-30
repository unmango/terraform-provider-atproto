package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Resources in this file are fully described by their record; Tangled's
// appview and knots pick up the changes from the firehose.

func createdAtAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "Record creation time. Defaults to the time the record is created.",
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func didAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Required:            true,
		Validators:          []validator.String{stringvalidator.RegexMatches(didPattern, "must be a DID")},
	}
}

type tangledFollowModel struct {
	recordFields
	Subject   types.String `tfsdk:"subject"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (m *tangledFollowModel) fields() *recordFields { return &m.recordFields }

func (m *tangledFollowModel) value() map[string]any {
	return map[string]any{
		"subject":   m.Subject.ValueString(),
		"createdAt": createdAt(&m.CreatedAt),
	}
}

func (m *tangledFollowModel) load(v map[string]any) {
	m.Subject = loadString(m.Subject, v, "subject")
	m.CreatedAt = loadString(m.CreatedAt, v, "createdAt")
}

func NewTangledFollowResource() resource.Resource {
	return &recordResource[tangledFollowModel, *tangledFollowModel]{spec: recordSpec[*tangledFollowModel]{
		name:        "tangled_follow",
		collection:  "sh.tangled.graph.follow",
		description: "Follows an account on Tangled.",
		key:         keyTID,
		attributes: map[string]schema.Attribute{
			"subject":    didAttribute("DID of the account to follow."),
			"created_at": createdAtAttribute(),
		},
	}}
}

type tangledKnotModel struct {
	recordFields
	Domain    types.String `tfsdk:"domain"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (m *tangledKnotModel) fields() *recordFields { return &m.recordFields }

func (m *tangledKnotModel) value() map[string]any {
	return map[string]any{"createdAt": createdAt(&m.CreatedAt)}
}

func (m *tangledKnotModel) load(v map[string]any) {
	m.Domain = types.StringValue(m.RKey.ValueString())
	m.CreatedAt = loadString(m.CreatedAt, v, "createdAt")
}

func NewTangledKnotResource() resource.Resource {
	return &recordResource[tangledKnotModel, *tangledKnotModel]{spec: recordSpec[*tangledKnotModel]{
		name:       "tangled_knot",
		collection: "sh.tangled.knot",
		description: "Registers a knot with Tangled. The appview verifies the registration by asking the knot for its owner, " +
			"so the knot must be reachable over HTTPS and configured with this account as its owner.",
		key: keyDerived,
		rkey: func(m *tangledKnotModel) string {
			return knotDomain(m.Domain.ValueString())
		},
		attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: "Domain the knot serves, such as `knot.example.com`. It is the record key.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": createdAtAttribute(),
		},
	}}
}

// knotDomain normalizes a knot address the way the appview does.
func knotDomain(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")

	return strings.TrimSuffix(s, "/")
}

type tangledPublicKeyModel struct {
	recordFields
	Name      types.String `tfsdk:"name"`
	Key       types.String `tfsdk:"key"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (m *tangledPublicKeyModel) fields() *recordFields { return &m.recordFields }

func (m *tangledPublicKeyModel) value() map[string]any {
	return map[string]any{
		"name":      m.Name.ValueString(),
		"key":       m.Key.ValueString(),
		"createdAt": createdAt(&m.CreatedAt),
	}
}

func (m *tangledPublicKeyModel) load(v map[string]any) {
	m.Name = loadString(m.Name, v, "name")
	m.Key = loadString(m.Key, v, "key")
	m.CreatedAt = loadString(m.CreatedAt, v, "createdAt")
}

func NewTangledPublicKeyResource() resource.Resource {
	return &recordResource[tangledPublicKeyModel, *tangledPublicKeyModel]{spec: recordSpec[*tangledPublicKeyModel]{
		name:        "tangled_public_key",
		collection:  "sh.tangled.publicKey",
		description: "Adds an SSH public key to the account. Knots read these records to authorize pushes.",
		key:         keyTID,
		attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable name for the key.",
				Required:            true,
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "Public key in OpenSSH `authorized_keys` format.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtMost(4096)},
			},
			"created_at": createdAtAttribute(),
		},
	}}
}

var profileStats = []string{
	"merged-pull-request-count",
	"closed-pull-request-count",
	"open-pull-request-count",
	"open-issue-count",
	"closed-issue-count",
	"repository-count",
	"star-count",
}

type tangledProfileModel struct {
	recordFields
	Bluesky            types.Bool   `tfsdk:"bluesky"`
	Description        types.String `tfsdk:"description"`
	Location           types.String `tfsdk:"location"`
	Pronouns           types.String `tfsdk:"pronouns"`
	PreferredHandle    types.String `tfsdk:"preferred_handle"`
	Links              types.List   `tfsdk:"links"`
	Stats              types.List   `tfsdk:"stats"`
	PinnedRepositories types.List   `tfsdk:"pinned_repositories"`
}

func (m *tangledProfileModel) fields() *recordFields { return &m.recordFields }

func (m *tangledProfileModel) value() map[string]any {
	return map[string]any{
		"bluesky":            m.Bluesky.ValueBool(),
		"description":        optionalString(m.Description),
		"location":           optionalString(m.Location),
		"pronouns":           optionalString(m.Pronouns),
		"preferredHandle":    optionalString(m.PreferredHandle),
		"links":              optionalList(m.Links),
		"stats":              optionalList(m.Stats),
		"pinnedRepositories": optionalList(m.PinnedRepositories),
	}
}

func (m *tangledProfileModel) load(v map[string]any) {
	if b, ok := v["bluesky"].(bool); ok {
		m.Bluesky = types.BoolValue(b)
	}
	m.Description = loadString(m.Description, v, "description")
	m.Location = loadString(m.Location, v, "location")
	m.Pronouns = loadString(m.Pronouns, v, "pronouns")
	m.PreferredHandle = loadString(m.PreferredHandle, v, "preferredHandle")
	m.Links = loadList(m.Links, v, "links")
	m.Stats = loadList(m.Stats, v, "stats")
	m.PinnedRepositories = loadList(m.PinnedRepositories, v, "pinnedRepositories")
}

func NewTangledProfileResource() resource.Resource {
	return &recordResource[tangledProfileModel, *tangledProfileModel]{spec: recordSpec[*tangledProfileModel]{
		name:       "tangled_profile",
		collection: "sh.tangled.actor.profile",
		description: "Manages the account's Tangled profile. There is one profile per account, so creating this " +
			"resource adopts an existing profile. The avatar is left as it is.",
		key: keySelf,
		attributes: map[string]schema.Attribute{
			"bluesky": schema.BoolAttribute{
				MarkdownDescription: "Link to this account on Bluesky.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-form profile description.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtMost(2560)},
			},
			"location": schema.StringAttribute{
				MarkdownDescription: "Free-form location.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtMost(400)},
			},
			"pronouns": schema.StringAttribute{
				MarkdownDescription: "Preferred pronouns.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtMost(40)},
			},
			"preferred_handle": schema.StringAttribute{
				MarkdownDescription: "Handle to display the account as.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtMost(253)},
			},
			"links": schema.ListAttribute{
				MarkdownDescription: "Up to 5 URIs, such as social profiles or websites.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators:          []validator.List{listvalidator.SizeAtMost(5)},
			},
			"stats": schema.ListAttribute{
				MarkdownDescription: "Up to 2 vanity stats to show. One of `" + strings.Join(profileStats, "`, `") + "`.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtMost(2),
					listvalidator.ValueStringsAre(stringvalidator.OneOf(profileStats...)),
				},
			},
			"pinned_repositories": schema.ListAttribute{
				MarkdownDescription: "Up to 6 pinned repositories, as repo DIDs or AT URIs.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators:          []validator.List{listvalidator.SizeAtMost(6)},
			},
		},
	}}
}
