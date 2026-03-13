package kcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/platform-mesh/golang-commons/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/platform-mesh/search/internal/config"
	"github.com/platform-mesh/search/internal/service/search"
)

type SearchIndexResolver struct {
	client dynamic.Interface
	cfg    config.SearchIndexConfig
	log    *logger.Logger
}

const searchIndexOrgClusterIDLabel = "search.platform-mesh.io/org-cluster-id"

type searchIndexResource struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		OrganizationClusterID string `json:"organizationClusterID"`
	} `json:"spec"`
	Status struct {
		IndexName string `json:"indexName"`
	} `json:"status"`
}

func NewSearchIndexResolver(restCfg *rest.Config, cfg config.SearchIndexConfig, log *logger.Logger) (*SearchIndexResolver, error) {
	scopedCfg, err := configForKCPCluster(cfg.WorkspacePath, restCfg)
	if err != nil {
		return nil, fmt.Errorf("create KCP workspace config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(scopedCfg)
	if err != nil {
		return nil, fmt.Errorf("create KCP dynamic client: %w", err)
	}

	return &SearchIndexResolver{
		client: dynamicClient,
		cfg:    cfg,
		log:    log,
	}, nil
}

func (r *SearchIndexResolver) ResolveIndex(ctx context.Context, org string) (search.SearchIndexRef, error) {
	org = strings.TrimSpace(org)
	if org == "" {
		return search.SearchIndexRef{}, fmt.Errorf("organization is required")
	}

	orgClusterID, err := r.resolveOrganizationClusterID(ctx, org)
	if err != nil {
		return search.SearchIndexRef{}, err
	}

	list, err := r.listSearchIndices(ctx, orgClusterID)
	if err != nil {
		return search.SearchIndexRef{}, fmt.Errorf("list SearchIndex resources: %w", err)
	}
	// Backward-compatible fallback for SearchIndex resources created before
	// org-cluster-id labels were introduced.
	if len(list.Items) == 0 {
		list, err = r.listSearchIndices(ctx, "")
		if err != nil {
			return search.SearchIndexRef{}, fmt.Errorf("list SearchIndex resources (fallback): %w", err)
		}
	}
	if len(list.Items) == 0 {
		return search.SearchIndexRef{}, fmt.Errorf(
			"no SearchIndex resources found in workspace %q",
			r.cfg.WorkspacePath,
		)
	}

	selected, err := selectSearchIndex(org, orgClusterID, list.Items)
	if err != nil {
		return search.SearchIndexRef{}, err
	}

	indexName := strings.TrimSpace(selected.Status.IndexName)
	if indexName == "" {
		indexName = strings.TrimSpace(selected.Spec.OrganizationClusterID)
	}
	if indexName == "" {
		return search.SearchIndexRef{}, fmt.Errorf("searchindex %q has neither status.indexName nor spec.organizationClusterID", selected.Metadata.Name)
	}

	return search.SearchIndexRef{
		IndexName:             indexName,
		OrganizationClusterID: firstNonEmpty(strings.TrimSpace(selected.Spec.OrganizationClusterID), orgClusterID),
		Group:                 r.cfg.Group,
		Version:               r.cfg.Version,
	}, nil
}

func (r *SearchIndexResolver) resolveOrganizationClusterID(ctx context.Context, org string) (string, error) {
	obj, err := r.client.Resource(workspaceGVR).Get(ctx, org, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("resolve workspace cluster ID for org %q: %w", org, err)
	}

	clusterID, found, err := unstructured.NestedString(obj.Object, "spec", "cluster")
	if err != nil || !found {
		return "", fmt.Errorf("workspace %q does not expose spec.cluster", org)
	}

	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return "", fmt.Errorf("workspace %q does not expose spec.cluster", org)
	}
	return clusterID, nil
}

func (r *SearchIndexResolver) listSearchIndices(ctx context.Context, orgClusterID string) (struct {
	Items []searchIndexResource `json:"items"`
}, error) {
	list := struct {
		Items []searchIndexResource `json:"items"`
	}{}

	listOpts := metav1.ListOptions{}
	if orgClusterID != "" {
		listOpts.LabelSelector = fmt.Sprintf("%s=%s", searchIndexOrgClusterIDLabel, orgClusterID)
	}

	objList, err := r.client.Resource(searchIndexGVR(r.cfg)).List(ctx, listOpts)
	if err != nil {
		return list, err
	}

	items := make([]searchIndexResource, 0, len(objList.Items))
	for _, item := range objList.Items {
		items = append(items, searchIndexResource{
			Metadata: struct {
				Name string `json:"name"`
			}{Name: item.GetName()},
			Spec: struct {
				OrganizationClusterID string `json:"organizationClusterID"`
			}{OrganizationClusterID: strings.TrimSpace(getNestedString(item.Object, "spec", "organizationClusterID"))},
			Status: struct {
				IndexName string `json:"indexName"`
			}{IndexName: strings.TrimSpace(getNestedString(item.Object, "status", "indexName"))},
		})
	}

	list.Items = items
	return list, nil
}

func configForKCPCluster(clusterName string, cfg *rest.Config) (*rest.Config, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	clusterCfg := rest.CopyConfig(cfg)
	clusterCfgURL, err := url.Parse(clusterCfg.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host URL: %w", err)
	}

	clusterCfgURL.Path = fmt.Sprintf("/clusters/%s", clusterName)
	clusterCfg.Host = clusterCfgURL.String()

	return clusterCfg, nil
}

func searchIndexGVR(cfg config.SearchIndexConfig) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    cfg.Group,
		Version:  cfg.Version,
		Resource: cfg.Resource,
	}
}

var workspaceGVR = schema.GroupVersionResource{
	Group:    "tenancy.kcp.io",
	Version:  "v1alpha1",
	Resource: "workspaces",
}

func getNestedString(obj map[string]interface{}, fields ...string) string {
	value, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return ""
	}
	return value
}

func selectSearchIndex(org, orgClusterID string, items []searchIndexResource) (searchIndexResource, error) {
	clusterMatches := make([]searchIndexResource, 0, 1)
	for _, item := range items {
		if strings.TrimSpace(item.Spec.OrganizationClusterID) == orgClusterID {
			clusterMatches = append(clusterMatches, item)
		}
	}
	if len(clusterMatches) == 1 {
		return clusterMatches[0], nil
	}
	if len(clusterMatches) > 1 {
		for _, item := range clusterMatches {
			if strings.TrimSpace(item.Metadata.Name) == org {
				return item, nil
			}
		}
		return searchIndexResource{}, fmt.Errorf("multiple SearchIndex resources match org cluster %q", orgClusterID)
	}

	for _, item := range items {
		if strings.TrimSpace(item.Metadata.Name) == org {
			return item, nil
		}
	}

	return searchIndexResource{}, fmt.Errorf("no SearchIndex matched org %q and org cluster %q", org, orgClusterID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
