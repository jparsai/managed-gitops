package db

import (
	"strings"
	"unicode/utf8"
)

// These values should be equiv. with their related VARCHAR values from 'db-schema.sql' respectively
const (
	ClusterCredentialsClustercredentialsCredIDLength                        = 48
	ClusterCredentialsHostLength                                            = 512
	ClusterCredentialsKubeConfigLength                                      = 65000
	ClusterCredentialsKubeConfigContextLength                               = 64
	ClusterCredentialsServiceaccountBearerTokenLength                       = 128
	ClusterCredentialsServiceaccountNsLength                                = 128
	GitopsEngineClusterGitopsengineclusterIDLength                          = 48
	GitopsEngineInstanceGitopsengineinstanceIDLength                        = 48
	GitopsEngineInstanceNamespaceNameLength                                 = 48
	GitopsEngineInstanceNamespaceUIDLength                                  = 48
	GitopsEngineClusterClustercredentialsIDLength                           = 48
	GitopsEngineInstanceEngineclusterIDLength                               = 48
	ManagedEnvironmentManagedenvironmentIDLength                            = 48
	ManagedEnvironmentNameLength                                            = 256
	ManagedEnvironmentClustercredentialsIDLength                            = 48
	ClusterUserClusteruserIDLength                                          = 48
	ClusterUserUserNameLength                                               = 256
	ClusterAccessClusteraccessUserIDLength                                  = 48
	ClusterAccessClusteraccessManagedEnvironmentIDLength                    = 48
	ClusterAccessClusteraccessGitopsEngineInstanceIDLength                  = 48
	OperationOperationIDLength                                              = 48
	OperationInstanceIDLength                                               = 48
	OperationResourceIDLength                                               = 48
	OperationOperationOwnerUserIDLength                                     = 48
	OperationResourceTypeLength                                             = 32
	OperationStateLength                                                    = 30
	OperationHumanReadableStateLength                                       = 1024
	ApplicationApplicationIDLength                                          = 48
	ApplicationNameLength                                                   = 256
	ApplicationSpecFieldLength                                              = 16384
	ApplicationEngineInstanceInstIDLength                                   = 48
	ApplicationManagedEnvironmentIDLength                                   = 48
	ApplicationStateApplicationstateApplicationIDLength                     = 48
	ApplicationStateHealthLength                                            = 30
	ApplicationStateMessageLength                                           = 1024
	ApplicationStateRevisionLength                                          = 1024
	ApplicationStateSyncStatusLength                                        = 30
	DeploymentToApplicationMappingDeploymenttoapplicationmappingUIDIDLength = 48
	DeploymentToApplicationMappingNameLength                                = 256
	DeploymentToApplicationMappingNamespaceLength                           = 96
	DeploymentToApplicationMappingWorkspaceUIDLength                        = 48
	DeploymentToApplicationMappingApplicationIDLength                       = 48
	KubernetesToDBResourceMappingKubernetesResourceTypeLength               = 64
	KubernetesToDBResourceMappingKubernetesResourceUIDLength                = 64
	KubernetesToDBResourceMappingDbRelationTypeLength                       = 64
	KubernetesToDBResourceMappingDbRelationKeyLength                        = 64
	APICRToDatabaseMappingApiResourceTypeLength                             = 64
	APICRToDatabaseMappingApiResourceUIDLength                              = 64
	APICRToDatabaseMappingApiResourceNameLength                             = 256
	APICRToDatabaseMappingApiResourceNamespaceLength                        = 256
	APICRToDatabaseMappingApiResourceWorkspaceUIDLength                     = 64
	APICRToDatabaseMappingDbRelationTypeLength                              = 32
	APICRToDatabaseMappingDbRelationKeyLength                               = 64
	SyncOperationSyncoperationIDLength                                      = 48
	SyncOperationApplicationIDLength                                        = 48
	SyncOperationOperationIDLength                                          = 48
	SyncOperationDeploymentNameLength                                       = 256
	SyncOperationRevisionLength                                             = 256
	SyncOperationDesiredStateLength                                         = 16
)

