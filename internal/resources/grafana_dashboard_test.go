/*
Copyright 2026 Paperclip Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

func dashInstance() *hermesv1.HermesInstance {
	inst := minimalInstance()
	inst.Spec.Observability.Metrics.GrafanaDashboard = &hermesv1.GrafanaDashboardSpec{
		Enabled: Ptr(true),
	}
	return inst
}

func TestGrafanaDashboardEnabled(t *testing.T) {
	t.Parallel()
	assert.False(t, GrafanaDashboardEnabled(minimalInstance()), "absent block means disabled")

	off := minimalInstance()
	off.Spec.Observability.Metrics.GrafanaDashboard = &hermesv1.GrafanaDashboardSpec{Enabled: Ptr(false)}
	assert.False(t, GrafanaDashboardEnabled(off))

	assert.True(t, GrafanaDashboardEnabled(dashInstance()))
}

func TestBuildGrafanaDashboardOperator_Metadata(t *testing.T) {
	t.Parallel()
	cm := BuildGrafanaDashboardOperator(dashInstance())

	assert.Equal(t, "demo-dashboard-operator", cm.Name)
	assert.Equal(t, "agents", cm.Namespace)
	assert.Equal(t, "1", cm.Labels["grafana_dashboard"], "Grafana sidecar discovery label")
	assert.Equal(t, "Hermes", cm.Annotations["grafana_folder"], "default folder")

	body, ok := cm.Data["hermes-operator.json"]
	require.True(t, ok, "dashboard JSON key present")

	var dash map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &dash), "dashboard is valid JSON")
	assert.Equal(t, "hermes-operator-overview", dash["uid"])
}

func TestBuildGrafanaDashboardInstance_Metadata(t *testing.T) {
	t.Parallel()
	cm := BuildGrafanaDashboardInstance(dashInstance())

	assert.Equal(t, "demo-dashboard-instance", cm.Name)
	assert.Equal(t, "1", cm.Labels["grafana_dashboard"])

	body, ok := cm.Data["hermes-instance.json"]
	require.True(t, ok)

	var dash map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &dash))
	assert.Equal(t, "hermes-instance-detail", dash["uid"])
}

func TestBuildGrafanaDashboard_CustomFolderAndLabels(t *testing.T) {
	t.Parallel()
	inst := dashInstance()
	inst.Spec.Observability.Metrics.GrafanaDashboard.Folder = "Platform"
	inst.Spec.Observability.Metrics.GrafanaDashboard.Labels = map[string]string{"team": "agents"}

	cm := BuildGrafanaDashboardOperator(inst)
	assert.Equal(t, "Platform", cm.Annotations["grafana_folder"])
	assert.Equal(t, "agents", cm.Labels["team"])
	assert.Equal(t, "1", cm.Labels["grafana_dashboard"], "discovery label still present")
}
