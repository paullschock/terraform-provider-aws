// Code generated by "internal/generate/listpages/main.go -ListOps=ListTrafficPolicyVersions -Paginator=TrafficPolicyVersionMarker -ContextOnly list_traffic_policy_versions_pages_gen.go"; DO NOT EDIT.

package route53

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

func listTrafficPolicyVersionsPages(ctx context.Context, conn route53iface.Route53API, input *route53.ListTrafficPolicyVersionsInput, fn func(*route53.ListTrafficPolicyVersionsOutput, bool) bool) error {
	for {
		output, err := conn.ListTrafficPolicyVersionsWithContext(ctx, input)
		if err != nil {
			return err
		}

		lastPage := aws.StringValue(output.TrafficPolicyVersionMarker) == ""
		if !fn(output, lastPage) || lastPage {
			break
		}

		input.TrafficPolicyVersionMarker = output.TrafficPolicyVersionMarker
	}
	return nil
}
