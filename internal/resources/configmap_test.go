package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	hermesv1 "github.com/stubbi/hermes-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildConfigMap_EmptyConfig(t *testing.T) {
	t.Parallel()
	inst := &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"},
	}
	cm := BuildConfigMap(inst, "")
	assert.Equal(t, "demo-config", cm.Name)
	assert.Equal(t, "{}\n", cm.Data["config.yaml"])
}

func TestBuildConfigMap_RawBody(t *testing.T) {
	t.Parallel()
	inst := &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"},
		Spec: hermesv1.HermesInstanceSpec{
			Config: hermesv1.ConfigSpec{
				Raw: &hermesv1.RawConfig{RawExtension: runtime.RawExtension{Raw: []byte(`{"telegram":{"enabled":true}}`)}},
			},
		},
	}
	cm := BuildConfigMap(inst, "")
	body := cm.Data["config.yaml"]
	assert.Contains(t, body, "telegram:")
	assert.Contains(t, body, "enabled: true")
}

func TestBuildConfigMap_RefOnly_PassesResolvedBody(t *testing.T) {
	t.Parallel()
	inst := &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"},
	}
	resolved := "discord:\n  enabled: true\n"
	cm := BuildConfigMap(inst, resolved)
	assert.Equal(t, resolved, cm.Data["config.yaml"])
}

func TestBuildConfigMap_MergeMode(t *testing.T) {
	t.Parallel()
	inst := &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"},
		Spec: hermesv1.HermesInstanceSpec{
			Config: hermesv1.ConfigSpec{
				Raw:       &hermesv1.RawConfig{RawExtension: runtime.RawExtension{Raw: []byte(`{"telegram":{"enabled":true}}`)}},
				MergeMode: hermesv1.ConfigMergeModeMerge,
			},
		},
	}
	cm := BuildConfigMap(inst, "discord:\n  enabled: true\ntelegram:\n  enabled: true\n")
	assert.Contains(t, cm.Data["config.yaml"], "discord:")
	assert.Contains(t, cm.Data["config.yaml"], "telegram:")
}

func TestMergeYAMLBodies(t *testing.T) {
	t.Parallel()
	base := "discord:\n  enabled: true\n"
	overlay := `{"telegram":{"enabled":true},"discord":{"enabled":false}}`
	got, err := MergeYAMLBodies(base, overlay)
	assert.NoError(t, err)
	assert.Contains(t, got, "telegram:")
	assert.Contains(t, got, "enabled: false")
}
