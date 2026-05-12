package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	hermesv1 "github.com/stubbi/hermes-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildConfigMap_HasConfigKey(t *testing.T) {
	inst := &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "agents"},
	}
	cm := BuildConfigMap(inst)
	assert.Equal(t, "demo-config", cm.Name)
	assert.Contains(t, cm.Data, "config.yaml")
	assert.Equal(t, "{}\n", cm.Data["config.yaml"], "minimal config is an empty YAML mapping")
}
