package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/unmango/terraform-provider-atproto/internal/atproto"
)

// keyMode is how a collection's record key is chosen, following the
// lexicon's "key" field.
type keyMode int

const (
	// keyTID lets the PDS mint a timestamp key unless rkey is configured.
	keyTID keyMode = iota
	// keySelf is the literal key "self", for one-per-account records.
	keySelf
	// keyDerived computes the key from the model, e.g. a knot's domain.
	keyDerived
)

// recordFields are the attributes every record resource shares.
type recordFields struct {
	ID   types.String `tfsdk:"id"`
	URI  types.String `tfsdk:"uri"`
	CID  types.String `tfsdk:"cid"`
	RKey types.String `tfsdk:"rkey"`
}

func (f *recordFields) set(ref atproto.Ref) {
	f.ID = types.StringValue(ref.URI)
	f.URI = types.StringValue(ref.URI)
	f.CID = types.StringValue(ref.CID)
	f.RKey = types.StringValue(rkeyOf(ref.URI))
}

// recordModel is implemented by a pointer to each record resource's model.
type recordModel interface {
	fields() *recordFields

	// value returns the record body. It fills any unknown computed
	// attributes, such as created_at, first. A nil entry removes the key when
	// updating an existing record, which preserves keys the model doesn't know.
	value() map[string]any

	// load copies a record read from the PDS into the model.
	load(value map[string]any)
}

// collectionModel is implemented by models whose collection is configured
// rather than fixed by the resource type.
type collectionModel interface {
	collection() string
}

// wholeRecord is implemented by models that describe the entire record, so
// an update replaces it instead of merging into it.
type wholeRecord interface {
	wholeRecord()
}

type recordSpec[P recordModel] struct {
	// name is the type name after the "atproto_" prefix.
	name        string
	collection  string
	description string
	key         keyMode
	rkey        func(P) string
	attributes  map[string]schema.Attribute
}

// recordResource manages a single record in one collection.
type recordResource[M any, P interface {
	*M
	recordModel
}] struct {
	spec   recordSpec[P]
	client *atproto.Client
}

func (r *recordResource[M, P]) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.name
}

func (r *recordResource[M, P]) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "AT URI of the record.",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"uri": schema.StringAttribute{
			MarkdownDescription: "AT URI of the record.",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"cid": schema.StringAttribute{
			MarkdownDescription: "CID of the current version of the record.",
			Computed:            true,
		},
		"rkey": r.rkeyAttribute(),
	}
	for name, a := range r.spec.attributes {
		attrs[name] = a
	}

	description := r.spec.description
	if r.spec.collection != "" {
		description += "\n\nManages a `" + r.spec.collection + "` record."
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: description,
		Attributes:          attrs,
	}
}

