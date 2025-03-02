package ec2

import (
	"context"
	"log"
	"math/big"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
)

func ResourceSpotInstanceRequest() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceSpotInstanceRequestCreate,
		ReadWithoutTimeout:   resourceSpotInstanceRequestRead,
		DeleteWithoutTimeout: resourceSpotInstanceRequestDelete,
		UpdateWithoutTimeout: resourceSpotInstanceRequestUpdate,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(20 * time.Minute),
		},

		Schema: func() map[string]*schema.Schema {
			// The Spot Instance Request Schema is based on the AWS Instance schema.
			s := ResourceInstance().Schema

			// Everything on a spot instance is ForceNew except tags
			for k, v := range s {
				if k == "tags" || k == "tags_all" {
					continue
				}
				v.ForceNew = true
			}

			s["volume_tags"] = &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			}

			s["spot_price"] = &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					oldFloat, _ := strconv.ParseFloat(old, 64)
					newFloat, _ := strconv.ParseFloat(new, 64)

					return big.NewFloat(oldFloat).Cmp(big.NewFloat(newFloat)) == 0
				},
			}
			s["spot_type"] = &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				Default:      ec2.SpotInstanceTypePersistent,
				ValidateFunc: validation.StringInSlice(ec2.SpotInstanceType_Values(), false),
			}
			s["wait_for_fulfillment"] = &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			}
			s["launch_group"] = &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			}
			s["spot_bid_status"] = &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			}
			s["spot_request_state"] = &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			}
			s["spot_instance_id"] = &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			}
			s["block_duration_minutes"] = &schema.Schema{
				Type:         schema.TypeInt,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.IntDivisibleBy(60),
			}
			s["instance_interruption_behavior"] = &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				Default:      ec2.InstanceInterruptionBehaviorTerminate,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice(ec2.InstanceInterruptionBehavior_Values(), false),
			}
			s["valid_from"] = &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.IsRFC3339Time,
				Computed:     true,
			}
			s["valid_until"] = &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validation.IsRFC3339Time,
				Computed:     true,
			}
			return s
		}(),

		CustomizeDiff: customdiff.All(
			verify.SetTagsDiff,
		),
	}
}

func resourceSpotInstanceRequestCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()
	defaultTagsConfig := meta.(*conns.AWSClient).DefaultTagsConfig
	tags := defaultTagsConfig.MergeTags(tftags.New(ctx, d.Get("tags").(map[string]interface{})))

	instanceOpts, err := buildInstanceOpts(ctx, d, meta)
	if err != nil {
		return sdkdiag.AppendErrorf(diags, "requesting EC2 Spot Instances: %s", err)
	}

	spotOpts := &ec2.RequestSpotInstancesInput{
		ClientToken: aws.String(resource.UniqueId()),
		// Though the AWS API supports creating spot instance requests for multiple
		// instances, for TF purposes we fix this to one instance per request.
		// Users can get equivalent behavior out of TF's "count" meta-parameter.
		InstanceCount:                aws.Int64(1),
		InstanceInterruptionBehavior: aws.String(d.Get("instance_interruption_behavior").(string)),
		LaunchSpecification: &ec2.RequestSpotLaunchSpecification{
			BlockDeviceMappings: instanceOpts.BlockDeviceMappings,
			EbsOptimized:        instanceOpts.EBSOptimized,
			Monitoring:          instanceOpts.Monitoring,
			IamInstanceProfile:  instanceOpts.IAMInstanceProfile,
			ImageId:             instanceOpts.ImageID,
			InstanceType:        instanceOpts.InstanceType,
			KeyName:             instanceOpts.KeyName,
			SecurityGroupIds:    instanceOpts.SecurityGroupIDs,
			SecurityGroups:      instanceOpts.SecurityGroups,
			SubnetId:            instanceOpts.SubnetID,
			UserData:            instanceOpts.UserData64,
			NetworkInterfaces:   instanceOpts.NetworkInterfaces,
		},
		SpotPrice:         aws.String(d.Get("spot_price").(string)),
		TagSpecifications: tagSpecificationsFromKeyValueTags(tags, ec2.ResourceTypeSpotInstancesRequest),
		Type:              aws.String(d.Get("spot_type").(string)),
	}

	if v, ok := d.GetOk("block_duration_minutes"); ok {
		spotOpts.BlockDurationMinutes = aws.Int64(int64(v.(int)))
	}

	if v, ok := d.GetOk("launch_group"); ok {
		spotOpts.LaunchGroup = aws.String(v.(string))
	}

	if v, ok := d.GetOk("valid_from"); ok {
		validFrom, err := time.Parse(time.RFC3339, v.(string))
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "requesting EC2 Spot Instances: %s", err)
		}
		spotOpts.ValidFrom = aws.Time(validFrom)
	}

	if v, ok := d.GetOk("valid_until"); ok {
		validUntil, err := time.Parse(time.RFC3339, v.(string))
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "requesting EC2 Spot Instances: %s", err)
		}
		spotOpts.ValidUntil = aws.Time(validUntil)
	}

	// Placement GroupName can only be specified when instanceInterruptionBehavior is not set or set to 'terminate'
	if v, exists := d.GetOkExists("instance_interruption_behavior"); v.(string) == ec2.InstanceInterruptionBehaviorTerminate || !exists {
		spotOpts.LaunchSpecification.Placement = instanceOpts.SpotPlacement
	}

	// Make the spot instance request
	var resp *ec2.RequestSpotInstancesOutput
	err = resource.RetryContext(ctx, propagationTimeout, func() *resource.RetryError {
		resp, err = conn.RequestSpotInstancesWithContext(ctx, spotOpts)
		// IAM instance profiles can take ~10 seconds to propagate in AWS:
		// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-roles-for-amazon-ec2.html#launch-instance-with-role-console
		if tfawserr.ErrMessageContains(err, "InvalidParameterValue", "Invalid IAM Instance Profile") {
			log.Printf("[DEBUG] Invalid IAM Instance Profile referenced, retrying...")
			return resource.RetryableError(err)
		}
		// IAM roles can also take time to propagate in AWS:
		if tfawserr.ErrMessageContains(err, "InvalidParameterValue", " has no associated IAM Roles") {
			log.Printf("[DEBUG] IAM Instance Profile appears to have no IAM roles, retrying...")
			return resource.RetryableError(err)
		}
		if err != nil {
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if tfresource.TimedOut(err) {
		resp, err = conn.RequestSpotInstancesWithContext(ctx, spotOpts)
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "requesting EC2 Spot Instances: %s", err)
	}
	if len(resp.SpotInstanceRequests) != 1 {
		return sdkdiag.AppendErrorf(diags, "Expected response with length 1, got: %s", resp)
	}

	sir := resp.SpotInstanceRequests[0]
	d.SetId(aws.StringValue(sir.SpotInstanceRequestId))

	if d.Get("wait_for_fulfillment").(bool) {
		spotStateConf := &resource.StateChangeConf{
			// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-bid-status.html
			Pending:    []string{"start", "pending-evaluation", "pending-fulfillment"},
			Target:     []string{"fulfilled"},
			Refresh:    SpotInstanceStateRefreshFunc(ctx, conn, sir),
			Timeout:    d.Timeout(schema.TimeoutCreate),
			Delay:      10 * time.Second,
			MinTimeout: 3 * time.Second,
		}

		log.Printf("[DEBUG] waiting for spot bid to resolve... this may take several minutes.")
		_, err = spotStateConf.WaitForStateContext(ctx)

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "Error while waiting for spot request (%s) to resolve: %s", sir, err)
		}
	}

	return append(diags, resourceSpotInstanceRequestRead(ctx, d, meta)...)
}

