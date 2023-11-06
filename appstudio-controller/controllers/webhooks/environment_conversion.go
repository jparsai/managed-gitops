//
// Copyright 2023 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhooks

import (
	"fmt"

	appstudiov1beta1 "github.com/redhat-appstudio/application-api/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

type MyHub interface {
	runtime.Object
	Hub()
}

// Hub marks this type as a conversion hub.
func (*EnvironmentWebhook) MyHub() {}

// ConvertTo converts this Memcached to the Hub version (vbeta1).
func (src *EnvironmentWebhook) ConvertTo(dstRaw MyHub) error {

	fmt.Println("###############")
	fmt.Println("ConvertTo")
	fmt.Println("###############")

	// fetch v1beta1 version from Hub, converted values will be set in this object
	dst := dstRaw.(*EnvironmentWebhook)

	// copy ObjectMeta from v1alpha1 to v1beta1 version
	dst.EnvironmentV2.ObjectMeta = src.EnvironmentV1.Environment.ObjectMeta

	// copy Spec fields from v1alpha1 to v1beta1 version
	dst.EnvironmentV2.Spec = appstudiov1beta1.EnvironmentSpec{
		DisplayName:        src.EnvironmentV1.Spec.DisplayName,
		DeploymentStrategy: appstudiov1beta1.DeploymentStrategyType(src.EnvironmentV1.Spec.DeploymentStrategy),
		ParentEnvironment:  src.EnvironmentV1.Spec.ParentEnvironment,
		Tags:               src.EnvironmentV1.Spec.Tags,
	}

	// if v1alpha1 version has src.Spec.Configuration.Env field then copy it to v1beta1
	if src.EnvironmentV1.Spec.Configuration.Env != nil {
		dst.EnvironmentV2.Spec.Configuration.Env = []appstudiov1beta1.EnvVarPair{}

		for _, env := range src.EnvironmentV1.Spec.Configuration.Env {
			dst.EnvironmentV2.Spec.Configuration.Env = append(dst.EnvironmentV2.Spec.Configuration.Env, appstudiov1beta1.EnvVarPair(env))
		}
	}

	// if v1alpha1 version has Spec.Configuration.Target field then copy it to v1beta1
	if src.EnvironmentV1.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName != "" {
		// This filed is renamed and moved to Target in v1beta1
		dst.EnvironmentV2.Spec.Target = &appstudiov1beta1.TargetConfiguration{
			Claim: appstudiov1beta1.TargetClaim{
				DeploymentTargetClaim: appstudiov1beta1.DeploymentTargetClaimConfig{
					ClaimName: src.EnvironmentV1.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName,
				},
			},
		}
	}

	// if v1alpha1 has Spec.UnstableConfigurationFields field then copy it to v1beta1
	if src.EnvironmentV1.Spec.UnstableConfigurationFields != nil {

		if dst.EnvironmentV2.Spec.Target == nil {
			dst.EnvironmentV2.Spec.Target = &appstudiov1beta1.TargetConfiguration{}
		}

		dst.EnvironmentV2.Spec.Target.ClusterType = appstudiov1beta1.ConfigurationClusterType(string(src.Spec.UnstableConfigurationFields.ClusterType))

		dst.EnvironmentV2.Spec.Target.KubernetesClusterCredentials = appstudiov1beta1.KubernetesClusterCredentials{
			TargetNamespace:            src.EnvironmentV1.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.TargetNamespace,
			APIURL:                     src.EnvironmentV1.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.APIURL,
			IngressDomain:              src.EnvironmentV1.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.IngressDomain,
			ClusterCredentialsSecret:   src.EnvironmentV1.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.ClusterCredentialsSecret,
			AllowInsecureSkipTLSVerify: src.EnvironmentV1.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify,
			Namespaces:                 src.EnvironmentV1.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.Namespaces,
			ClusterResources:           src.EnvironmentV1.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.ClusterResources,
		}
	}

	// copy Status from v1alpha1 to v1beta1 version
	dst.EnvironmentV2.Status = appstudiov1beta1.EnvironmentStatus(src.EnvironmentV1.Status)

	return nil
}

// ConvertFrom converts from the Hub version (vbeta1) to this version.
func (dst *EnvironmentWebhook) ConvertFrom(srcRaw MyHub) error {

	fmt.Println("###############")
	fmt.Println("ConvertFrom")
	fmt.Println("###############")

	return nil
}
