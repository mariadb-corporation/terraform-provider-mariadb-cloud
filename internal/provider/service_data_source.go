package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/skysqlinc/terraform-provider-skysql/internal/skysql"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &ServiceDataSource{}

func NewServiceDataSource() datasource.DataSource {
	return &ServiceDataSource{}
}

// ServiceDataSource defines the data source implementation.
type ServiceDataSource struct {
	client *skysql.Client
}

type ServiceDataSourceModel struct {
	ID                 types.String                     `tfsdk:"service_id"`
	Name               types.String                     `tfsdk:"name"`
	Region             types.String                     `tfsdk:"region"`
	Provider           types.String                     `tfsdk:"cloud_provider"`
	Tier               types.String                     `tfsdk:"tier"`
	Topology           types.String                     `tfsdk:"topology"`
	Version            types.String                     `tfsdk:"version"`
	Architecture       types.String                     `tfsdk:"architecture"`
	Size               types.String                     `tfsdk:"size"`
	Nodes              types.Int64                      `tfsdk:"nodes"`
	SslEnabled         types.Bool                       `tfsdk:"ssl_enabled"`
	NosqlEnabled       types.Bool                       `tfsdk:"nosql_enabled"`
	FQDN               types.String                     `tfsdk:"fqdn"`
	Status             types.String                     `tfsdk:"status"`
	CreatedOn          types.Int64                      `tfsdk:"created_on"`
	UpdatedOn          types.Int64                      `tfsdk:"updated_on"`
	CreatedBy          types.String                     `tfsdk:"created_by"`
	UpdatedBy          types.String                     `tfsdk:"updated_by"`
	Endpoints          []ServiceEndpointDataSourceModel `tfsdk:"endpoints"`
	StorageVolume      *StorageVolumeDataSourceModel    `tfsdk:"storage_volume"`
	OutboundIps        []types.String                   `tfsdk:"outbound_ips"`
	IsActive           types.Bool                       `tfsdk:"is_active"`
	ServiceType        types.String                     `tfsdk:"service_type"`
	ReplicationEnabled types.Bool                       `tfsdk:"replication_enabled"`
	PrimaryHost        types.String                     `tfsdk:"primary_host"`
}

type ServiceEndpointDataSourceModel struct {
	Name            types.String                  `tfsdk:"name"`
	Ports           []EndpointPortDataSourceModel `tfsdk:"ports"`
	Mechanism       types.String                  `tfsdk:"mechanism"`
	AllowedAccounts []types.String                `tfsdk:"allowed_accounts"`
	EndpointService types.String                  `tfsdk:"endpoint_service"`
	Visibility      types.String                  `tfsdk:"visibility"`
}

type EndpointPortDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	Port    types.Int64  `tfsdk:"port"`
	Purpose types.String `tfsdk:"purpose"`
}

type StorageVolumeDataSourceModel struct {
	Size       types.Int64  `tfsdk:"size"`
	VolumeType types.String `tfsdk:"volume_type"`
	IOPS       types.Int64  `tfsdk:"iops"`
	Throughput types.Int64  `tfsdk:"throughput"`
}

func (d *ServiceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (d *ServiceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Returns an full SkySQL service details",
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the service",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "The name of the service",
			},
			"region": schema.StringAttribute{
				Computed:    true,
				Description: "The region where the service is deployed",
			},
			"cloud_provider": schema.StringAttribute{
				Computed:    true,
				Description: "The cloud provider where the service is deployed",
			},
			"tier": schema.StringAttribute{
				Computed:    true,
				Description: "The tier of the service. Possible values are: foundation or power",
			},
			"topology": schema.StringAttribute{
				Computed:    true,
				Description: "The topology of the service. Possible values are: standalone, masterslave, es-single, es-replica, galera, and serverless-standalone",
			},
			"version": schema.StringAttribute{
				Computed:    true,
				Description: "The database service version.",
			},
			"architecture": schema.StringAttribute{
				Computed:    true,
				Description: "The CPU architecture of the service. Possible values are: amd64 or arm64",
			},
			"size": schema.StringAttribute{
				Computed:    true,
				Description: "The size of the service. Possible values are: sky-2x4, sky-2x8 etc",
			},
			"nodes": schema.Int64Attribute{
				Computed:    true,
				Description: "The number of nodes in the service.",
			},
			"ssl_enabled": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether SSL is enabled for the service.",
			},
			"nosql_enabled": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether NoSQL is enabled for the service.",
			},
			"fqdn": schema.StringAttribute{
				Computed:    true,
				Description: "The fully qualified domain name of the service.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "The service status",
			},
			"created_on": schema.Int64Attribute{
				Computed:    true,
				Description: "The timestamp when the service was created.",
			},
			"updated_on": schema.Int64Attribute{
				Computed:    true,
				Description: "The timestamp when the service was last updated.",
			},
			"created_by": schema.StringAttribute{
				Computed:    true,
				Description: "The user who created the service.",
			},
			"updated_by": schema.StringAttribute{
				Computed:    true,
				Description: "The user who last updated the service.",
			},
			"endpoints": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The list of endpoints for the service. Each endpoint has a name and a list of ports. ",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed: true,
						},
						"ports": schema.ListNestedAttribute{
							Computed: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Computed: true,
									},
									"port": schema.Int64Attribute{
										Computed: true,
									},
									"purpose": schema.StringAttribute{
										Computed: true,
									},
								},
							},
						},
						"mechanism": schema.StringAttribute{
							Computed: true,
						},
						"visibility": schema.StringAttribute{
							Computed: true,
						},
						"endpoint_service": schema.StringAttribute{
							Computed: true,
						},
						"allowed_accounts": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
			"storage_volume": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The storage volume for the service.",
				Attributes: map[string]schema.Attribute{
					"size": schema.Int64Attribute{
						Computed:    true,
						Description: "The size of the storage volume in GB.",
					},
					"volume_type": schema.StringAttribute{
						Computed:    true,
						Description: "The type of the storage volume. Possible values are: gp3, io1, io2 etc",
					},
					"iops": schema.Int64Attribute{
						Computed:    true,
						Optional:    true,
						Description: "The number of IOPS for the storage volume. This is only applicable for io1, io2 and gp3 volumes.",
					},
					"throughput": schema.Int64Attribute{
						Computed:    true,
						Optional:    true,
						Description: "The Throughput for the storage volume. This is only applicable for gp3 volumes.",
					},
				},
			},
			"outbound_ips": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "The list of outbound IP addresses for the service.",
			},
			"is_active": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether the service is active.",
			},
			"service_type": schema.StringAttribute{
				Computed:    true,
				Description: "The service type. Possible values: analytical or transactional",
			},
			"replication_enabled": schema.BoolAttribute{
				Computed:    true,
				Description: "Indicates whether replication is enabled for the service.",
			},
			"primary_host": schema.StringAttribute{
				Computed:    true,
				Description: "The primary host for the service. This is only applicable for replication enabled services.",
			},
		},
	}
}

