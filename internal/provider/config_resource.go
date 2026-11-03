package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql"
	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql/provisioning"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ConfigResource{}
var _ resource.ResourceWithImportState = &ConfigResource{}
var _ resource.ResourceWithConfigure = &ConfigResource{}

func NewConfigResource() resource.Resource {
	return &ConfigResource{}
}

// ConfigResource defines the resource implementation.
type ConfigResource struct {
	client *skysql.Client
}

// ConfigResourceModel describes the resource data model.
type ConfigResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Topology      types.String `tfsdk:"topology"`
	Version       types.String `tfsdk:"version"`
	TopologyID    types.String `tfsdk:"topology_id"`
	VersionID     types.String `tfsdk:"version_id"`
	AllowRestart  types.Bool   `tfsdk:"allow_restart"`
	Values        types.Map    `tfsdk:"values"`
	ReplicaValues types.Map    `tfsdk:"replica_values"`
}

func (r *ConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *ConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates and manages a custom configuration object in SkySQL. " +
			"A configuration object holds MariaDB server variable overrides that can be applied to one or more services. " +
			"Use the `values` attribute to set server variables by name.",
		MarkdownDescription: "Creates and manages a custom configuration object in SkySQL.\n\n" +
			"A configuration object holds MariaDB server variable overrides that can be applied to one or more services. " +
			"Use the `values` attribute to set server variables by name.\n\n" +
			"Supported topologies: `es-single` (standalone), `es-replica` (primary with replicas), `galera` (Galera cluster).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "The unique identifier for the configuration object.",
				MarkdownDescription: "The unique identifier for the configuration object.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The name of the configuration object. Must be unique within the organization.",
				MarkdownDescription: "The name of the configuration object. Must be unique within the organization.",
			},
			"topology": schema.StringAttribute{
				Required:            true,
				Description:         "The topology name (e.g. es-single, es-replica, galera). Determines which server variables are available.",
				MarkdownDescription: "The topology name (e.g. `es-single`, `es-replica`, `galera`). Determines which server variables are available.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				Required:            true,
				Description:         "The MariaDB server version (e.g. 10.6.7-3-1). Must match an available version for the topology.",
				MarkdownDescription: "The MariaDB server version (e.g. `10.6.7-3-1`). Must match an available version for the topology.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"topology_id": schema.StringAttribute{
				Computed:            true,
				Description:         "The resolved topology UUID.",
				MarkdownDescription: "The resolved topology UUID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version_id": schema.StringAttribute{
				Computed:            true,
				Description:         "The resolved version UUID.",
				MarkdownDescription: "The resolved version UUID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_restart": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Whether to allow configuration values that require a service restart. When false (the default), setting any variable that has requires_restart = true in the DPS parameter catalog will be rejected both client-side and server-side. Set to true to permit restart-causing variables. The parameter is forwarded to DPS as ?allow_restart=true on config value API calls.",
				MarkdownDescription: "Whether to allow configuration values that require a service restart. " +
					"When `false` (the default), setting any variable that has `requires_restart = true` in the DPS parameter catalog will be rejected " +
					"both client-side (before the API call) and server-side (by DPS). " +
					"Set to `true` to permit restart-causing variables. The parameter is forwarded to DPS as `?allow_restart=true` on config value API calls.",
			},
			"values": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				Description:         "A map of MariaDB server variable names to their values (e.g. max_connections = \"500\").",
				MarkdownDescription: "A map of MariaDB server variable names to their values (e.g. `max_connections = \"500\"`).",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"replica_values": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A map of MariaDB server variable names to the value that the replicas use. " +
					"A variable that is not in the map has the value of the primary on the replicas too. " +
					"Only for the es-replica topology, and only for the variables that the key list marks with replica_configurable. " +
					"To make the replicas follow the primary again, remove the variable from the map.",
				MarkdownDescription: "A map of MariaDB server variable names to the value that the replicas use " +
					"(e.g. `sync_binlog = \"100\"`). A variable that is not in the map has the value of the primary on the replicas too. " +
					"Only for the `es-replica` topology, and only for the variables that the key list marks with `replica_configurable`. " +
					"To make the replicas follow the primary again, remove the variable from the map. " +
					"The variable does not have to be in `values`.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*skysql.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *skysql.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// checkRestartValues fetches config keys for the given topology/version and returns
// the names of any values that require a service restart.
func (r *ConfigResource) checkRestartValues(ctx context.Context, topologyName string, version string, valueNames []string) ([]string, error) {
	keys, err := r.client.GetConfigKeysByTopology(ctx, topologyName, version)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch config keys for topology %q: %w", topologyName, err)
	}

	// Build a lookup of key name → requires_restart.
	keyRestartMap := make(map[string]bool, len(keys))
	for _, k := range keys {
		keyRestartMap[k.Name] = k.RequiresRestart
	}

	var restartVars []string
	for _, name := range valueNames {
		if keyRestartMap[name] {
			restartVars = append(restartVars, name)
		}
	}
	sort.Strings(restartVars)
	return restartVars, nil
}

