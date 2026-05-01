package discovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type DockerAPI interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error
	Close() error
}

type StatusProvider interface {
	Refresh(ctx context.Context, server Server) Server
}

type Service struct {
	namespace      string
	docker         DockerAPI
	statusProvider StatusProvider
	staticServers  []Server
	dockerEnabled  bool

	mu      sync.RWMutex
	servers map[string]Server
}

func New(namespace string, docker DockerAPI, statusProvider StatusProvider, dockerEnabled bool, staticServers []Server) *Service {
	return &Service{
		namespace:      namespace,
		docker:         docker,
		statusProvider: statusProvider,
		staticServers:  staticServers,
		dockerEnabled:  dockerEnabled,
		servers:        make(map[string]Server),
	}
}

func NewDockerClient(host string) (DockerAPI, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	} else {
		opts = append(opts, client.FromEnv)
	}
	return client.NewClientWithOpts(opts...)
}

func (s *Service) Refresh(ctx context.Context) error {
	next := make(map[string]Server)
	for _, server := range s.staticServers {
		next[server.ID] = s.refreshStatus(ctx, server)
	}

	var dockerErr error
	if s.dockerEnabled {
		if s.docker == nil {
			dockerErr = errors.New("docker client is not configured")
		} else {
			containers, err := s.docker.ContainerList(ctx, container.ListOptions{All: true})
			if err != nil {
				dockerErr = fmt.Errorf("list containers: %w", err)
			} else {
				for _, item := range containers {
					server, ok := s.fromContainer(item)
					if !ok {
						continue
					}

					next[server.ID] = s.refreshStatus(ctx, server)
				}
			}
		}
	}

	s.mu.Lock()
	s.servers = next
	s.mu.Unlock()

	return dockerErr
}

func (s *Service) All() []Server {
	s.mu.RLock()
	defer s.mu.RUnlock()

	servers := make([]Server, 0, len(s.servers))
	for _, server := range s.servers {
		servers = append(servers, server)
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	return servers
}

func (s *Service) ByID(id string) (Server, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	server, ok := s.servers[id]
	return server, ok
}

func (s *Service) Start(ctx context.Context, id string) error {
	if s.docker == nil {
		return errors.New("docker client is not configured")
	}
	server, ok := s.ByID(id)
	if !ok {
		return fmt.Errorf("server %q not found", id)
	}
	return s.docker.ContainerStart(ctx, server.controlRef(), container.StartOptions{})
}

func (s *Service) Stop(ctx context.Context, id string) error {
	if s.docker == nil {
		return errors.New("docker client is not configured")
	}
	server, ok := s.ByID(id)
	if !ok {
		return fmt.Errorf("server %q not found", id)
	}
	return s.docker.ContainerStop(ctx, server.controlRef(), container.StopOptions{})
}

func (s *Service) Restart(ctx context.Context, id string) error {
	if s.docker == nil {
		return errors.New("docker client is not configured")
	}
	server, ok := s.ByID(id)
	if !ok {
		return fmt.Errorf("server %q not found", id)
	}
	return s.docker.ContainerRestart(ctx, server.controlRef(), container.StopOptions{})
}

func (s *Service) fromContainer(item container.Summary) (Server, bool) {
	labels := item.Labels
	if labels[s.key("enabled")] != "true" {
		return Server{}, false
	}

	name := labels[s.key("name")]
	if name == "" {
		name = strings.TrimPrefix(firstName(item.Names), "/")
	}

	visibility := VisibilityPrivate
	switch labels[s.key("visibility")] {
	case string(VisibilityPublic):
		visibility = VisibilityPublic
	case string(VisibilityPrivate), "":
		visibility = VisibilityPrivate
	}

	server := Server{
		ID:            item.ID,
		Name:          name,
		ContainerName: strings.TrimPrefix(firstName(item.Names), "/"),
		ControlRef:    item.ID,
		Kind:          defaultString(labels[s.key("kind")], "minecraft-java"),
		Visibility:    visibility,
		Running:       item.State == "running",
		Status:        item.Status,
		Connect: ConnectInfo{
			Host:    labels[s.key("connect.host")],
			Port:    labels[s.key("connect.port")],
			Version: labels[s.key("connect.version")],
			Notes:   labels[s.key("connect.notes")],
		},
		RCON: RCONInfo{
			Host:            labels[s.key("rcon.host")],
			Port:            labels[s.key("rcon.port")],
			PasswordEnvName: labels[s.key("rcon.password-env")],
		},
		Actions: Actions{
			Start:   boolLabel(labels, s.key("actions.start"), true),
			Stop:    boolLabel(labels, s.key("actions.stop"), true),
			Restart: boolLabel(labels, s.key("actions.restart"), true),
			Op:      boolLabel(labels, s.key("actions.op"), true),
			Deop:    boolLabel(labels, s.key("actions.deop"), true),
			Say:     boolLabel(labels, s.key("actions.say"), true),
		},
	}

	if server.Connect.Host == "" {
		server.Connect.Host = server.ContainerName
	}

	if server.RCON.Host == "" {
		server.RCON.Host = server.ContainerName
	}

	if server.Connect.Port == "" {
		server.Connect.Port = "25565"
	}

	if server.RCON.Port == "" {
		server.RCON.Port = "25575"
	}

	if envName := server.RCON.PasswordEnvName; envName != "" && os.Getenv(envName) == "" {
		server.LastError = fmt.Sprintf("missing environment variable %s for RCON password", envName)
	}

	return server, true
}

func (s *Service) key(suffix string) string {
	return s.namespace + "." + suffix
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func defaultString(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func boolLabel(labels map[string]string, key string, fallback bool) bool {
	value, ok := labels[key]
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func (s *Service) Close() error {
	if s == nil || s.docker == nil {
		return nil
	}
	return s.docker.Close()
}

func (s *Service) refreshStatus(ctx context.Context, server Server) Server {
	if s.statusProvider != nil {
		return s.statusProvider.Refresh(ctx, server)
	}
	return server
}

func (s Server) controlRef() string {
	if s.ControlRef != "" {
		return s.ControlRef
	}
	if s.ContainerName != "" {
		return s.ContainerName
	}
	return s.ID
}
