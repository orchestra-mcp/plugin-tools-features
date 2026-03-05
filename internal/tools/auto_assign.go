package tools

import (
	"context"

	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
)

// applyAutoAssignment checks for assignment rules matching the feature kind
// and auto-assigns the feature if a matching rule exists. This is best-effort:
// if anything fails, the feature remains unassigned.
func applyAutoAssignment(ctx context.Context, store *storage.FeatureStorage, projectSlug, featureID, kind string) {
	rules, err := store.ListAssignmentRules(ctx, projectSlug)
	if err != nil {
		return
	}
	for _, rule := range rules {
		if rule.Kind == kind {
			feat, body, version, err := store.ReadFeature(ctx, projectSlug, featureID)
			if err != nil {
				return
			}
			feat.Assignee = rule.PersonID
			feat.UpdatedAt = helpers.NowISO()
			store.WriteFeature(ctx, projectSlug, featureID, feat, body, version)
			return // apply first matching rule only
		}
	}
}
