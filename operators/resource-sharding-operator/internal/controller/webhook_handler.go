package controller

import (
	"context"
	"encoding/json"
	"net/http"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	admissionv1 "k8s.io/api/admission/v1"
)

type ShardAssignHandler struct {
	Registry *DynamicControllerRegistry
}

func (h *ShardAssignHandler) Handle(_ context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("webhook").WithValues("resource", req.Name, "namespace", req.Namespace)

	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}

	running := h.Registry.FindByGVR(req.Resource.Group, req.Resource.Version, req.Resource.Resource)
	if running == nil {
		return admission.Allowed("no matching ResourceSharding")
	}

	labels := extractLabels(req.Object.Raw)
	if _, exists := labels[running.LabelKey]; exists {
		return admission.Allowed("already labeled")
	}

	shard := running.Assigner.Next()
	if shard == "" {
		return admission.Allowed("no shards configured")
	}
	logger.Info("webhook assigning shard", "shard", shard)

	var patch []map[string]interface{}
	if labels == nil {
		patch = []map[string]interface{}{
			{
				"op":    "add",
				"path":  "/metadata/labels",
				"value": map[string]string{running.LabelKey: shard},
			},
		}
	} else {
		patch = []map[string]interface{}{
			{
				"op":    "add",
				"path":  "/metadata/labels/" + escapeJSONPointer(running.LabelKey),
				"value": shard,
			},
		}
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	resp := admission.Allowed("shard assigned")
	patchType := admissionv1.PatchTypeJSONPatch
	resp.PatchType = &patchType
	resp.Patch = patchBytes
	return resp
}

func extractLabels(raw []byte) map[string]string {
	var obj struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(raw, &obj)
	return obj.Metadata.Labels
}

func escapeJSONPointer(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case '~':
			result += "~0"
		case '/':
			result += "~1"
		default:
			result += string(c)
		}
	}
	return result
}
