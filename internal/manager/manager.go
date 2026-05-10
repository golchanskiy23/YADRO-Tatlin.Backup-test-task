package manager

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrAlreadyExists = errors.New("dns server already exists")
	ErrNotFound      = errors.New("dns server not found")
	ErrInvalidIP     = errors.New("invalid IP address")
)

const (
	lineOther lineType = iota
	lineNameserverIP
)

type lineType int

type line struct {
	kind lineType
	raw  string
	ip   string
}

type Manager struct {
	mu     sync.RWMutex
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

func parse(content string) []line {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return []line{}
	}

	var lines []line
	for _, raw := range strings.Split(content, "\n") {
		fields := strings.Fields(raw)
		if len(fields) == 2 && fields[0] == "nameserver" {
			ipStr := fields[1]
			if idx := strings.IndexByte(ipStr, '%'); idx != -1 {
				ipStr = ipStr[:idx]
			}
			if net.ParseIP(ipStr) != nil {
				lines = append(lines, line{
					kind: lineNameserverIP,
					raw:  raw,
					ip:   fields[1],
				})
				continue
			}
		}
		lines = append(lines, line{kind: lineOther, raw: raw})
	}
	return lines
}

func format(lines []line) string {
	raws := make([]string, len(lines))
	for i, l := range lines {
		raws[i] = l.raw
	}
	return strings.Join(raws, "\n") + "\n"
}

func (m *Manager) listNameservers() ([]string, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return nil, fmt.Errorf("read resolv.conf: %w", err)
	}

	var ips []string
	for _, l := range parse(string(data)) {
		if l.kind == lineNameserverIP {
			ips = append(ips, l.ip)
		}
	}
	return ips, nil
}

func (m *Manager) List() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listNameservers()
}

// Warning: if "resolv.conf" contains more than 3 nameservers,
// glibc-based systems will only use the first 3.
func (m *Manager) Add(ip string) error {
	if net.ParseIP(ip) == nil {
		return ErrInvalidIP
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("read resolv.conf: %w", err)
	}

	lines := parse(string(data))

	for _, l := range lines {
		if l.kind == lineNameserverIP && l.ip == ip {
			return ErrAlreadyExists
		}
	}

	lines = append(lines, line{
		kind: lineNameserverIP,
		raw:  "nameserver " + ip,
		ip:   ip,
	})

	if err := writeAtomic(m.path, format(lines)); err != nil {
		return err
	}

	m.logger.Debug("added nameserver", slog.String("ip", ip))
	return nil
}

func (m *Manager) Remove(ip string) error {
	if net.ParseIP(ip) == nil {
		return ErrInvalidIP
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("read resolv.conf: %w", err)
	}

	lines := parse(string(data))

	filtered := make([]line, 0, len(lines))
	found := false
	for _, l := range lines {
		if l.kind == lineNameserverIP && l.ip == ip {
			found = true
			continue
		}
		filtered = append(filtered, l)
	}

	if !found {
		return ErrNotFound
	}

	if err := writeAtomic(m.path, format(filtered)); err != nil {
		return err
	}

	m.logger.Debug("removed nameserver", slog.String("ip", ip))
	return nil
}

func writeAtomic(path, content string) error {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		realPath = path
	}

	dir := filepath.Dir(realPath)
	tmp, err := os.CreateTemp(dir, ".resolv.conf.tmp*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err = tmp.WriteString(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err = os.Rename(tmp.Name(), realPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
