package nifi

import (
	"bytes"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/imdario/mergo"
	"github.com/orangeopensource/nifi-operator/pkg/apis/nifi/v1alpha1"
	"github.com/orangeopensource/nifi-operator/pkg/resources/templates"
	"github.com/orangeopensource/nifi-operator/pkg/resources/templates/config"
	"github.com/orangeopensource/nifi-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sort"
	"strings"
	"text/template"
)

func (r *Reconciler) configMap(id int32, nodeConfig *v1alpha1.NodeConfig, log logr.Logger) runtime.Object {
	return &corev1.ConfigMap{
		ObjectMeta: templates.ObjectMeta(
			fmt.Sprintf(templates.NodeConfigTemplate+"-%d", r.NifiCluster.Name, id),
			util.MergeLabels(
				labelsForNifi(r.NifiCluster.Name),
				map[string]string{"nodeId": fmt.Sprintf("%d", id)},
			),
			r.NifiCluster,
		),
		Data: map[string]string{
			"nifi.properties": r.generateNifiPropertiesNodeConfig(id, nodeConfig, log),
			"zookeeper.properties": r.generateZookeeperPropertiesNodeConfig(id, nodeConfig, log),
			"state-management.xml": r.getStateManagementConfigString(nodeConfig, id, log),
			"login-identity-providers.xml": "",
//			"logback.xml": "",
//			"bootstrap.conf": "",
//			"bootstrap-notification-servces.xml": "",
//			"authorizers.xml": "",
		},
	}
}

////////////////////////////////////
//  Nifi properties configuration //
////////////////////////////////////

//
func (r Reconciler) generateNifiPropertiesNodeConfig(id int32, nodeConfig *v1alpha1.NodeConfig, log logr.Logger) string {
	var parsedReadOnlyClusterConfig map[string]string

	if &r.NifiCluster.Spec.ReadOnlyConfig != nil && &r.NifiCluster.Spec.ReadOnlyConfig.NifiProperties != nil {
		parsedReadOnlyClusterConfig = util.ParsePropertiesFormat(r.NifiCluster.Spec.ReadOnlyConfig.NifiProperties.OverrideConfigs)
	}

	var parsedReadOnlyNodeConfig = map[string]string{}

	for _, node := range r.NifiCluster.Spec.Nodes {
		if node.Id == id && node.ReadOnlyConfig != nil && &node.ReadOnlyConfig.NifiProperties != nil {
			parsedReadOnlyNodeConfig = util.ParsePropertiesFormat(node.ReadOnlyConfig.NifiProperties.OverrideConfigs)
			break
		}
	}

	if err := mergo.Merge(&parsedReadOnlyNodeConfig, parsedReadOnlyClusterConfig); err != nil {
		log.Error(err, "error occurred during merging readonly configs")
	}

	//Generate the Complete Configuration for the Node
	completeConfigMap := map[string]string{}

	if err := mergo.Merge(&completeConfigMap, util.ParsePropertiesFormat(r.getNifiPropertiesConfigString(nodeConfig, id, log))); err != nil {
		log.Error(err, "error occurred during merging operator generated configs")
	}

	if err := mergo.Merge(&completeConfigMap, parsedReadOnlyNodeConfig); err != nil {
		log.Error(err, "error occurred during merging readOnly config to complete configs")
	}

	completeConfig := []string{}

	for key, value := range completeConfigMap {
		completeConfig = append(completeConfig, fmt.Sprintf("%s=%s", key, value))
	}

	// We need to sort the config every time to avoid diffs occurred because of ranging through map
	sort.Strings(completeConfig)

	return strings.Join(completeConfig, "\n")
}

//
func (r *Reconciler) getNifiPropertiesConfigString(nConfig *v1alpha1.NodeConfig, id int32, log logr.Logger) string {
	base := r.NifiCluster.Spec.ReadOnlyConfig.NifiProperties.DeepCopy()
	for _, node := range r.NifiCluster.Spec.Nodes {
		if node.Id == id && node.ReadOnlyConfig != nil && &node.ReadOnlyConfig.NifiProperties != nil{
			mergo.Merge(base, node.ReadOnlyConfig.NifiProperties, mergo.WithOverride)
		}
	}

	var out bytes.Buffer
	t := template.Must(template.New("nConfig-config").Parse(config.NifiPropertiesTemplate))
	if err := t.Execute(&out, map[string]interface{}{
		"NifiCluster":				r.NifiCluster,
		"Id": 						id,
		"ListenerConfig":			config.GenerateListenerSpecificConfig(&r.NifiCluster.Spec.ListenersConfig, id, r.NifiCluster.Namespace, r.NifiCluster.Name, r.NifiCluster.Spec.HeadlessServiceEnabled, log),
		"ProvenanceStorage":		config.GenerateProvenanceStorageConfig(nConfig.StorageConfigs),
		"SiteToSiteSecure": 		base.SiteToSiteSecure,
		"ClusterSecure":			base.ClusterSecure,
		"WebProxyHost": 			base.WebProxyHost,
		"NeedClientAuth": 			base.NeedClientAuth,
		"Authorizer": 				base.GetAuthorizer(),
		//
		"LdapConfiguration": 		r.NifiCluster.Spec.LdapConfiguration,
		"IsNode": 					nConfig.GetIsNode(),
		"ZookeeperConnectString":	r.NifiCluster.Spec.ZKAddresse,
		"ZookeeperPath": 			r.NifiCluster.Spec.GetZkPath(),
	}); err != nil {
		log.Error(err, "error occurred during parsing the config template")
	}
	return out.String()
}



