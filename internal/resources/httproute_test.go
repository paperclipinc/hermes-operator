package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

func TestBuildHTTPRoute_NilWhenUnset(t *testing.T) {
	t.Parallel()
	inst := &hermesv1.HermesInstance{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"}}
	assert.Nil(t, BuildHTTPRoute(inst), "no HTTPRoute spec means no route")
}

func TestBuildHTTPRoute_Basics(t *testing.T) {
	t.Parallel()
	ns := "gateways"
	section := "https"
	inst := &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"},
		Spec: hermesv1.HermesInstanceSpec{
			Networking: hermesv1.NetworkingSpec{
				HTTPRoute: &hermesv1.HTTPRouteSpec{
					Enabled:   Ptr(true),
					Hostnames: []string{"agent.example.com"},
					ParentRefs: []hermesv1.HTTPRouteParentRef{
						{Name: "public-gw", Namespace: &ns, SectionName: &section},
					},
					Annotations: map[string]string{"team": "platform"},
				},
			},
		},
	}

	route := BuildHTTPRoute(inst)
	require.NotNil(t, route)
	assert.Equal(t, "gateway.networking.k8s.io/v1", route.GetAPIVersion())
	assert.Equal(t, "HTTPRoute", route.GetKind())
	assert.Equal(t, "demo", route.GetName())
	assert.Equal(t, "agents", route.GetNamespace())
	assert.Equal(t, "platform", route.GetAnnotations()["team"])

	spec, _, _ := getNestedMap(route.Object, "spec")

	parents := spec["parentRefs"].([]interface{})
	require.Len(t, parents, 1)
	pr := parents[0].(map[string]interface{})
	assert.Equal(t, "public-gw", pr["name"])
	assert.Equal(t, "gateways", pr["namespace"])
	assert.Equal(t, "https", pr["sectionName"])

	hostnames := spec["hostnames"].([]interface{})
	assert.Equal(t, "agent.example.com", hostnames[0])

	rules := spec["rules"].([]interface{})
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})

	match := rule["matches"].([]interface{})[0].(map[string]interface{})
	path := match["path"].(map[string]interface{})
	assert.Equal(t, "PathPrefix", path["type"])
	assert.Equal(t, "/", path["value"])

	backend := rule["backendRefs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "demo", backend["name"])
	assert.Equal(t, int64(GatewayPort), backend["port"], "backendRefs target the Service port by number")
}

func TestBuildHTTPRoute_CustomPathAndPort(t *testing.T) {
	t.Parallel()
	inst := &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"},
		Spec: hermesv1.HermesInstanceSpec{
			Networking: hermesv1.NetworkingSpec{
				Service: hermesv1.ServiceSpec{
					Ports: []hermesv1.NamedServicePort{{Name: "web", Port: 9000}},
				},
				HTTPRoute: &hermesv1.HTTPRouteSpec{
					Enabled:         Ptr(true),
					Path:            "/api",
					ServicePortName: "web",
				},
			},
		},
	}
	route := BuildHTTPRoute(inst)
	require.NotNil(t, route)
	spec, _, _ := getNestedMap(route.Object, "spec")
	rule := spec["rules"].([]interface{})[0].(map[string]interface{})
	path := rule["matches"].([]interface{})[0].(map[string]interface{})["path"].(map[string]interface{})
	assert.Equal(t, "/api", path["value"])
	backend := rule["backendRefs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, int64(9000), backend["port"], "named port resolves to its Service port number")
}

func TestHTTPRouteName(t *testing.T) {
	t.Parallel()
	inst := &hermesv1.HermesInstance{ObjectMeta: metav1.ObjectMeta{Name: "demo"}}
	assert.Equal(t, "demo", HTTPRouteName(inst))
}