func (d *ServiceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*skysql.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *ServiceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ServiceDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Service ID",
			"service_id is required",
		)
		return
	}

	service, err := d.client.GetServiceByID(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read SkySQL service", err.Error())
		return
	}

	data.ID = types.StringValue(service.ID)
	data.Name = types.StringValue(service.Name)
	data.Region = types.StringValue(service.Region)
	data.Provider = types.StringValue(service.Provider)
	data.Tier = types.StringValue(service.Tier)
	data.Topology = types.StringValue(service.Topology)
	data.Version = types.StringValue(service.Version)
	data.Architecture = types.StringValue(service.Architecture)
	data.Size = types.StringValue(service.Size)
	data.Nodes = types.Int64Value(int64(service.Nodes))
	data.SslEnabled = types.BoolValue(service.SSLEnabled)
	data.NosqlEnabled = types.BoolValue(service.NosqlEnabled)
	data.FQDN = types.StringValue(service.FQDN)
	data.Status = types.StringValue(service.Status)
	data.CreatedOn = types.Int64Value(int64(service.CreatedOn))
	data.UpdatedOn = types.Int64Value(int64(service.UpdatedOn))
	data.CreatedBy = types.StringValue(service.CreatedBy)
	data.UpdatedBy = types.StringValue(service.UpdatedBy)
	data.Endpoints = make([]ServiceEndpointDataSourceModel, len(service.Endpoints))
	for i := range service.Endpoints {
		data.Endpoints[i].Name = types.StringValue(service.Endpoints[i].Name)
		data.Endpoints[i].Mechanism = types.StringValue(service.Endpoints[i].Mechanism)
		data.Endpoints[i].Visibility = types.StringValue(service.Endpoints[i].Visibility)
		data.Endpoints[i].EndpointService = types.StringValue(service.Endpoints[i].EndpointService)
		data.Endpoints[i].Ports = make([]EndpointPortDataSourceModel, len(service.Endpoints[i].Ports))
		for j := range service.Endpoints[i].Ports {
			data.Endpoints[i].Ports[j].Name = types.StringValue(service.Endpoints[i].Ports[j].Name)
			data.Endpoints[i].Ports[j].Port = types.Int64Value(int64(service.Endpoints[i].Ports[j].Port))
			data.Endpoints[i].Ports[j].Purpose = types.StringValue(service.Endpoints[i].Ports[j].Purpose)
		}
		data.Endpoints[i].AllowedAccounts = make([]types.String, len(service.Endpoints[i].AllowedAccounts))
		for a := range service.Endpoints[i].AllowedAccounts {
			data.Endpoints[i].AllowedAccounts[a] = types.StringValue(service.Endpoints[i].AllowedAccounts[a])
		}
	}

	data.StorageVolume = &StorageVolumeDataSourceModel{
		Size:       types.Int64Value(int64(service.StorageVolume.Size)),
		VolumeType: types.StringValue(service.StorageVolume.VolumeType),
		IOPS:       types.Int64Value(int64(service.StorageVolume.IOPS)),
		Throughput: types.Int64Value(int64(service.StorageVolume.Throughput)),
	}

	data.OutboundIps = make([]types.String, len(service.OutboundIps))
	for i := range service.OutboundIps {
		data.OutboundIps[i] = types.StringValue(service.OutboundIps[i])
	}

	data.IsActive = types.BoolValue(service.IsActive)
	data.ServiceType = types.StringValue(service.ServiceType)
	data.ReplicationEnabled = types.BoolValue(service.ReplicationEnabled)
	data.PrimaryHost = types.StringValue(service.PrimaryHost)
	// Set state
	diags := resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