/////////////////////////////////////////
//  Zookeeper properties configuration //
/////////////////////////////////////////

//
func (r Reconciler) generateZookeeperPropertiesNodeConfig(id int32, nodeConfig *v1alpha1.NodeConfig, log logr.Logger) string {
	var parsedReadOnlyClusterConfig map[string]string

	if &r.NifiCluster.Spec.ReadOnlyConfig != nil && &r.NifiCluster.Spec.ReadOnlyConfig.ZookeeperProperties != nil {
		parsedReadOnlyClusterConfig = util.ParsePropertiesFormat(r.NifiCluster.Spec.ReadOnlyConfig.ZookeeperProperties.OverrideConfigs)
	}

	var parsedReadOnlyNodeConfig = map[string]string{}

	for _, node := range r.NifiCluster.Spec.Nodes {
		if node.Id == id && node.ReadOnlyConfig != nil && &node.ReadOnlyConfig.ZookeeperProperties != nil {
			parsedReadOnlyNodeConfig = util.ParsePropertiesFormat(node.ReadOnlyConfig.ZookeeperProperties.OverrideConfigs)
			break
		}
	}

	if err := mergo.Merge(&parsedReadOnlyNodeConfig, parsedReadOnlyClusterConfig); err != nil {
		log.Error(err, "error occurred during merging readonly configs")
	}

	//Generate the Complete Configuration for the Node
	completeConfigMap := map[string]string{}

	if err := mergo.Merge(&completeConfigMap, util.ParsePropertiesFormat(r.getZookeeperPropertiesConfigString(nodeConfig, id, log))); err != nil {
		log.Error(err, "error occurred during merging operator generated configs")
	}

	if err := mergo.Merge(&completeConfigMap, parsedReadOnlyNodeConfig); err != nil {
		log.Error(err, "error occurred during merging readOnly config to complete configs")
	}

	completeConfig := []string{}

	for key, value := range completeConfigMap {
		completeConfig = append(completeConfig, fmt.Sprintf("%s=%s", key, value))
	}

	// We need to sort the config every time to avoid diffs occurred because of ranging through map
	sort.Strings(completeConfig)

	return strings.Join(completeConfig, "\n")
}

//
func (r *Reconciler) getZookeeperPropertiesConfigString(nConfig *v1alpha1.NodeConfig, id int32, log logr.Logger) string {

	base := r.NifiCluster.Spec.ReadOnlyConfig.ZookeeperProperties.DeepCopy()
	for _, node := range r.NifiCluster.Spec.Nodes {
		if node.Id == id && node.ReadOnlyConfig != nil && &node.ReadOnlyConfig.ZookeeperProperties != nil{
			mergo.Merge(base, node.ReadOnlyConfig.ZookeeperProperties, mergo.WithOverride)
		}
	}

	var out bytes.Buffer
	t := template.Must(template.New("nConfig-config").Parse(config.ZookeeperPropertiesTemplate))
	if err := t.Execute(&out, map[string]interface{}{
	}); err != nil {
		log.Error(err, "error occurred during parsing the config template")
	}
	return out.String()
}

//
func (r *Reconciler) getStateManagementConfigString(nConfig *v1alpha1.NodeConfig, id int32, log logr.Logger) string {

	var out bytes.Buffer
	t := template.Must(template.New("nConfig-config").Parse(config.StateManagementTemplate))
	if err := t.Execute(&out, map[string]interface{}{
		"NifiCluster":				r.NifiCluster,
		"Id": 						id,
		"ZookeeperConnectString":	r.NifiCluster.Spec.ZKAddresse,
		"ZookeeperPath": 			r.NifiCluster.Spec.GetZkPath(),
	}); err != nil {
		log.Error(err, "error occurred during parsing the config template")
	}
	return out.String()
}