func (r *ConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate allow_restart before creating the config.
	if !data.Values.IsNull() && !data.Values.IsUnknown() && !data.AllowRestart.ValueBool() {
		values := make(map[string]string)
		resp.Diagnostics.Append(data.Values.ElementsAs(ctx, &values, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}

		restartVars, err := r.checkRestartValues(ctx, data.Topology.ValueString(), data.Version.ValueString(), names)
		if err != nil {
			resp.Diagnostics.AddError("Error checking config key restart requirements", err.Error())
			return
		}

		if len(restartVars) > 0 {
			resp.Diagnostics.AddError(
				"Configuration contains variables that require a service restart",
				fmt.Sprintf(
					"The following variables require a service restart: %s. "+
						"Set allow_restart = true on the skysql_config resource to permit restart-causing variables.",
					strings.Join(restartVars, ", "),
				),
			)
			return
		}
	}

	// Create the config using name-based topology/version resolution.
	createReq := &provisioning.CreateConfigRequest{
		Name:     data.Name.ValueString(),
		Topology: data.Topology.ValueString(),
		Version:  data.Version.ValueString(),
	}

	config, err := r.client.CreateConfig(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating configuration", err.Error())
		return
	}

	data.ID = types.StringValue(config.ID)
	data.TopologyID = types.StringValue(config.TopologyID)
	data.VersionID = types.StringValue(config.VersionID)

	// Set values if provided (sorted for deterministic ordering).
	if !data.Values.IsNull() && !data.Values.IsUnknown() {
		values := make(map[string]string)
		resp.Diagnostics.Append(data.Values.ElementsAs(ctx, &values, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			if err := r.client.SetConfigValue(ctx, config.ID, name, values[name], data.AllowRestart.ValueBool()); err != nil {
				resp.Diagnostics.AddError(
					"Error setting config value",
					fmt.Sprintf("Failed to set %q = %q: %s", name, values[name], err.Error()),
				)
				return
			}
		}
	}

	// Set the replica values if provided.
	if !data.ReplicaValues.IsNull() && !data.ReplicaValues.IsUnknown() {
		replicaValues := make(map[string]string)
		resp.Diagnostics.Append(data.ReplicaValues.ElementsAs(ctx, &replicaValues, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		resp.Diagnostics.Append(r.applyReplicaValues(ctx, config.ID, nil, replicaValues)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	tflog.Trace(ctx, "created config resource", map[string]interface{}{
		"id":   config.ID,
		"name": config.Name,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.client.GetConfigByID(ctx, data.ID.ValueString())
	if err != nil {
		if errors.Is(err, skysql.ErrorServiceNotFound) {
			tflog.Warn(ctx, "SkySQL config not found, removing from state", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading configuration", err.Error())
		return
	}

	data.Name = types.StringValue(config.Name)
	data.TopologyID = types.StringValue(config.TopologyID)
	data.VersionID = types.StringValue(config.VersionID)

	// Values are not readable from the API by variable name,
	// so we preserve whatever is in state.

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ConfigResourceModel
	var state ConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	configID := state.ID.ValueString()

	// Update name if changed.
	if !plan.Name.Equal(state.Name) {
		_, err := r.client.UpdateConfig(ctx, configID, &provisioning.UpdateConfigRequest{
			Name: plan.Name.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Error updating configuration name", err.Error())
			return
		}
	}

	// Diff values and apply changes.
	oldValues := make(map[string]string)
	newValues := make(map[string]string)

	if !state.Values.IsNull() && !state.Values.IsUnknown() {
		resp.Diagnostics.Append(state.Values.ElementsAs(ctx, &oldValues, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if !plan.Values.IsNull() && !plan.Values.IsUnknown() {
		resp.Diagnostics.Append(plan.Values.ElementsAs(ctx, &newValues, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Validate allow_restart for new or changed values.
	if !plan.AllowRestart.ValueBool() {
		changed := make([]string, 0)
		for name, newVal := range newValues {
			if oldVal, exists := oldValues[name]; !exists || oldVal != newVal {
				changed = append(changed, name)
			}
		}

		if len(changed) > 0 {
			restartVars, err := r.checkRestartValues(ctx, plan.Topology.ValueString(), plan.Version.ValueString(), changed)
			if err != nil {
				resp.Diagnostics.AddError("Error checking config key restart requirements", err.Error())
				return
			}

			if len(restartVars) > 0 {
				resp.Diagnostics.AddError(
					"Configuration contains variables that require a service restart",
					fmt.Sprintf(
						"The following variables require a service restart: %s. "+
							"Set allow_restart = true on the skysql_config resource to permit restart-causing variables.",
						strings.Join(restartVars, ", "),
					),
				)
				return
			}
		}
	}

	// Unset removed values (sorted for deterministic ordering).
	removed := make([]string, 0)
	for name := range oldValues {
		if _, exists := newValues[name]; !exists {
			removed = append(removed, name)
		}
	}
	sort.Strings(removed)

	for _, name := range removed {
		if err := r.client.UnsetConfigValue(ctx, configID, name, plan.AllowRestart.ValueBool()); err != nil {
			resp.Diagnostics.AddError(
				"Error unsetting config value",
				fmt.Sprintf("Failed to unset %q: %s", name, err.Error()),
			)
			return
		}
	}

	// Set new or changed values (sorted for deterministic ordering).
	changed := make([]string, 0)
	for name, newVal := range newValues {
		if oldVal, exists := oldValues[name]; !exists || oldVal != newVal {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)

	for _, name := range changed {
		if err := r.client.SetConfigValue(ctx, configID, name, newValues[name], plan.AllowRestart.ValueBool()); err != nil {
			resp.Diagnostics.AddError(
				"Error setting config value",
				fmt.Sprintf("Failed to set %q = %q: %s", name, newValues[name], err.Error()),
			)
			return
		}
	}

	// Diff the replica values and apply the changes.
	oldReplica := make(map[string]string)
	newReplica := make(map[string]string)

	if !state.ReplicaValues.IsNull() && !state.ReplicaValues.IsUnknown() {
		resp.Diagnostics.Append(state.ReplicaValues.ElementsAs(ctx, &oldReplica, false)...)
	}

	if !plan.ReplicaValues.IsNull() && !plan.ReplicaValues.IsUnknown() {
		resp.Diagnostics.Append(plan.ReplicaValues.ElementsAs(ctx, &newReplica, false)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.applyReplicaValues(ctx, configID, oldReplica, newReplica)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Refresh state from plan.
	plan.ID = state.ID
	plan.TopologyID = state.TopologyID
	plan.VersionID = state.VersionID

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteConfig(ctx, data.ID.ValueString())
	if err != nil {
		if errors.Is(err, skysql.ErrorServiceNotFound) {
			tflog.Warn(ctx, "SkySQL config already deleted", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			return
		}
		resp.Diagnostics.AddError("Error deleting configuration", err.Error())
		return
	}

	tflog.Trace(ctx, "deleted config resource", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

func (r *ConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyReplicaValues moves the replica values of a config from the old map to the new one. A
// variable that left the map is unset, so that the replicas follow the primary again. A new or
// changed variable is set. Both lists are sorted, so that the order of the calls is stable.
func (r *ConfigResource) applyReplicaValues(ctx context.Context, configID string, oldValues, newValues map[string]string) diag.Diagnostics {
	var diags diag.Diagnostics

	removed := make([]string, 0)
	for name := range oldValues {
		if _, exists := newValues[name]; !exists {
			removed = append(removed, name)
		}
	}
	sort.Strings(removed)

	for _, name := range removed {
		if err := r.client.UnsetConfigReplicaValue(ctx, configID, name); err != nil {
			diags.AddError(
				"Error unsetting replica config value",
				replicaErrorDetail(fmt.Sprintf("Failed to unset the replica value of %q", name), err),
			)
			return diags
		}
	}

	changed := make([]string, 0)
	for name, newVal := range newValues {
		if oldVal, exists := oldValues[name]; !exists || oldVal != newVal {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)

	for _, name := range changed {
		if err := r.client.SetConfigReplicaValue(ctx, configID, name, newValues[name]); err != nil {
			diags.AddError(
				"Error setting replica config value",
				replicaErrorDetail(fmt.Sprintf("Failed to set the replica value of %q = %q", name, newValues[name]), err),
			)
			return diags
		}
	}

	return diags
}

// replicaErrorDetail writes the message of a failed replica value call. DPS answers 404 to every
// replica value call when the replica values are not turned on, and that looks the same as a
// missing config, so the message names both.
func replicaErrorDetail(prefix string, err error) string {
	if errors.Is(err, skysql.ErrorServiceNotFound) {
		return prefix + ": the configuration was not found, or the replica values are not available yet in this SkySQL environment."
	}

	return prefix + ": " + err.Error()
}
