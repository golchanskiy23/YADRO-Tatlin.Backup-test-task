package manager

import (
	"errors"
	"log/slog"
	"net"
	"strings"
)

var (
	ErrAlreadyExists = errors.New("dns server already exists")
	ErrNotFound      = errors.New("dns server not found")
	ErrInvalidIP     = errors.New("invalid IP address")
	ErrInvalidConfig = errors.New("invalid resolv.conf content")
)

const (
	lineOther      lineType = iota
	lineNameserverIP
)

type lineType int

type line struct {
	kind lineType
	raw  string
	ip   string
}

type Manager struct {
	path   string
	logger *slog.Logger
}

func New(path string, logger *slog.Logger) *Manager {
	if path == "" {
		path = "/etc/resolv.conf"
	}
	return &Manager{
		path:   path,
		logger: logger,
	}
}

func parse(content string) ([]line, error) {
	hasSuffix := strings.HasSuffix(content, "\n")
	if hasSuffix {
		content = content[:len(content)-1]
	}

	if content == "" {
		return []line{}, nil
	}

	var lines []line

	for _, raw := range strings.Split(content, "\n") {
		fields := strings.Fields(raw)
		if len(fields) > 0 && fields[0] == "nameserver" {
			if len(fields) != 2 || net.ParseIP(fields[1]) == nil {
				return nil, ErrInvalidConfig
			}
			lines = append(lines, line{
				kind: lineNameserverIP,
				raw:  raw,
				ip:   fields[1],
			})
			continue
		}

		lines = append(lines, line{kind: lineOther, raw: raw})
	}

	if hasSuffix {
		lines = append(lines, line{kind: lineOther, raw: ""})
	}

	return lines, nil
}

func format(lines []line) string {
	raws := make([]string, len(lines))
	for i, line := range lines {
		raws[i] = line.raw
	}
	return strings.Join(raws, "\n")
}