// Code generated by internal/generate/tags/main.go; DO NOT EDIT.
package rum

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatchrum"
	"github.com/aws/aws-sdk-go/service/cloudwatchrum/cloudwatchrumiface"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
)

// map[string]*string handling

// Tags returns rum service tags.
func Tags(tags tftags.KeyValueTags) map[string]*string {
	return aws.StringMap(tags.Map())
}

// KeyValueTags creates KeyValueTags from rum service tags.
func KeyValueTags(ctx context.Context, tags map[string]*string) tftags.KeyValueTags {
	return tftags.New(ctx, tags)
}

// UpdateTags updates rum service tags.
// The identifier is typically the Amazon Resource Name (ARN), although
// it may also be a different identifier depending on the service.
func UpdateTags(ctx context.Context, conn cloudwatchrumiface.CloudWatchRUMAPI, identifier string, oldTagsMap interface{}, newTagsMap interface{}) error {
	oldTags := tftags.New(ctx, oldTagsMap)
	newTags := tftags.New(ctx, newTagsMap)

	if removedTags := oldTags.Removed(newTags); len(removedTags) > 0 {
		input := &cloudwatchrum.UntagResourceInput{
			ResourceArn: aws.String(identifier),
			TagKeys:     aws.StringSlice(removedTags.IgnoreAWS().Keys()),
		}

		_, err := conn.UntagResourceWithContext(ctx, input)

		if err != nil {
			return fmt.Errorf("untagging resource (%s): %w", identifier, err)
		}
	}

	if updatedTags := oldTags.Updated(newTags); len(updatedTags) > 0 {
		input := &cloudwatchrum.TagResourceInput{
			ResourceArn: aws.String(identifier),
			Tags:        Tags(updatedTags.IgnoreAWS()),
		}

		_, err := conn.TagResourceWithContext(ctx, input)

		if err != nil {
			return fmt.Errorf("tagging resource (%s): %w", identifier, err)
		}
	}

	return nil
}