func (r *recordResource[M, P]) rkeyAttribute() schema.StringAttribute {
	if r.spec.key != keyTID {
		return schema.StringAttribute{
			MarkdownDescription: "Record key.",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}

	return schema.StringAttribute{
		MarkdownDescription: "Record key. The PDS generates a TID when this is not set.",
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func (r *recordResource[M, P]) collectionOf(p P) string {
	if c, ok := any(p).(collectionModel); ok {
		return c.collection()
	}

	return r.spec.collection
}

func (r *recordResource[M, P]) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	resp.Diagnostics.Append(configureClient(req.ProviderData, &r.client)...)
}

func (r *recordResource[M, P]) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p := P(&model)

	var (
		ref atproto.Ref
		err error
	)
	collection := r.collectionOf(p)
	body := compact(p.value())
	switch r.spec.key {
	case keySelf:
		// The account may already have this record; adopt it, keeping the
		// fields the model doesn't manage.
		ref, err = updateRecord(ctx, r.client, collection, "self", body)
	case keyDerived:
		ref, err = r.client.CreateRecord(ctx, collection, r.spec.rkey(p), body)
	default:
		ref, err = r.client.CreateRecord(ctx, collection, p.fields().RKey.ValueString(), body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Record", collection+": "+err.Error())
		return
	}

	p.fields().set(ref)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *recordResource[M, P]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model M
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p := P(&model)

	collection := r.collectionOf(p)
	rec, err := r.client.GetRecord(ctx, collection, p.fields().RKey.ValueString())
	if errors.Is(err, atproto.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Record", collection+": "+err.Error())
		return
	}

	p.load(rec.Value)
	p.fields().set(rec.Ref)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *recordResource[M, P]) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p := P(&model)

	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	collection := r.collectionOf(p)
	rkey := P(&state).fields().RKey.ValueString()

	var (
		ref atproto.Ref
		err error
	)
	if _, ok := any(p).(wholeRecord); ok {
		ref, err = r.client.PutRecord(ctx, collection, rkey, p.value(), P(&state).fields().CID.ValueString())
	} else {
		ref, err = updateRecord(ctx, r.client, collection, rkey, p.value())
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to Update Record", collection+": "+err.Error())
		return
	}

	p.fields().set(ref)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *recordResource[M, P]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model M
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p := P(&model)
	collection := r.collectionOf(p)
	if err := r.client.DeleteRecord(ctx, collection, p.fields().RKey.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to Delete Record", collection+": "+err.Error())
	}
}

func (r *recordResource[M, P]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	rkey, err := importKey(req.ID, r.spec.collection, r.client.DID())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rkey"), rkey)...)
}

// updateRecord merges changes into the stored record, so keys the provider
// doesn't model survive, and writes it back guarded by the CID it read.
func updateRecord(ctx context.Context, c *atproto.Client, collection, rkey string, changes map[string]any) (atproto.Ref, error) {
	current, err := c.GetRecord(ctx, collection, rkey)
	if err != nil && !errors.Is(err, atproto.ErrNotFound) {
		return atproto.Ref{}, err
	}

	merged := map[string]any{}
	swap := ""
	if current != nil {
		merged = current.Value
		delete(merged, "$type")
		swap = current.CID
	}
	for k, v := range changes {
		if v == nil {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}

	return c.PutRecord(ctx, collection, rkey, merged, swap)
}

// importKey accepts a bare record key or an AT URI naming a record in
// collection owned by did.
func importKey(id, collection, did string) (string, error) {
	if !strings.HasPrefix(id, "at://") {
		if _, err := syntax.ParseRecordKey(id); err != nil {
			return "", fmt.Errorf("%q is neither a record key nor an AT URI: %w", id, err)
		}
		return id, nil
	}

	uri, err := syntax.ParseATURI(id)
	if err != nil {
		return "", err
	}
	if uri.Collection().String() != collection {
		return "", fmt.Errorf("%s is in collection %s, not %s", id, uri.Collection(), collection)
	}
	if owner, err := uri.Authority().AsDID(); err != nil || owner.String() != did {
		return "", fmt.Errorf("%s is not owned by the provider's account %s", id, did)
	}

	return uri.RecordKey().String(), nil
}

func rkeyOf(uri string) string {
	return uri[strings.LastIndex(uri, "/")+1:]
}

func configureClient(data any, client **atproto.Client) diag.Diagnostics {
	var diags diag.Diagnostics
	if data == nil {
		return diags
	}

	c, ok := data.(*atproto.Client)
	if !ok {
		diags.AddError("Unexpected Provider Data", fmt.Sprintf("expected *atproto.Client, got %T", data))
		return diags
	}
	*client = c

	return diags
}

// compact drops nil entries, which only mean something when updating.
func compact(value map[string]any) map[string]any {
	for k, v := range value {
		if v == nil {
			delete(value, k)
		}
	}

	return value
}

// createdAt returns the configured timestamp, or stamps now into v when unset.
func createdAt(v *types.String) string {
	if v.IsNull() || v.IsUnknown() {
		*v = types.StringValue(syntax.DatetimeNow().String())
	}

	return v.ValueString()
}

func optionalString(v types.String) any {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}

	return v.ValueString()
}

func optionalList(v types.List) any {
	if v.IsNull() || v.IsUnknown() || len(v.Elements()) == 0 {
		return nil
	}

	out := make([]string, 0, len(v.Elements()))
	for _, e := range v.Elements() {
		out = append(out, e.(types.String).ValueString())
	}

	return out
}

// loadString reads key from a record, keeping an empty configured string
// when the record omits it so the plan doesn't show a perpetual diff.
func loadString(cur types.String, value map[string]any, key string) types.String {
	if s, ok := value[key].(string); ok {
		return types.StringValue(s)
	}
	if !cur.IsNull() && cur.ValueString() == "" {
		return cur
	}

	return types.StringNull()
}

// loadList reads a string array from a record, keeping an empty configured
// list when the record omits it.
func loadList(cur types.List, value map[string]any, key string) types.List {
	raw, ok := value[key].([]any)
	if !ok {
		if !cur.IsNull() && len(cur.Elements()) == 0 {
			return cur
		}
		return types.ListNull(types.StringType)
	}

	elems := make([]attr.Value, 0, len(raw))
	for _, e := range raw {
		s, _ := e.(string)
		elems = append(elems, types.StringValue(s))
	}

	return types.ListValueMust(types.StringType, elems)
}