// Update spot state, etc
func resourceSpotInstanceRequestRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()
	defaultTagsConfig := meta.(*conns.AWSClient).DefaultTagsConfig
	ignoreTagsConfig := meta.(*conns.AWSClient).IgnoreTagsConfig

	outputRaw, err := tfresource.RetryWhenNewResourceNotFound(ctx, propagationTimeout, func() (interface{}, error) {
		return FindSpotInstanceRequestByID(ctx, conn, d.Id())
	}, d.IsNewResource())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] EC2 Spot Instance Request (%s) not found, removing from state", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading EC2 Spot Instance Request (%s): %s", d.Id(), err)
	}

	request := outputRaw.(*ec2.SpotInstanceRequest)

	d.Set("spot_bid_status", request.Status.Code)
	// Instance ID is not set if the request is still pending
	if request.InstanceId != nil {
		d.Set("spot_instance_id", request.InstanceId)
		// Read the instance data, setting up connection information
		diags = append(diags, sdkdiag.WrapDiagsf(readInstance(ctx, d, meta), "reading EC2 Spot Instance Request (%s)", d.Id())...)
		if diags.HasError() {
			return diags
		}
	}

	d.Set("spot_request_state", request.State)
	d.Set("launch_group", request.LaunchGroup)
	d.Set("block_duration_minutes", request.BlockDurationMinutes)

	tags := KeyValueTags(ctx, request.Tags).IgnoreAWS().IgnoreConfig(ignoreTagsConfig)

	//lintignore:AWSR002
	if err := d.Set("tags", tags.RemoveDefaultConfig(defaultTagsConfig).Map()); err != nil {
		return sdkdiag.AppendErrorf(diags, "setting tags: %s", err)
	}

	if err := d.Set("tags_all", tags.Map()); err != nil {
		return sdkdiag.AppendErrorf(diags, "setting tags_all: %s", err)
	}

	d.Set("instance_interruption_behavior", request.InstanceInterruptionBehavior)
	d.Set("valid_from", aws.TimeValue(request.ValidFrom).Format(time.RFC3339))
	d.Set("valid_until", aws.TimeValue(request.ValidUntil).Format(time.RFC3339))
	d.Set("spot_type", request.Type)
	d.Set("spot_price", request.SpotPrice)
	d.Set("key_name", request.LaunchSpecification.KeyName)
	d.Set("instance_type", request.LaunchSpecification.InstanceType)
	d.Set("ami", request.LaunchSpecification.ImageId)

	return diags
}

