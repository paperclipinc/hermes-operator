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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

const defaultGrafanaFolder = "Hermes"

// GrafanaDashboardEnabled reports whether Grafana dashboard ConfigMaps should
// be emitted for this instance.
func GrafanaDashboardEnabled(inst *hermesv1.HermesInstance) bool {
	gd := inst.Spec.Observability.Metrics.GrafanaDashboard
	return gd != nil && BoolValue(gd.Enabled)
}

// GrafanaDashboardOperatorName returns the name of the operator overview dashboard ConfigMap.
func GrafanaDashboardOperatorName(inst *hermesv1.HermesInstance) string {
	return inst.Name + "-dashboard-operator"
}

// GrafanaDashboardInstanceName returns the name of the per-instance dashboard ConfigMap.
func GrafanaDashboardInstanceName(inst *hermesv1.HermesInstance) string {
	return inst.Name + "-dashboard-instance"
}

// BuildGrafanaDashboardOperator creates a ConfigMap containing the operator overview dashboard.
func BuildGrafanaDashboardOperator(inst *hermesv1.HermesInstance) *corev1.ConfigMap {
	return buildDashboardConfigMap(inst, GrafanaDashboardOperatorName(inst), "hermes-operator.json", buildOperatorDashboard())
}

// BuildGrafanaDashboardInstance creates a ConfigMap containing the per-instance dashboard.
func BuildGrafanaDashboardInstance(inst *hermesv1.HermesInstance) *corev1.ConfigMap {
	return buildDashboardConfigMap(inst, GrafanaDashboardInstanceName(inst), "hermes-instance.json", buildInstanceDashboard())
}

func buildDashboardConfigMap(inst *hermesv1.HermesInstance, name, dataKey, dashboardJSON string) *corev1.ConfigMap {
	labels := LabelsForInstance(inst)
	labels["grafana_dashboard"] = "1"

	folder := defaultGrafanaFolder
	if gd := inst.Spec.Observability.Metrics.GrafanaDashboard; gd != nil {
		if gd.Folder != "" {
			folder = gd.Folder
		}
		for k, v := range gd.Labels {
			labels[k] = v
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: inst.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"grafana_folder": folder,
			},
		},
		Data: map[string]string{
			dataKey: dashboardJSON,
		},
	}
}

// --- Dashboard JSON model ---

type grafanaDashboard struct {
	Annotations   grafanaAnnotations `json:"annotations"`
	Editable      bool               `json:"editable"`
	GraphTooltip  int                `json:"graphTooltip"`
	Panels        []grafanaPanel     `json:"panels"`
	SchemaVersion int                `json:"schemaVersion"`
	Tags          []string           `json:"tags"`
	Templating    grafanaTemplating  `json:"templating"`
	Time          grafanaTime        `json:"time"`
	Refresh       string             `json:"refresh"`
	Title         string             `json:"title"`
	UID           string             `json:"uid"`
}

type grafanaAnnotations struct {
	List []interface{} `json:"list"`
}

type grafanaTemplating struct {
	List []grafanaVariable `json:"list"`
}

type grafanaVariable struct {
	Current    map[string]interface{} `json:"current"`
	Hide       int                    `json:"hide"`
	IncludeAll bool                   `json:"includeAll"`
	Label      string                 `json:"label"`
	Multi      bool                   `json:"multi"`
	Name       string                 `json:"name"`
	Options    []interface{}          `json:"options"`
	Query      interface{}            `json:"query"`
	Refresh    int                    `json:"refresh"`
	Regex      string                 `json:"regex"`
	Type       string                 `json:"type"`
	Datasource interface{}            `json:"datasource,omitempty"`
	Definition string                 `json:"definition,omitempty"`
	Sort       int                    `json:"sort,omitempty"`
	AllValue   string                 `json:"allValue,omitempty"`
}

