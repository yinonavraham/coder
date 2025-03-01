package codersdk

type RBACResource string

const (
	ResourceWorkspace                   RBACResource = "workspace"
	ResourceWorkspaceProxy              RBACResource = "workspace_proxy"
	ResourceWorkspaceExecution          RBACResource = "workspace_execution"
	ResourceWorkspaceApplicationConnect RBACResource = "application_connect"
	ResourceAuditLog                    RBACResource = "audit_log"
	ResourceTemplate                    RBACResource = "template"
	ResourceGroup                       RBACResource = "group"
	ResourceFile                        RBACResource = "file"
	ResourceProvisionerDaemon           RBACResource = "provisioner_daemon"
	ResourceOrganization                RBACResource = "organization"
	ResourceRoleAssignment              RBACResource = "assign_role"
	ResourceOrgRoleAssignment           RBACResource = "assign_org_role"
	ResourceAPIKey                      RBACResource = "api_key"
	ResourceUser                        RBACResource = "user"
	ResourceUserData                    RBACResource = "user_data"
	ResourceOrganizationMember          RBACResource = "organization_member"
	ResourceLicense                     RBACResource = "license"
	ResourceDeploymentValues            RBACResource = "deployment_config"
	ResourceDeploymentStats             RBACResource = "deployment_stats"
	ResourceReplicas                    RBACResource = "replicas"
	ResourceDebugInfo                   RBACResource = "debug_info"
	ResourceSystem                      RBACResource = "system"
)

func (r RBACResource) String() string {
	return string(r)
}
