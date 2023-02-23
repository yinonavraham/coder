package builder

type KubeOptions struct {
	Os        string            `json:"os,omitempty"`
	Arch      string            `json:"arch,omitempty"`
	Namespace Variable          `json:"namespace,omitempty"`
	Image     string            `json:"image,omitempty"`
	Resources Resources         `json:"resources,omitempty"`
	Env       map[string]EnvVar `json:"env,omitempty"`
	HomePVC   bool              `json:"home_pvc,omitempty"`
}

type Variable struct {
	Value        string `json:"value,omitempty"`
	UserEditable bool   `json:"user_editable,omitempty"`
	Mutable      bool   `json:"mutable,omitempty"`
}

type Resources struct {
	CPU    Resource `json:"cpu,omitempty"`
	Memory Resource `json:"memory,omitempty"`
	Disk   Resource `json:"disk,omitempty"`
}

type Resource struct {
	Value        int  `json:"value,omitempty"`
	UserEditable bool `json:"user_editable,omitempty"`
	Min          int  `json:"min,omitempty"`
	Max          int  `json:"max,omitempty"`
}

type EnvSource string

const (
	EnvSourceString EnvSource = "string"
	EnvSourceSecret EnvSource = "secret"
)

type EnvVar struct {
	Source EnvSource `json:"source,omitempty"`

	String string `json:"string,omitempty"`

	SecretName string `json:"secret_name,omitempty"`
	SecretKey  string `json:"secret_key,omitempty"`
}