type grafanaTime struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type grafanaPanel struct {
	ID          int                    `json:"id"`
	Title       string                 `json:"title"`
	Type        string                 `json:"type"`
	GridPos     grafanaGridPos         `json:"gridPos"`
	Targets     []grafanaTarget        `json:"targets,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
	FieldConfig *grafanaFieldConfig    `json:"fieldConfig,omitempty"`
	Datasource  *grafanaDatasource     `json:"datasource,omitempty"`
	Panels      []grafanaPanel         `json:"panels,omitempty"`
	Collapsed   bool                   `json:"collapsed,omitempty"`
}

type grafanaGridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

type grafanaTarget struct {
	Expr         string `json:"expr"`
	LegendFormat string `json:"legendFormat"`
	RefID        string `json:"refId"`
	Instant      bool   `json:"instant,omitempty"`
	Format       string `json:"format,omitempty"`
}

type grafanaFieldConfig struct {
	Defaults  map[string]interface{}   `json:"defaults"`
	Overrides []map[string]interface{} `json:"overrides,omitempty"`
}

type grafanaDatasource struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

func dsVar() *grafanaDatasource {
	return &grafanaDatasource{Type: "prometheus", UID: "${datasource}"}
}

func datasourceVar() grafanaVariable {
	return grafanaVariable{
		Current: map[string]interface{}{},
		Hide:    0,
		Label:   "Datasource",
		Name:    "datasource",
		Options: []interface{}{},
		Query:   "prometheus",
		Refresh: 1,
		Type:    "datasource",
	}
}

func namespaceVar(multi bool) grafanaVariable {
	return grafanaVariable{
		Current:    map[string]interface{}{},
		Hide:       0,
		IncludeAll: multi,
		Label:      "Namespace",
		Multi:      multi,
		Name:       "namespace",
		Options:    []interface{}{},
		Query:      `label_values(kube_pod_labels{label_app_kubernetes_io_name="hermes-agent"}, namespace)`,
		Definition: `label_values(kube_pod_labels{label_app_kubernetes_io_name="hermes-agent"}, namespace)`,
		Refresh:    2,
		Type:       "query",
		Sort:       1,
		Datasource: map[string]interface{}{"type": "prometheus", "uid": "${datasource}"},
	}
}

func instanceVar(multi bool) grafanaVariable {
	v := grafanaVariable{
		Current:    map[string]interface{}{},
		Hide:       0,
		IncludeAll: multi,
		Label:      "Instance",
		Multi:      multi,
		Name:       "instance",
		Options:    []interface{}{},
		Query:      `label_values(kube_pod_labels{namespace=~"$namespace",label_app_kubernetes_io_name="hermes-agent"}, label_app_kubernetes_io_instance)`,
		Definition: `label_values(kube_pod_labels{namespace=~"$namespace",label_app_kubernetes_io_name="hermes-agent"}, label_app_kubernetes_io_instance)`,
		Refresh:    2,
		Type:       "query",
		Sort:       1,
		Datasource: map[string]interface{}{"type": "prometheus", "uid": "${datasource}"},
	}
	if multi {
		v.AllValue = ".*"
	}
	return v
}

func statPanel(id int, title, expr string, pos grafanaGridPos) grafanaPanel {
	return grafanaPanel{
		ID:          id,
		Title:       title,
		Type:        "stat",
		GridPos:     pos,
		Targets:     []grafanaTarget{{Expr: expr, RefID: "A", Instant: true}},
		Datasource:  dsVar(),
		FieldConfig: &grafanaFieldConfig{Defaults: map[string]interface{}{}},
	}
}

func timeseriesPanel(id int, title string, targets []grafanaTarget, pos grafanaGridPos) grafanaPanel {
	return grafanaPanel{
		ID:         id,
		Title:      title,
		Type:       "timeseries",
		GridPos:    pos,
		Targets:    targets,
		Datasource: dsVar(),
		FieldConfig: &grafanaFieldConfig{Defaults: map[string]interface{}{
			"custom": map[string]interface{}{
				"lineWidth":   1,
				"fillOpacity": 10,
				"pointSize":   5,
				"showPoints":  "auto",
			},
		}},
	}
}

func gaugePanel(id int, title, expr string, pos grafanaGridPos) grafanaPanel {
	return grafanaPanel{
		ID:         id,
		Title:      title,
		Type:       "gauge",
		GridPos:    pos,
		Targets:    []grafanaTarget{{Expr: expr, RefID: "A", Instant: true}},
		Datasource: dsVar(),
		FieldConfig: &grafanaFieldConfig{Defaults: map[string]interface{}{
			"max": 1,
			"min": 0,
			"thresholds": map[string]interface{}{
				"steps": []map[string]interface{}{
					{"color": "green", "value": nil},
					{"color": "yellow", "value": 0.8},
					{"color": "red", "value": 0.9},
				},
			},
			"unit": "percentunit",
		}},
	}
}

func rowPanel(id int, title string, y int, collapsed bool, panels []grafanaPanel) grafanaPanel {
	return grafanaPanel{
		ID:        id,
		Title:     title,
		Type:      "row",
		GridPos:   grafanaGridPos{H: 1, W: 24, X: 0, Y: y},
		Collapsed: collapsed,
		Panels:    panels,
	}
}

func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		// This should never happen with our static dashboard structures.
		panic(err)
	}
	return string(b)
}

// --- Operator overview dashboard ---

func buildOperatorDashboard() string {
	dashboard := grafanaDashboard{
		Annotations:   grafanaAnnotations{List: []interface{}{}},
		Editable:      true,
		GraphTooltip:  1,
		SchemaVersion: 39,
		Tags:          []string{"hermes", "operator"},
		Time:          grafanaTime{From: "now-1h", To: "now"},
		Refresh:       "30s",
		Title:         "Hermes Operator",
		UID:           "hermes-operator-overview",
		Templating: grafanaTemplating{
			List: []grafanaVariable{datasourceVar(), namespaceVar(true), instanceVar(true)},
		},
		Panels: buildOperatorPanels(),
	}
	return mustMarshalJSON(dashboard)
}

func buildOperatorPanels() []grafanaPanel {
	gp := func(h, w, x, y int) grafanaGridPos { return grafanaGridPos{H: h, W: w, X: x, Y: y} }

	podSel := `kube_pod_labels{namespace=~"$namespace",label_app_kubernetes_io_name="hermes-agent",label_app_kubernetes_io_instance=~"$instance"}`

	return []grafanaPanel{
		rowPanel(100, "Fleet Overview", 0, false, nil),
		statPanel(1, "Managed Instances", `count(`+podSel+`)`, gp(4, 8, 0, 1)),
		statPanel(2, "Pods Running",
			`count(kube_pod_status_phase{namespace=~"$namespace",phase="Running",pod=~"$instance-.*"})`, gp(4, 8, 8, 1)),
		statPanel(3, "Container Restarts (1h)",
			`sum(increase(kube_pod_container_status_restarts_total{namespace=~"$namespace",pod=~"$instance-.*",container="hermes"}[1h]))`,
			gp(4, 8, 16, 1)),

		rowPanel(101, "SelfConfigure", 5, false, nil),
		timeseriesPanel(4, "SelfConfig Applied",
			[]grafanaTarget{
				{Expr: `sum(rate(hermes_selfconfig_applied_total{namespace=~"$namespace"}[5m])) by (namespace)`, LegendFormat: "{{ namespace }}", RefID: "A"},
			}, gp(8, 12, 0, 6)),
		timeseriesPanel(5, "SelfConfig Denied",
			[]grafanaTarget{
				{Expr: `sum(rate(hermes_selfconfig_denied_total{namespace=~"$namespace"}[5m])) by (namespace)`, LegendFormat: "{{ namespace }}", RefID: "A"},
			}, gp(8, 12, 12, 6)),

		rowPanel(102, "Fleet Resource Usage", 14, false, nil),
		timeseriesPanel(6, "CPU Usage by Instance",
			[]grafanaTarget{
				{Expr: `sum(rate(container_cpu_usage_seconds_total{namespace=~"$namespace",pod=~"$instance-.*",container="hermes"}[5m])) by (pod)`, LegendFormat: "{{ pod }}", RefID: "A"},
			}, gp(8, 12, 0, 15)),
		timeseriesPanel(7, "Memory Working Set by Instance",
			[]grafanaTarget{
				{Expr: `sum(container_memory_working_set_bytes{namespace=~"$namespace",pod=~"$instance-.*",container="hermes"}) by (pod)`, LegendFormat: "{{ pod }}", RefID: "A"},
			}, gp(8, 12, 12, 15)),
	}
}

// --- Per-instance dashboard ---

func buildInstanceDashboard() string {
	dashboard := grafanaDashboard{
		Annotations:   grafanaAnnotations{List: []interface{}{}},
		Editable:      true,
		GraphTooltip:  1,
		SchemaVersion: 39,
		Tags:          []string{"hermes", "instance"},
		Time:          grafanaTime{From: "now-1h", To: "now"},
		Refresh:       "30s",
		Title:         "Hermes Instance",
		UID:           "hermes-instance-detail",
		Templating: grafanaTemplating{
			List: []grafanaVariable{datasourceVar(), namespaceVar(false), instanceVar(false)},
		},
		Panels: buildInstancePanels(),
	}
	return mustMarshalJSON(dashboard)
}

func buildInstancePanels() []grafanaPanel {
	gp := func(h, w, x, y int) grafanaGridPos { return grafanaGridPos{H: h, W: w, X: x, Y: y} }

	return []grafanaPanel{
		rowPanel(200, "Health", 0, false, nil),
		statPanel(21, "Pods Running",
			`count(kube_pod_status_phase{namespace="$namespace",phase="Running",pod=~"$instance-.*"})`, gp(4, 6, 0, 1)),
		gaugePanel(22, "CPU %",
			`sum(rate(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$instance-.*",container="hermes"}[5m])) / sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="hermes",resource="cpu"})`,
			gp(4, 6, 6, 1)),
		gaugePanel(23, "Memory %",
			`sum(container_memory_working_set_bytes{namespace="$namespace",pod=~"$instance-.*",container="hermes"}) / sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="hermes",resource="memory"})`,
			gp(4, 6, 12, 1)),
		gaugePanel(24, "PVC %",
			`kubelet_volume_stats_used_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"} / kubelet_volume_stats_capacity_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"}`,
			gp(4, 6, 18, 1)),

		rowPanel(201, "CPU and Memory", 5, false, nil),
		timeseriesPanel(25, "CPU Usage vs Request/Limit",
			[]grafanaTarget{
				{Expr: `sum(rate(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$instance-.*",container="hermes"}[5m]))`, LegendFormat: "usage", RefID: "A"},
				{Expr: `sum(kube_pod_container_resource_requests{namespace="$namespace",pod=~"$instance-.*",container="hermes",resource="cpu"})`, LegendFormat: "request", RefID: "B"},
				{Expr: `sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="hermes",resource="cpu"})`, LegendFormat: "limit", RefID: "C"},
			}, gp(8, 12, 0, 6)),
		timeseriesPanel(26, "Memory Working Set vs Limit",
			[]grafanaTarget{
				{Expr: `sum(container_memory_working_set_bytes{namespace="$namespace",pod=~"$instance-.*",container="hermes"})`, LegendFormat: "working set", RefID: "A"},
				{Expr: `sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="hermes",resource="memory"})`, LegendFormat: "limit", RefID: "B"},
			}, gp(8, 12, 12, 6)),

		rowPanel(202, "Pod Health", 14, false, nil),
		timeseriesPanel(27, "Container Restarts",
			[]grafanaTarget{
				{Expr: `sum(kube_pod_container_status_restarts_total{namespace="$namespace",pod=~"$instance-.*"}) by (container)`, LegendFormat: "{{ container }}", RefID: "A"},
			}, gp(8, 12, 0, 15)),
		timeseriesPanel(28, "OOM Kills",
			[]grafanaTarget{
				{Expr: `sum(kube_pod_container_status_last_terminated_reason{namespace="$namespace",pod=~"$instance-.*",container="hermes",reason="OOMKilled"})`, LegendFormat: "OOM killed", RefID: "A"},
			}, gp(8, 12, 12, 15)),

		rowPanel(203, "Storage", 23, true, []grafanaPanel{
			timeseriesPanel(29, "PVC Usage",
				[]grafanaTarget{
					{Expr: `kubelet_volume_stats_used_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"}`, LegendFormat: "used", RefID: "A"},
					{Expr: `kubelet_volume_stats_capacity_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"}`, LegendFormat: "capacity", RefID: "B"},
				}, gp(8, 24, 0, 24)),
		}),
	}
}
