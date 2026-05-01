package discovery

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

type Actions struct {
	Start   bool
	Stop    bool
	Restart bool
	Op      bool
	Deop    bool
	Say     bool
}

type ConnectInfo struct {
	Host    string
	Port    string
	Version string
	Notes   string
}

type RCONInfo struct {
	Host            string
	Port            string
	PasswordEnvName string
}

type Server struct {
	ID            string
	Name          string
	ContainerName string
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