func readInstance(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()

	instance, err := FindInstanceByID(ctx, conn, d.Get("spot_instance_id").(string))

	if err != nil {
		return sdkdiag.AppendFromErr(diags, err)
	}

	d.Set("public_dns", instance.PublicDnsName)
	d.Set("public_ip", instance.PublicIpAddress)
	d.Set("private_dns", instance.PrivateDnsName)
	d.Set("private_ip", instance.PrivateIpAddress)
	d.Set("source_dest_check", instance.SourceDestCheck)

	// set connection information
	if instance.PublicIpAddress != nil {
		d.SetConnInfo(map[string]string{
			"type": "ssh",
			"host": *instance.PublicIpAddress,
		})
	} else if instance.PrivateIpAddress != nil {
		d.SetConnInfo(map[string]string{
			"type": "ssh",
			"host": *instance.PrivateIpAddress,
		})
	}
	if err := readBlockDevices(ctx, d, instance, conn); err != nil {
		return sdkdiag.AppendFromErr(diags, err)
	}

	var ipv6Addresses []string
	if len(instance.NetworkInterfaces) > 0 {
		for _, ni := range instance.NetworkInterfaces {
			if aws.Int64Value(ni.Attachment.DeviceIndex) == 0 {
				d.Set("subnet_id", ni.SubnetId)
				d.Set("primary_network_interface_id", ni.NetworkInterfaceId)
				d.Set("associate_public_ip_address", ni.Association != nil)
				d.Set("ipv6_address_count", len(ni.Ipv6Addresses))

				for _, address := range ni.Ipv6Addresses {
					ipv6Addresses = append(ipv6Addresses, *address.Ipv6Address)
				}
			}
		}
	} else {
		d.Set("subnet_id", instance.SubnetId)
		d.Set("primary_network_interface_id", "")
	}

	if err := d.Set("ipv6_addresses", ipv6Addresses); err != nil {
		log.Printf("[WARN] Error setting ipv6_addresses for AWS Spot Instance (%s): %s", d.Id(), err)
	}

	if err := readSecurityGroups(ctx, d, instance, conn); err != nil {
		return sdkdiag.AppendErrorf(diags, "reading EC2 Instance (%s): %s", aws.StringValue(instance.InstanceId), err)
	}

	if d.Get("get_password_data").(bool) {
		passwordData, err := getInstancePasswordData(ctx, *instance.InstanceId, conn)
		if err != nil {
			return sdkdiag.AppendFromErr(diags, err)
		}
		d.Set("password_data", passwordData)
	} else {
		d.Set("get_password_data", false)
		d.Set("password_data", nil)
	}

	return diags
}

func resourceSpotInstanceRequestUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()

	if d.HasChange("tags_all") {
		o, n := d.GetChange("tags_all")

		if err := UpdateTags(ctx, conn, d.Id(), o, n); err != nil {
			return sdkdiag.AppendErrorf(diags, "updating EC2 Spot Instance Request (%s) tags: %s", d.Id(), err)
		}
	}

	return append(diags, resourceSpotInstanceRequestRead(ctx, d, meta)...)
}

func resourceSpotInstanceRequestDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()

	log.Printf("[INFO] Cancelling spot request: %s", d.Id())
	_, err := conn.CancelSpotInstanceRequestsWithContext(ctx, &ec2.CancelSpotInstanceRequestsInput{
		SpotInstanceRequestIds: []*string{aws.String(d.Id())},
	})

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "Error cancelling spot request (%s): %s", d.Id(), err)
	}

	if instanceId := d.Get("spot_instance_id").(string); instanceId != "" {
		if err := terminateInstance(ctx, conn, instanceId, d.Timeout(schema.TimeoutDelete)); err != nil {
			return sdkdiag.AppendFromErr(diags, err)
		}
	}

	return diags
}

// SpotInstanceStateRefreshFunc returns a resource.StateRefreshFunc that is used to watch
// an EC2 spot instance request
func SpotInstanceStateRefreshFunc(ctx context.Context, conn *ec2.EC2, sir *ec2.SpotInstanceRequest) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, err := conn.DescribeSpotInstanceRequestsWithContext(ctx, &ec2.DescribeSpotInstanceRequestsInput{
			SpotInstanceRequestIds: []*string{sir.SpotInstanceRequestId},
		})

		if err != nil {
			if tfawserr.ErrCodeEquals(err, "InvalidSpotInstanceRequestID.NotFound") {
				// Set this to nil as if we didn't find anything.
				resp = nil
			} else {
				log.Printf("Error on StateRefresh: %s", err)
				return nil, "", err
			}
		}

		if resp == nil || len(resp.SpotInstanceRequests) == 0 {
			// Sometimes AWS just has consistency issues and doesn't see
			// our request yet. Return an empty state.
			return nil, "", nil
		}

		req := resp.SpotInstanceRequests[0]
		return req, *req.Status.Code, nil
	}
}
