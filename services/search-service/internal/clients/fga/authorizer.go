package fga

import (
	"context"
	"fmt"
	"strings"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/platform-mesh/golang-commons/fga/helpers"
	"github.com/platform-mesh/golang-commons/fga/util"

	"github.com/platform-mesh/search/internal/service/search"
)

const batchCheckChunkSize = 100

type Authorizer struct {
	client openfgav1.OpenFGAServiceClient
}

func NewAuthorizer(client openfgav1.OpenFGAServiceClient) *Authorizer {
	return &Authorizer{client: client}
}

func (a *Authorizer) FilterAuthorized(ctx context.Context, req search.AuthorizationRequest) (search.AuthorizationResult, error) {
	allowed := make([]bool, len(req.Hits))
	if len(req.Hits) == 0 {
		return search.AuthorizationResult{Allowed: allowed}, nil
	}

	storeID, err := helpers.GetStoreIDForTenant(ctx, a.client, req.Organization)
	if err != nil {
		return search.AuthorizationResult{}, fmt.Errorf("resolve store ID: %w", err)
	}

	result := search.AuthorizationResult{Allowed: allowed}

	for _, chunk := range chunkRanges(len(req.Hits), batchCheckChunkSize) {
		start := chunk[0]
		end := chunk[1]
		items := make([]*openfgav1.BatchCheckItem, 0, end-start)
		indicesByCorrelation := make(map[string]int, end-start)

		for idx := start; idx < end; idx++ {
			item, missingContext := buildBatchCheckItem(req.User, req.Relation, idx, req.Hits[idx])
			if missingContext {
				result.DroppedMissingContext++
				continue
			}
			items = append(items, item)
			indicesByCorrelation[item.CorrelationId] = idx
		}

		if len(items) == 0 {
			continue
		}

		batchResponse, err := a.client.BatchCheck(ctx, &openfgav1.BatchCheckRequest{
			StoreId: storeID,
			Checks:  items,
		})
		if err != nil {
			return search.AuthorizationResult{}, fmt.Errorf("openfga batch check: %w", err)
		}
		result.Calls++

		for correlationID, index := range indicesByCorrelation {
			entry, ok := batchResponse.GetResult()[correlationID]
			if !ok {
				result.Denied++
				continue
			}
			if entry.GetAllowed() {
				result.Allowed[index] = true
				continue
			}
			result.Denied++
		}
	}

	return result, nil
}

func chunkRanges(total, chunkSize int) [][2]int {
	if total <= 0 || chunkSize <= 0 {
		return nil
	}

	ranges := make([][2]int, 0, (total+chunkSize-1)/chunkSize)
	for start := 0; start < total; start += chunkSize {
		end := start + chunkSize
		if end > total {
			end = total
		}
		ranges = append(ranges, [2]int{start, end})
	}

	return ranges
}

func buildBatchCheckItem(user, relation string, index int, hit search.OpenSearchHit) (*openfgav1.BatchCheckItem, bool) {
	ctx, ok := buildAuthorizationContext(hit.Source)
	if !ok {
		return nil, true
	}

	tupleKey := &openfgav1.CheckRequestTupleKey{
		User:     fmt.Sprintf("user:%s", user),
		Relation: relation,
		Object:   ctx.object,
	}

	return &openfgav1.BatchCheckItem{
		TupleKey:         tupleKey,
		ContextualTuples: &openfgav1.ContextualTupleKeys{TupleKeys: ctx.contextualTuples},
		CorrelationId:    fmt.Sprintf("%d", index),
	}, false
}

type authzContext struct {
	object           string
	contextualTuples []*openfgav1.TupleKey
}

func buildAuthorizationContext(source map[string]interface{}) (authzContext, bool) {
	if source == nil {
		return authzContext{}, false
	}

	kind := readString(source, "kind")
	name := readString(source, "name")
	namespace := readString(source, "namespace")
	apiGroup := readString(source, "api_group")
	clusterName := readString(source, "cluster_name")
	organizationID := readString(source, "organization_id")
	accountID := readString(source, "account_id")
	accountName := readString(source, "account_name")

	clusterID := firstNonEmpty(accountID, organizationID)
	if clusterID == "" && !strings.Contains(clusterName, ":") {
		clusterID = clusterName
	}

	if kind == "" || name == "" || clusterID == "" {
		return authzContext{}, false
	}

	if namespace != "" {
		if accountName == "" {
			return authzContext{}, false
		}
		if firstNonEmpty(accountID, organizationID, clusterID) == "" {
			return authzContext{}, false
		}
	}

	resourceType := util.ConvertToTypeName(apiGroup, kind)
	object := fmt.Sprintf("%s:%s/%s", resourceType, clusterID, name)
	if namespace != "" {
		object = fmt.Sprintf("%s:%s/%s/%s", resourceType, clusterID, namespace, name)
	}

	parentAccountCluster := firstNonEmpty(accountID, organizationID, clusterID)
	accountObject := ""
	if accountName != "" && parentAccountCluster != "" {
		accountType := util.ConvertToTypeName("core.platform-mesh.io", "Account")
		accountObject = fmt.Sprintf("%s:%s/%s", accountType, parentAccountCluster, accountName)
	}

	tuples := make([]*openfgav1.TupleKey, 0, 2)
	resourceManaged := managedTuple(apiGroup, kind)

	if namespace != "" && accountObject != "" {
		namespaceType := util.ConvertToTypeName("", "Namespace")
		namespaceObject := fmt.Sprintf("%s:%s/%s", namespaceType, clusterID, namespace)

		tuples = append(tuples, &openfgav1.TupleKey{
			Object:   namespaceObject,
			Relation: "parent",
			User:     accountObject,
		})

		if !resourceManaged {
			tuples = append(tuples, &openfgav1.TupleKey{
				Object:   object,
				Relation: "parent",
				User:     namespaceObject,
			})
		}
	} else if accountObject != "" && !resourceManaged {
		tuples = append(tuples, &openfgav1.TupleKey{
			Object:   object,
			Relation: "parent",
			User:     accountObject,
		})
	}

	return authzContext{object: object, contextualTuples: tuples}, true
}

func managedTuple(group, kind string) bool {
	if strings.EqualFold(group, "core.platform-mesh.io") && strings.EqualFold(kind, "Account") {
		return true
	}
	return false
}

func readString(source map[string]interface{}, key string) string {
	v, ok := source[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
