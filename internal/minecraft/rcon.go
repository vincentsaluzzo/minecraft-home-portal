package minecraft

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorcon/rcon"
	"github.com/vsaluzzo/minecraft-home-portal/internal/discovery"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Refresh(ctx context.Context, server discovery.Server) discovery.Server {
	if !server.Running || server.RCON.PasswordEnvName == "" {
		return server
	}

	response, err := c.Execute(ctx, server, "list")
	if err != nil {
		server.LastError = err.Error()
		return server
	}

	server.PlayerCount, server.PlayerNames = parsePlayerList(response)
	return server
}

func (c *Client) Execute(ctx context.Context, server discovery.Server, command string) (string, error) {
	password := os.Getenv(server.RCON.PasswordEnvName)
	if password == "" {
		return "", fmt.Errorf("missing RCON password in env %s", server.RCON.PasswordEnvName)
	}

	address := server.RCON.Host + ":" + server.RCON.Port

	timeout := 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = maxDuration(1*time.Second, time.Until(deadline))
	}

	conn, err := rcon.Dial(address, password, rcon.SetDialTimeout(timeout), rcon.SetDeadline(timeout))
	if err != nil {
		return "", fmt.Errorf("connect to RCON %s: %w", address, err)
	}
	defer conn.Close()

	response, err := conn.Execute(command)
	if err != nil {
		return "", fmt.Errorf("execute RCON command: %w", err)
	}

	return response, nil
}

var playerListPattern = regexp.MustCompile(`There are (\d+) of a max of \d+ players online:?\s*(.*)$`)

func parsePlayerList(response string) (int, []string) {
	matches := playerListPattern.FindStringSubmatch(strings.TrimSpace(response))
	if len(matches) != 3 {
		return 0, nil
	}

	count, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, nil
	}

	rawNames := strings.TrimSpace(matches[2])
	if rawNames == "" {
		return count, nil
	}

	parts := strings.Split(rawNames, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			names = append(names, name)
		}
	}

	return count, names
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
