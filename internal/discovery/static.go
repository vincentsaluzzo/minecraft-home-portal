package discovery

import (
	"encoding/json"
	"fmt"
	"os"
)

type StaticFile struct {
	Servers []StaticServer `json:"servers"`
}

type StaticServer struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	ContainerName string      `json:"container_name"`
	ControlRef    string      `json:"control_ref"`
	Kind          string      `json:"kind"`
	Visibility    Visibility  `json:"visibility"`
	Actions       Actions     `json:"actions"`
	Connect       ConnectInfo `json:"connect"`
	RCON          RCONInfo    `json:"rcon"`
}

func LoadStaticServers(path string) ([]Server, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read static servers file: %w", err)
	}

	var file StaticFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse static servers file: %w", err)
	}

	servers := make([]Server, 0, len(file.Servers))
	for _, item := range file.Servers {
		server := Server{
			ID:            item.ID,
			Name:          item.Name,
			ContainerName: item.ContainerName,
			ControlRef:    item.ControlRef,
			Kind:          defaultString(item.Kind, "minecraft-java"),
			Visibility:    item.Visibility,
			Actions:       item.Actions,
			Connect:       item.Connect,
			RCON:          item.RCON,
			Status:        "static",
		}

		if server.ID == "" {
			server.ID = defaultString(server.ContainerName, server.Name)
		}
		if server.Visibility == "" {
			server.Visibility = VisibilityPrivate
		}
		if server.ControlRef == "" {
			server.ControlRef = defaultString(server.ContainerName, server.ID)
		}
		if server.Connect.Host == "" {
			server.Connect.Host = defaultString(server.ContainerName, server.Name)
		}
		if server.Connect.Port == "" {
			server.Connect.Port = "25565"
		}
		if server.RCON.Host == "" {
			server.RCON.Host = defaultString(server.ContainerName, server.Name)
		}
		if server.RCON.Port == "" {
			server.RCON.Port = "25575"
		}

		servers = append(servers, server)
	}

	return servers, nil
}
