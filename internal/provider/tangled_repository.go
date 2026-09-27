package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/unmango/terraform-provider-atproto/internal/atproto"
)

const repoCollection = "sh.tangled.repo"

var (
	_ resource.Resource                = &tangledRepositoryResource{}
	_ resource.ResourceWithConfigure   = &tangledRepositoryResource{}
	_ resource.ResourceWithImportState = &tangledRepositoryResource{}
)

// tangledRepositoryResource creates the git repository on a knot, then the
// record that announces it. The record alone is not enough: the knot mints
// the repo DID the record has to carry.
type tangledRepositoryResource struct {
	client *atproto.Client
}

type tangledRepositoryModel struct {
	recordFields
	Name          types.String `tfsdk:"name"`
	Knot          types.String `tfsdk:"knot"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	Source        types.String `tfsdk:"source"`
	Spindle       types.String `tfsdk:"spindle"`
	Description   types.String `tfsdk:"description"`
	Website       types.String `tfsdk:"website"`
	Topics        types.List   `tfsdk:"topics"`
	Labels        types.List   `tfsdk:"labels"`
	RepoDID       types.String `tfsdk:"repo_did"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func (m *tangledRepositoryModel) value() map[string]any {
	return map[string]any{
		"name":        m.Name.ValueString(),
		"knot":        m.Knot.ValueString(),
		"repoDid":     m.RepoDID.ValueString(),
		"source":      optionalString(m.Source),
		"spindle":     optionalString(m.Spindle),
		"description": optionalString(m.Description),
		"website":     optionalString(m.Website),
		"topics":      optionalList(m.Topics),
		"labels":      optionalList(m.Labels),
		"createdAt":   createdAt(&m.CreatedAt),
	}
}

func (m *tangledRepositoryModel) load(v map[string]any) {
	m.Name = loadString(m.Name, v, "name")
	m.Knot = loadString(m.Knot, v, "knot")
	m.RepoDID = loadString(m.RepoDID, v, "repoDid")
	m.Source = loadString(m.Source, v, "source")
	m.Spindle = loadString(m.Spindle, v, "spindle")
	m.Description = loadString(m.Description, v, "description")
	m.Website = loadString(m.Website, v, "website")
	m.Topics = loadList(m.Topics, v, "topics")
	m.Labels = loadList(m.Labels, v, "labels")
	m.CreatedAt = loadString(m.CreatedAt, v, "createdAt")
}

// repoKey is the record key the appview derives from a repository name.
func repoKey(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, ".git"))
}

func NewTangledRepositoryResource() resource.Resource {
	return &tangledRepositoryResource{}
}

func (r *tangledRepositoryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tangled_repository"
}

func (r *tangledRepositoryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates a git repository on a knot and the `" + repoCollection + "` record that announces it.\n\n" +
			"~> Destroying this resource, or changing `name`, `knot`, or `source`, deletes the repository and its history from the knot.",
		Attributes: map[string]schema.Attribute{
			"id":  schema.StringAttribute{MarkdownDescription: "AT URI of the record.", Computed: true, PlanModifiers: keep},
			"uri": schema.StringAttribute{MarkdownDescription: "AT URI of the record.", Computed: true, PlanModifiers: keep},
			"cid": schema.StringAttribute{MarkdownDescription: "CID of the current version of the record.", Computed: true},
			"rkey": schema.StringAttribute{
				MarkdownDescription: "Record key, the lowercased name without a `.git` suffix.",
				Computed:            true,
				PlanModifiers:       keep,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Repository name.",
				Required:            true,
				PlanModifiers:       replace,
			},
			"knot": schema.StringAttribute{
				MarkdownDescription: "Domain of the knot that hosts the repository. The account must be a member of it.",
				Required:            true,
				PlanModifiers:       replace,
			},
			"default_branch": schema.StringAttribute{
				MarkdownDescription: "Default branch. Defaults to the knot's default, usually `main`.",
				Optional:            true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "URL of a repository to clone from when creating this one.",
				Optional:            true,
				PlanModifiers:       replace,
			},
			"spindle": schema.StringAttribute{
				MarkdownDescription: "Spindle that runs CI for the repository.",
				Optional:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Short description, up to 140 characters.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.UTF8LengthBetween(1, 140)},
			},
			"website": schema.StringAttribute{
				MarkdownDescription: "URI related to the repository.",
				Optional:            true,
			},
			"topics": schema.ListAttribute{
				MarkdownDescription: "Up to 50 topics.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtMost(50),
					listvalidator.ValueStringsAre(stringvalidator.LengthBetween(1, 50)),
				},
			},
			"labels": schema.ListAttribute{
				MarkdownDescription: "AT URIs of the label definitions the repository subscribes to.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"repo_did": schema.StringAttribute{
				MarkdownDescription: "DID the knot assigned to the repository.",
				Computed:            true,
				PlanModifiers:       keep,
			},
			"created_at": createdAtAttribute(),
		},
	}
}

func (r *tangledRepositoryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	resp.Diagnostics.Append(configureClient(req.ProviderData, &r.client)...)
}

func (r *tangledRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model tangledRepositoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rkey := repoKey(model.Name.ValueString())
	knot := r.client.Knot(knotDomain(model.Knot.ValueString()))

	repoDID, err := knot.CreateRepo(ctx, atproto.CreateRepoInput{
		Name:          rkey,
		Rkey:          rkey,
		DefaultBranch: model.DefaultBranch.ValueString(),
		Source:        model.Source.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Repository", err.Error())
		return
	}
	model.RepoDID = types.StringValue(repoDID)

	ref, err := r.client.CreateRecord(ctx, repoCollection, rkey, compact(model.value()))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Repository Record", err.Error())
		if cleanup := knot.DeleteRepo(ctx, rkey, rkey); cleanup != nil {
			resp.Diagnostics.AddWarning("Unable to Clean Up Repository",
				"The repository was created on the knot but its record was not. Deleting it again failed: "+cleanup.Error())
		}
		return
	}

	model.set(ref)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *tangledRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model tangledRepositoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rec, err := r.client.GetRecord(ctx, repoCollection, model.RKey.ValueString())
	if errors.Is(err, atproto.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Repository Record", err.Error())
		return
	}

	model.load(rec.Value)
	model.set(rec.Ref)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *tangledRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model, state tangledRepositoryModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if branch := model.DefaultBranch; !branch.IsNull() && !branch.Equal(state.DefaultBranch) {
		knot := r.client.Knot(knotDomain(model.Knot.ValueString()))
		if err := knot.SetDefaultBranch(ctx, state.URI.ValueString(), branch.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to Set Default Branch", err.Error())
			return
		}
	}

	ref, err := updateRecord(ctx, r.client, repoCollection, state.RKey.ValueString(), model.value())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Update Repository Record", err.Error())
		return
	}

	model.set(ref)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *tangledRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model tangledRepositoryModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rkey := model.RKey.ValueString()
	if err := r.client.DeleteRecord(ctx, repoCollection, rkey); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Repository Record", err.Error())
		return
	}

	knot := r.client.Knot(knotDomain(model.Knot.ValueString()))
	if err := knot.DeleteRepo(ctx, model.Name.ValueString(), rkey); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Repository", err.Error())
	}
}

func (r *tangledRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	rkey, err := importKey(req.ID, repoCollection, r.client.DID())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rkey"), rkey)...)
}
