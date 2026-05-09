package manager

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrAlreadyExists = errors.New("dns server already exists")
	ErrNotFound      = errors.New("dns server not found")
	ErrInvalidIP     = errors.New("invalid IP address")
	ErrInvalidConfig = errors.New("invalid resolv.conf content")
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

type Nameserver struct {
	IP   string
	Line string
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

func (m *Manager) ListNameserverIP() ([]Nameserver, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return nil, fmt.Errorf("read resolv.conf: %w", err)
	}

	lines, err := parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("read resolv.conf: %w", err)
	}

	servers := make([]Nameserver, 0)
	for _, line := range lines {
		if line.kind == lineNameserverIP {
			servers = append(servers, Nameserver{
				IP:   line.ip,
				Line: line.raw,
			})
		}
	}
	return servers, nil
}

func (m *Manager) List() ([]string, error) {
	servers, err := m.ListNameserverIP()
	if err != nil {
		return nil, err
	}

	ips := make([]string, 0, len(servers))
	for _, s := range servers {
		ips = append(ips, s.IP)
	}
	return ips, nil
}

func (m *Manager) Add(ip string) error {
	if net.ParseIP(ip) == nil {
		return ErrInvalidIP
	}

	data, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("read resolv.conf: %w", err)
	}

	raws, err := parse(string(data))
	if err != nil {
		return fmt.Errorf("parse resolv.conf: %w", err)
	}

	for _, raw := range raws {
		if raw.kind == lineNameserverIP && raw.ip == ip {
			return ErrAlreadyExists
		}
	}

	raw := line{
		kind: lineNameserverIP,
		raw:  "nameserver " + ip,
		ip:   ip,
	}

	raws = append(raws, raw)

	formatted := format(raws)
	if err := writeAtomic(m.path, formatted); err != nil {
		return err
	}

	m.logger.Debug("added nameserver", slog.String("ip", ip))
	return nil
}

func (m *Manager) Remove(ip string) error {
	if net.ParseIP(ip) == nil {
		return ErrInvalidIP
	}

	data, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("read resolv.conf: %w", err)
	}

	lines, err := parse(string(data))
	if err != nil {
		return fmt.Errorf("parse resolv.conf: %w", err)
	}

	found := false
	filtered := lines[:0:0]
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
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".resolv.conf.tmp*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err = tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("close temp file: %w", err)
	}

	if err = os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
