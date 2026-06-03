package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

// HTTPRouteGVK is the Gateway API GroupVersionKind we emit. We build it as an
// unstructured object to avoid taking a dependency on sigs.k8s.io/gateway-api;
// the CRDs must be installed in the cluster for the route to take effect.
func HTTPRouteGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}
}

// HTTPRouteName returns the deterministic HTTPRoute name.
func HTTPRouteName(inst *hermesv1.HermesInstance) string { return inst.Name }

// servicePortNumber resolves a Service port name to its port number, mirroring
// buildServicePorts. Falls back to GatewayPort when the name is not found.
func servicePortNumber(inst *hermesv1.HermesInstance, name string) int32 {
	for _, p := range inst.Spec.Networking.Service.Ports {
		if p.Name == name {
			return p.Port
		}
	}
	if name == MetricsPortName {
		port := inst.Spec.Observability.Metrics.Port
		if port == 0 {
			return DefaultMetricsPort
		}
		return port
	}
	return GatewayPort
}

// BuildHTTPRoute constructs the desired Gateway API HTTPRoute as an unstructured
// object. It mirrors the Ingress builder: a single prefix rule routing to the
// agent Service. Returns nil when no HTTPRoute is requested.
func BuildHTTPRoute(inst *hermesv1.HermesInstance) *unstructured.Unstructured {
	spec := inst.Spec.Networking.HTTPRoute
	if spec == nil {
		return nil
	}

	path := spec.Path
	if path == "" {
		path = "/"
	}
	portName := spec.ServicePortName
	if portName == "" {
		portName = GatewayPortName
	}
	// Gateway API backendRefs target a Service port by number, so resolve the
	// requested named port to the port number emitted on the Service.
	port := servicePortNumber(inst, portName)

	parentRefs := make([]interface{}, 0, len(spec.ParentRefs))
	for _, ref := range spec.ParentRefs {
		pr := map[string]interface{}{
			"name": ref.Name,
		}
		if ref.Namespace != nil {
			pr["namespace"] = *ref.Namespace
		}
		if ref.SectionName != nil {
			pr["sectionName"] = *ref.SectionName
		}
		parentRefs = append(parentRefs, pr)
	}

	hostnames := make([]interface{}, 0, len(spec.Hostnames))
	for _, h := range spec.Hostnames {
		hostnames = append(hostnames, h)
	}

	metadata := map[string]interface{}{
		"name":      HTTPRouteName(inst),
		"namespace": inst.Namespace,
		"labels":    toIface(LabelsForInstance(inst)),
	}
	if len(spec.Annotations) > 0 {
		metadata["annotations"] = toIface(spec.Annotations)
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": HTTPRouteGVK().GroupVersion().String(),
			"kind":       HTTPRouteGVK().Kind,
			"metadata":   metadata,
			"spec": map[string]interface{}{
				"parentRefs": parentRefs,
				"hostnames":  hostnames,
				"rules": []interface{}{
					map[string]interface{}{
						"matches": []interface{}{
							map[string]interface{}{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": path,
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": ServiceName(inst),
								"port": int64(port),
							},
						},
					},
				},
			},
		},
	}
}
