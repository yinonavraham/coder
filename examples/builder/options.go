package quickstart

type KubeOptions struct {
	Os, Arch  string
	Namespace Variable
	Image     string
	Resources Resources
	Env       map[string]string
	HomePVC   bool
}

type Variable struct {
	Value        string
	UserEditable bool
	Mutable      bool
}

type Resources struct {
	CPU    Resource
	Memory Resource
	Disk   Resource
}

type Resource struct {
	Value        int
	UserEditable bool
	Min, Max     int
}

type EnvVar struct {
}