// TruncateVarchar converts string to "str..." if chars is > maxLength
// returns a relative number of dots '.' string if maxLength <= 3
// returns empty string if maxLength < 0 or if string is not UTF-8 encoded
// Notice: This is based on characters -- not bytes (default VARCHAR behavior)
func TruncateVarchar(s string, maxLength int) string {
	if maxLength <= 3 && maxLength >= 0 {
		return strings.Repeat(".", maxLength)
	}

	if maxLength < 0 || !utf8.ValidString(s) {
		return ""
	}

	var wb = strings.Split(s, "")

	if maxLength < len(wb) {
		maxLength = maxLength - 3
		return strings.Join(wb[:maxLength], "") + "..."
	}

	return s
}

var dbFieldMap = map[string]int{
	"ClusterCredentialsClustercredentialsCredID":                        ClusterCredentialsClustercredentialsCredIDLength,
	"ClusterCredentialsHost":                                            ClusterCredentialsHostLength,
	"ClusterCredentialsKubeConfig":                                      ClusterCredentialsKubeConfigLength,
	"ClusterCredentialsKubeConfigContext":                               ClusterCredentialsKubeConfigContextLength,
	"ClusterCredentialsServiceaccountBearerToken":                       ClusterCredentialsServiceaccountBearerTokenLength,
	"ClusterCredentialsServiceaccountNs":                                ClusterCredentialsServiceaccountNsLength,
	"GitopsEngineClusterGitopsengineclusterID":                          GitopsEngineClusterGitopsengineclusterIDLength,
	"GitopsEngineInstanceGitopsengineinstanceID":                        GitopsEngineInstanceGitopsengineinstanceIDLength,
	"GitopsEngineInstanceNamespaceName":                                 GitopsEngineInstanceNamespaceNameLength,
	"GitopsEngineInstanceNamespaceUID":                                  GitopsEngineInstanceNamespaceUIDLength,
	"GitopsEngineClusterClustercredentialsID":                           GitopsEngineClusterClustercredentialsIDLength,
	"GitopsEngineInstanceEngineclusterID":                               GitopsEngineInstanceEngineclusterIDLength,
	"GitopsEngineInstanceEngineClusterID":                               GitopsEngineInstanceEngineclusterIDLength,
	"ManagedEnvironmentManagedenvironmentID":                            ManagedEnvironmentManagedenvironmentIDLength,
	"ManagedEnvironmentName":                                            ManagedEnvironmentNameLength,
	"ManagedEnvironmentClustercredentialsID":                            ManagedEnvironmentClustercredentialsIDLength,
	"ClusterUserClusteruserID":                                          ClusterUserClusteruserIDLength,
	"ClusterUserUserName":                                               ClusterUserUserNameLength,
	"ClusterAccessClusteraccessUserID":                                  ClusterAccessClusteraccessUserIDLength,
	"ClusterAccessClusteraccessManagedEnvironmentID":                    ClusterAccessClusteraccessManagedEnvironmentIDLength,
	"ClusterAccessClusteraccessGitopsEngineInstanceID":                  ClusterAccessClusteraccessGitopsEngineInstanceIDLength,
	"OperationOperationID":                                              OperationOperationIDLength,
	"OperationInstanceID":                                               OperationInstanceIDLength,
	"OperationResourceID":                                               OperationResourceIDLength,
	"OperationOperationOwnerUserID":                                     OperationOperationOwnerUserIDLength,
	"OperationResourceType":                                             OperationResourceTypeLength,
	"OperationState":                                                    OperationStateLength,
	"OperationHumanReadableState":                                       OperationHumanReadableStateLength,
	"ApplicationApplicationID":                                          ApplicationApplicationIDLength,
	"ApplicationName":                                                   ApplicationNameLength,
	"ApplicationSpecField":                                              ApplicationSpecFieldLength,
	"ApplicationEngineInstanceInstID":                                   ApplicationEngineInstanceInstIDLength,
	"ApplicationManagedEnvironmentID":                                   ApplicationManagedEnvironmentIDLength,
	"ApplicationStateApplicationstateApplicationID":                     ApplicationStateApplicationstateApplicationIDLength,
	"ApplicationStateHealth":                                            ApplicationStateHealthLength,
	"ApplicationStateMessage":                                           ApplicationStateMessageLength,
	"ApplicationStateRevision":                                          ApplicationStateRevisionLength,
	"ApplicationStateSyncStatus":                                        ApplicationStateSyncStatusLength,
	"DeploymentToApplicationMappingDeploymenttoapplicationmappingUIDID": DeploymentToApplicationMappingDeploymenttoapplicationmappingUIDIDLength,
	"DeploymentToApplicationMappingName":                                DeploymentToApplicationMappingNameLength,
	"DeploymentToApplicationMappingDeploymentName":                      DeploymentToApplicationMappingNameLength,
	"DeploymentToApplicationMappingNamespace":                           DeploymentToApplicationMappingNamespaceLength,
	"DeploymentToApplicationMappingDeploymentNamespace":                 DeploymentToApplicationMappingNamespaceLength,
	"DeploymentToApplicationMappingWorkspaceUID":                        DeploymentToApplicationMappingWorkspaceUIDLength,
	"DeploymentToApplicationMappingApplicationID":                       DeploymentToApplicationMappingApplicationIDLength,
	"KubernetesToDBResourceMappingKubernetesResourceType":               KubernetesToDBResourceMappingKubernetesResourceTypeLength,
	"KubernetesToDBResourceMappingKubernetesResourceUID":                KubernetesToDBResourceMappingKubernetesResourceUIDLength,
	"KubernetesToDBResourceMappingDbRelationType":                       KubernetesToDBResourceMappingDbRelationTypeLength,
	"KubernetesToDBResourceMappingDBRelationType":                       KubernetesToDBResourceMappingDbRelationTypeLength,
	"KubernetesToDBResourceMappingDbRelationKey":                        KubernetesToDBResourceMappingDbRelationKeyLength,
	"KubernetesToDBResourceMappingDBRelationKey":                        KubernetesToDBResourceMappingDbRelationKeyLength,
	"APICRToDatabaseMappingApiResourceType":                             APICRToDatabaseMappingApiResourceTypeLength,
	"APICRToDatabaseMappingAPIResourceType":                             APICRToDatabaseMappingApiResourceTypeLength,
	"APICRToDatabaseMappingApiResourceUID":                              APICRToDatabaseMappingApiResourceUIDLength,
	"APICRToDatabaseMappingAPIResourceUID":                              APICRToDatabaseMappingApiResourceUIDLength,
	"APICRToDatabaseMappingApiResourceName":                             APICRToDatabaseMappingApiResourceNameLength,
	"APICRToDatabaseMappingAPIResourceName":                             APICRToDatabaseMappingApiResourceNameLength,
	"APICRToDatabaseMappingApiResourceNamespace":                        APICRToDatabaseMappingApiResourceNamespaceLength,
	"APICRToDatabaseMappingAPIResourceNamespace":                        APICRToDatabaseMappingApiResourceNamespaceLength,
	"APICRToDatabaseMappingApiResourceWorkspaceUID":                     APICRToDatabaseMappingApiResourceWorkspaceUIDLength,
	"APICRToDatabaseMappingWorkspaceUID":                                APICRToDatabaseMappingApiResourceWorkspaceUIDLength,
	"APICRToDatabaseMappingDbRelationType":                              APICRToDatabaseMappingDbRelationTypeLength,
	"APICRToDatabaseMappingDBRelationType":                              APICRToDatabaseMappingDbRelationTypeLength,
	"APICRToDatabaseMappingDbRelationKey":                               APICRToDatabaseMappingDbRelationKeyLength,
	"APICRToDatabaseMappingDBRelationKey":                               APICRToDatabaseMappingDbRelationKeyLength,
	"SyncOperationSyncoperationID":                                      SyncOperationSyncoperationIDLength,
	"SyncOperationSyncOperationID":                                      SyncOperationSyncoperationIDLength,
	"SyncOperationApplicationID":                                        SyncOperationApplicationIDLength,
	"SyncOperationOperationID":                                          SyncOperationOperationIDLength,
	"SyncOperationDeploymentName":                                       SyncOperationDeploymentNameLength,
	"SyncOperationDeploymentNameField":                                  SyncOperationDeploymentNameLength,
	"SyncOperationRevision":                                             SyncOperationRevisionLength,
	"SyncOperationDesiredState":                                         SyncOperationDesiredStateLength,
}

func GetConstantValue(variable string) int {
	return dbFieldMap[variable]
}
