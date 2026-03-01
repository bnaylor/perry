package agent

// Role identifies an agent's function in the pipeline.
type Role string

const (
	RoleStrategist    Role = "strategist"
	RoleResearcher    Role = "researcher"
	RoleCoder         Role = "coder"
	RoleAuditor       Role = "auditor_semantic"
	RoleShadowAuditor Role = "auditor_shadow"
)
