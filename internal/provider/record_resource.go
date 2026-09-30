package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// anyRecordModel is a record in any collection, with its body as JSON.
type anyRecordModel struct {
	recordFields
	Collection types.String         `tfsdk:"collection"`
	Record     jsontypes.Normalized `tfsdk:"record"`
}

func (m *anyRecordModel) fields() *recordFields { return &m.recordFields }
func (m *anyRecordModel) collection() string    { return m.Collection.ValueString() }
func (m *anyRecordModel) wholeRecord()          {}

func (m *anyRecordModel) value() map[string]any {
	body := map[string]any{}
	_ = json.Unmarshal([]byte(m.Record.ValueString()), &body)
	delete(body, "$type")

	return body
}

func (m *anyRecordModel) load(v map[string]any) {
	body := make(map[string]any, len(v))
	for k, val := range v {
		if k != "$type" {
			body[k] = val
		}
	}

	if b, err := json.Marshal(body); err == nil {
		m.Record = jsontypes.NewNormalizedValue(string(b))
	}
}

type anyRecordResource struct {
	recordResource[anyRecordModel, *anyRecordModel]
}

func NewRecordResource() resource.Resource {
	return &anyRecordResource{recordResource[anyRecordModel, *anyRecordModel]{spec: recordSpec[*anyRecordModel]{
		name: "record",
		description: "Manages a record in any collection. Prefer a dedicated resource where one exists. " +
			"The `$type` field is set from `collection`.",
		key: keyTID,
		attributes: map[string]schema.Attribute{
			"collection": schema.StringAttribute{
				MarkdownDescription: "NSID of the collection, such as `app.bsky.feed.post`.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"record": schema.StringAttribute{
				MarkdownDescription: "Record body as JSON, usually built with `jsonencode`. Keys and whitespace are compared semantically.",
				CustomType:          jsontypes.NormalizedType{},
				Required:            true,
			},
		},
	}}}
}

// ImportState accepts `collection/rkey` or an AT URI, since the collection
// isn't known from the resource type.
func (r *anyRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	collection, rkey, ok := strings.Cut(strings.TrimPrefix(req.ID, "at://"+r.client.DID()+"/"), "/")
	if !ok {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("expected collection/rkey or an AT URI, got %q", req.ID))
		return
	}

	rkey, err := importKey(rkey, collection, r.client.DID())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("collection"), collection)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rkey"), rkey)...)
}
