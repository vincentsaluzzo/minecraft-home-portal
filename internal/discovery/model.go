package discovery

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

type Actions struct {
	Start   bool `json:"start"`
	Stop    bool `json:"stop"`
	Restart bool `json:"restart"`
	Op      bool `json:"op"`
	Deop    bool `json:"deop"`
	Say     bool `json:"say"`
}

type ConnectInfo struct {
	Host    string `json:"host"`
	Port    string `json:"port"`
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

type RCONInfo struct {
	Host            string `json:"host"`
	Port            string `json:"port"`
	PasswordEnvName string `json:"password_env_name"`
}

type Server struct {
	ID            string
	Name          string
	ContainerName string
	ControlRef    string
	Kind          string
	Visibility    Visibility
	Actions       Actions
	Connect       ConnectInfo
	RCON          RCONInfo
	Running       bool
	Status        string
	PlayerCount   int
	PlayerNames   []string
	LastError     string
}
