package manager

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func genValidIPv4(r *rand.Rand) string {
	a := r.Intn(256)
	b := r.Intn(256)
	c := r.Intn(256)
	d := r.Intn(256)
	return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
}

func genInvalidIP(r *rand.Rand) string {
	candidates := []string{
		"not-an-ip",
		"256.0.0.1",
		"1.2.3",
		"1.2.3.4.5",
		"abc",
		"",
		"999.999.999.999",
		"1.2.3.-1",
		"::gggg",
		"hostname.example.com",
	}
	return candidates[r.Intn(len(candidates))]
}

func genIPSet(r *rand.Rand) []string {
	count := r.Intn(6)
	seen := make(map[string]bool)
	var ips []string
	for i := 0; i < count; i++ {
		for attempt := 0; attempt < 20; attempt++ {
			ip := genValidIPv4(r)
			if !seen[ip] {
				seen[ip] = true
				ips = append(ips, ip)
				break
			}
		}
	}
	return ips
}

func genCommentText(r *rand.Rand) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 ._-"
	n := r.Intn(24)
	if n == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		b.WriteByte(alphabet[r.Intn(len(alphabet))])
	}
	return b.String()
}

func genResolvConf(r *rand.Rand) string {
	lineCount := r.Intn(11)
	lineKinds := []string{"nameserver", "comment", "empty", "other"}

	var lines []string
	for i := 0; i < lineCount; i++ {
		kind := lineKinds[r.Intn(len(lineKinds))]
		switch kind {
		case "nameserver":
			ip := genValidIPv4(r)
			lines = append(lines, "nameserver "+ip)
		case "comment":
			comment := genCommentText(r)
			lines = append(lines, "# "+comment)
		case "empty":
			lines = append(lines, "")
		case "other":
			directives := []string{
				"search example.com",
				"domain local",
				"options ndots:5",
				"options timeout:2",
				"sortlist 130.155.160.0/255.255.240.0",
			}
			idx := r.Intn(len(directives))
			lines = append(lines, directives[idx])
		}
	}

	if len(lines) == 0 {
		if r.Intn(2) == 0 {
			return "\n"
		}
		return ""
	}

	content := strings.Join(lines, "\n")
	if r.Intn(2) == 0 {
		content += "\n"
	}
	return content
}

func TestParse_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []line
		wantErr error
	}{
		{
			name:    "empty file",
			content: "",
			want:    []line{},
		},
		{
			name:    "single newline",
			content: "\n",
			want:    []line{},
		},
		{
			name:    "no trailing newline still parsed",
			content: "nameserver 1.1.1.1",
			want: []line{
				{kind: lineNameserverIP, raw: "nameserver 1.1.1.1", ip: "1.1.1.1"},
			},
		},
		{
			name:    "comment above nameserver",
			content: "# comment\nnameserver 8.8.8.8\n",
			want: []line{
				{kind: lineOther, raw: "# comment"},
				{kind: lineNameserverIP, raw: "nameserver 8.8.8.8", ip: "8.8.8.8"},
				{kind: lineOther, raw: ""},
			},
		},
		{
			name:    "invalid directive preserved",
			content: "branot namesspacer\n",
			want: []line{
				{kind: lineOther, raw: "branot namesspacer"},
				{kind: lineOther, raw: ""},
			},
		},
		{
			name:    "invalid nameserver ip treated as other",
			content: "nameserver not-an-ip\n",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "nameserver with inline comment treated as other",
			content: "nameserver 1.1.1.1 # cloudflare\n",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "mixed content preserved",
			content: "# c\noptions ndots:5\nnameserver 9.9.9.9\n\n",
			want: []line{
				{kind: lineOther, raw: "# c"},
				{kind: lineOther, raw: "options ndots:5"},
				{kind: lineNameserverIP, raw: "nameserver 9.9.9.9", ip: "9.9.9.9"},
				{kind: lineOther, raw: ""},
				{kind: lineOther, raw: ""},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parse(tt.content)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("parse(%q) expected error %v, got nil", tt.content, tt.wantErr)
				}
				if err != tt.wantErr {
					t.Fatalf("parse(%q) expected error %v, got %v", tt.content, tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse(%q) unexpected error: %v", tt.content, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parse(%q) mismatch:\nwant: %#v\ngot:  %#v", tt.content, tt.want, got)
			}
		})
	}
}

func TestParse_Table_WithGeneratedIPs(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		build func(r *rand.Rand) (string, []line, error)
	}

	tests := []testCase{
		{
			name: "generated valid nameserver ip is parsed",
			build: func(r *rand.Rand) (string, []line, error) {
				ip := genValidIPv4(r)
				content := "nameserver " + ip + "\n"
				want := []line{
					{kind: lineNameserverIP, raw: "nameserver " + ip, ip: ip},
					{kind: lineOther, raw: ""},
				}
				return content, want, nil
			},
		},
		{
			name: "generated invalid ip in nameserver returns error",
			build: func(r *rand.Rand) (string, []line, error) {
				ip := genInvalidIP(r)
				content := "nameserver " + ip + "\n"
				return content, nil, ErrInvalidConfig
			},
		},
		{
			name: "generated ip set with comments and invalid directive still parses",
			build: func(r *rand.Rand) (string, []line, error) {
				ips := genIPSet(r)
				invalidLine := "branot namesspacer"

				var contentLines []string
				var want []line

				contentLines = append(contentLines, "# comment before nameservers")
				want = append(want, line{kind: lineOther, raw: "# comment before nameservers"})

				contentLines = append(contentLines, "search example.com")
				want = append(want, line{kind: lineOther, raw: "search example.com"})

				contentLines = append(contentLines, invalidLine)
				want = append(want, line{kind: lineOther, raw: invalidLine})

				for _, ip := range ips {
					contentLines = append(contentLines, "nameserver "+ip)
					want = append(want, line{kind: lineNameserverIP, raw: "nameserver " + ip, ip: ip})
				}

				contentLines = append(contentLines, "")
				want = append(want, line{kind: lineOther, raw: ""})

				content := strings.Join(contentLines, "\n") + "\n"
				want = append(want, line{kind: lineOther, raw: ""})
				return content, want, nil
			},
		},
		{
			name: "no ip at all only comments and unknown directives",
			build: func(r *rand.Rand) (string, []line, error) {
				_ = genIPSet(r)
				content := "# only comments\n# and garbage\nbranot namesspacer\n\n"
				want := []line{
					{kind: lineOther, raw: "# only comments"},
					{kind: lineOther, raw: "# and garbage"},
					{kind: lineOther, raw: "branot namesspacer"},
					{kind: lineOther, raw: ""},
					{kind: lineOther, raw: ""},
				}
				return content, want, nil
			},
		},
		{
			name: "nameserver without ip returns error",
			build: func(r *rand.Rand) (string, []line, error) {
				content := "nameserver\n"
				return content, nil, ErrInvalidConfig
			},
		},
		{
			name: "nameserver with inline comment returns error",
			build: func(r *rand.Rand) (string, []line, error) {
				ip := genValidIPv4(r)
				content := "nameserver " + ip + " # comment\n"
				return content, nil, ErrInvalidConfig
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i := 0; i < 100; i++ {
				r := rand.New(rand.NewSource(int64(i + 1)))
				content, want, wantErr := tc.build(r)
				got, err := parse(content)
				if wantErr != nil {
					if err == nil {
						t.Fatalf("parse(%q) expected error %v, got nil", content, wantErr)
					}
					if err != wantErr {
						t.Fatalf("parse(%q) expected error %v, got %v", content, wantErr, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("parse(%q) unexpected error: %v", content, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("parse(%q) mismatch:\nwant: %#v\ngot:  %#v", content, want, got)
				}
			}
		})
	}
}

func TestProp_ParseFormatRoundTrip(t *testing.T) {
	t.Parallel()

	for i := 0; i < 500; i++ {
		r := rand.New(rand.NewSource(int64(i + 1)))
		content := genResolvConf(r)
		parsed1, err := parse(content)
		if err != nil {
			t.Fatalf("parse(content) unexpected error: %v, content=%q", err, content)
		}
		formatted := format(parsed1)
		parsed2, err := parse(formatted)
		if err != nil {
			t.Fatalf("parse(formatted) unexpected error: %v, formatted=%q", err, formatted)
		}
		if !reflect.DeepEqual(parsed1, parsed2) {
			t.Fatalf("round-trip mismatch:\ncontent:   %q\nparsed1:   %v\nformatted: %q\nparsed2:   %v",
				content, parsed1, formatted, parsed2)
		}
	}
}

func TestManager_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
		wantErr bool
	}{
		{
			name:    "empty file returns empty slice",
			content: "",
			want:    []string{},
		},
		{
			name:    "single nameserver",
			content: "nameserver 1.1.1.1\n",
			want:    []string{"1.1.1.1"},
		},
		{
			name:    "multiple nameservers",
			content: "nameserver 1.1.1.1\nnameserver 8.8.8.8\n",
			want:    []string{"1.1.1.1", "8.8.8.8"},
		},
		{
			name:    "nameservers with comments and other directives",
			content: "# comment\noptions ndots:5\nnameserver 9.9.9.9\n\nnameserver 8.8.4.4\n",
			want:    []string{"9.9.9.9", "8.8.4.4"},
		},
		{
			name:    "no nameserver lines returns empty slice",
			content: "# only comments\noptions ndots:5\n",
			want:    []string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp, err := os.CreateTemp(t.TempDir(), "resolv.conf")
			if err != nil {
				t.Fatalf("create temp file: %v", err)
			}
			if _, err := tmp.WriteString(tt.content); err != nil {
				t.Fatalf("write temp file: %v", err)
			}
			tmp.Close()

			m := New(tmp.Name(), slog.Default())
			got, err := m.List()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("List() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("List() mismatch:\nwant: %v\ngot:  %v", tt.want, got)
			}
		})
	}
}

func TestManager_ListWithNameserver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []Nameserver
		wantErr bool
	}{
		{
			name:    "empty file returns empty slice",
			content: "",
			want:    []Nameserver{},
		},
		{
			name:    "single nameserver",
			content: "nameserver 1.1.1.1\n",
			want: []Nameserver{
				{IP: "1.1.1.1", Line: "nameserver 1.1.1.1"},
			},
		},
		{
			name:    "multiple nameservers with extra lines",
			content: "# comment\nnameserver 1.1.1.1\noptions ndots:5\nnameserver 8.8.8.8\n",
			want: []Nameserver{
				{IP: "1.1.1.1", Line: "nameserver 1.1.1.1"},
				{IP: "8.8.8.8", Line: "nameserver 8.8.8.8"},
			},
		},
		{
			name:    "invalid nameserver line returns error",
			content: "nameserver not-an-ip\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp, err := os.CreateTemp(t.TempDir(), "resolv.conf")
			if err != nil {
				t.Fatalf("create temp file: %v", err)
			}
			if _, err := tmp.WriteString(tt.content); err != nil {
				t.Fatalf("write temp file: %v", err)
			}
			tmp.Close()

			m := New(tmp.Name(), slog.Default())
			got, err := m.ListNameserverIP()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ListWithNameserver() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ListWithNameserver() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ListWithNameserver() mismatch:\nwant: %#v\ngot:  %#v", tt.want, got)
			}
		})
	}
}

func TestManager_List_ReadError(t *testing.T) {
	t.Parallel()

	m := New("/nonexistent/path/resolv.conf", slog.Default())
	_, err := m.List()
	if err == nil {
		t.Fatal("List() expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "read resolv.conf") {
		t.Fatalf("List() error should contain 'read resolv.conf', got: %v", err)
	}
}

func TestProp_ListReturnsNameserverIPs(t *testing.T) {
	t.Parallel()

	genRapidIPv4 := rapid.Custom(func(t *rapid.T) string {
		a := rapid.IntRange(0, 255).Draw(t, "a")
		b := rapid.IntRange(0, 255).Draw(t, "b")
		c := rapid.IntRange(0, 255).Draw(t, "c")
		d := rapid.IntRange(0, 255).Draw(t, "d")
		return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
	})

	genRapidIPSet := rapid.Custom(func(t *rapid.T) []string {
		count := rapid.IntRange(0, 5).Draw(t, "count")
		seen := make(map[string]bool)
		var ips []string
		for i := 0; i < count; i++ {
			for attempt := 0; attempt < 20; attempt++ {
				ip := genRapidIPv4.Draw(t, fmt.Sprintf("ip_%d_%d", i, attempt))
				if !seen[ip] {
					seen[ip] = true
					ips = append(ips, ip)
					break
				}
			}
		}
		return ips
	})

	genNonNameserverLine := rapid.Custom(func(t *rapid.T) string {
		kind := rapid.IntRange(0, 2).Draw(t, "line_kind")
		switch kind {
		case 0:
			text := rapid.StringMatching(`[a-zA-Z0-9 ._-]{0,24}`).Draw(t, "comment_text")
			return "# " + text
		case 1:
			return ""
		default:
			directives := []string{
				"search example.com",
				"domain local",
				"options ndots:5",
				"options timeout:2",
				"sortlist 130.155.160.0/255.255.240.0",
			}
			idx := rapid.IntRange(0, len(directives)-1).Draw(t, "directive_idx")
			return directives[idx]
		}
	})

	dir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		nameserverIPs := genRapidIPSet.Draw(t, "nameserver_ips")

		type entry struct {
			isNameserver bool
			line         string
		}

		var entries []entry

		for _, ip := range nameserverIPs {
			entries = append(entries, entry{isNameserver: true, line: "nameserver " + ip})
		}

		extraCount := rapid.IntRange(0, 4).Draw(t, "extra_count")
		for i := 0; i < extraCount; i++ {
			l := genNonNameserverLine.Draw(t, fmt.Sprintf("extra_%d", i))
			entries = append(entries, entry{isNameserver: false, line: l})
		}

		shuffled := append([]entry(nil), entries...)
		for i := len(shuffled) - 1; i > 0; i-- {
			j := rapid.IntRange(0, i).Draw(t, fmt.Sprintf("shuffle_%d", i))
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		}

		var lines []string
		for _, e := range shuffled {
			lines = append(lines, e.line)
		}
		content := strings.Join(lines, "\n")
		if len(lines) > 0 {
			content += "\n"
		}

		tmp, err := os.CreateTemp(dir, "resolv.conf")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		if _, err := tmp.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		tmp.Close()

		m := New(tmp.Name(), slog.Default())
		got, err := m.List()
		if err != nil {
			t.Fatalf("List() unexpected error: %v (content=%q)", err, content)
		}

		if len(got) != len(nameserverIPs) {
			t.Fatalf("List() returned %d IPs, want %d; content=%q got=%v want=%v",
				len(got), len(nameserverIPs), content, got, nameserverIPs)
		}

		seen := make(map[string]bool, len(got))
		for _, ip := range got {
			if seen[ip] {
				t.Fatalf("List() returned duplicate IP %q; content=%q got=%v", ip, content, got)
			}
			seen[ip] = true
		}

		wantSet := make(map[string]bool, len(nameserverIPs))
		for _, ip := range nameserverIPs {
			wantSet[ip] = true
		}
		gotSet := make(map[string]bool, len(got))
		for _, ip := range got {
			gotSet[ip] = true
		}
		if !reflect.DeepEqual(gotSet, wantSet) {
			wantSorted := append([]string(nil), nameserverIPs...)
			ipsSorted := append([]string(nil), got...)
			sort.Strings(wantSorted)
			sort.Strings(ipsSorted)
			t.Fatalf("List() IP set mismatch:\nwant: %v\ngot:  %v\ncontent: %q",
				wantSorted, ipsSorted, content)
		}
	})
}

func TestProp_AddAppearsInList(t *testing.T) {
	t.Parallel()

	genRapidIPv4 := rapid.Custom(func(t *rapid.T) string {
		a := rapid.IntRange(0, 255).Draw(t, "a")
		b := rapid.IntRange(0, 255).Draw(t, "b")
		c := rapid.IntRange(0, 255).Draw(t, "c")
		d := rapid.IntRange(0, 255).Draw(t, "d")
		return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
	})

	rapid.Check(t, func(t *rapid.T) {
		existingCount := rapid.IntRange(0, 4).Draw(t, "existing_count")
		seen := make(map[string]bool)
		var existingIPs []string
		for i := 0; i < existingCount; i++ {
			for attempt := 0; attempt < 20; attempt++ {
				ip := genRapidIPv4.Draw(t, fmt.Sprintf("existing_ip_%d_%d", i, attempt))
				if !seen[ip] {
					seen[ip] = true
					existingIPs = append(existingIPs, ip)
					break
				}
			}
		}

		var newIP string
		for attempt := 0; attempt < 50; attempt++ {
			candidate := genRapidIPv4.Draw(t, fmt.Sprintf("new_ip_%d", attempt))
			if !seen[candidate] {
				newIP = candidate
				break
			}
		}
		if newIP == "" {
			t.Skip("could not generate a unique new IP")
		}

		var lines []string
		for _, ip := range existingIPs {
			lines = append(lines, "nameserver "+ip)
		}
		content := strings.Join(lines, "\n")
		if len(lines) > 0 {
			content += "\n"
		}

		tmp, err := os.CreateTemp("", "resolv.conf")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		tmp.Close()

		m := New(tmp.Name(), slog.Default())

		if err := m.Add(newIP); err != nil {
			t.Fatalf("Add(%q) unexpected error: %v", newIP, err)
		}

		got, err := m.List()
		if err != nil {
			t.Fatalf("List() unexpected error: %v", err)
		}

		found := false
		for _, ip := range got {
			if ip == newIP {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Add(%q) did not add IP to list; got=%v", newIP, got)
		}
	})
}

func TestProp_InvalidIPRejected(t *testing.T) {
	t.Parallel()

	genRapidInvalidIP := rapid.Custom(func(t *rapid.T) string {
		candidates := []string{
			"not-an-ip",
			"256.0.0.1",
			"1.2.3",
			"1.2.3.4.5",
			"abc",
			"",
			"999.999.999.999",
			"1.2.3.-1",
			"::gggg",
			"hostname.example.com",
			"300.300.300.300",
			"1.2.3.4.5.6",
			"foo bar",
		}
		idx := rapid.IntRange(0, len(candidates)-1).Draw(t, "invalid_ip_idx")
		return candidates[idx]
	})

	rapid.Check(t, func(t *rapid.T) {
		invalidIP := genRapidInvalidIP.Draw(t, "invalid_ip")

		content := "nameserver 1.1.1.1\n"
		tmp, err := os.CreateTemp("", "resolv.conf")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		tmp.Close()

		originalData, err := os.ReadFile(tmp.Name())
		if err != nil {
			t.Fatalf("read original file: %v", err)
		}

		m := New(tmp.Name(), slog.Default())

		err = m.Add(invalidIP)
		if !errors.Is(err, ErrInvalidIP) {
			t.Fatalf("Add(%q) expected ErrInvalidIP, got: %v", invalidIP, err)
		}

		afterData, err := os.ReadFile(tmp.Name())
		if err != nil {
			t.Fatalf("read file after Add: %v", err)
		}
		if string(originalData) != string(afterData) {
			t.Fatalf("Add(%q) with invalid IP modified the file:\nbefore: %q\nafter:  %q",
				invalidIP, string(originalData), string(afterData))
		}
	})
}

func TestProp_AddDuplicateReturnsAlreadyExists(t *testing.T) {
	t.Parallel()

	genRapidIPv4 := rapid.Custom(func(t *rapid.T) string {
		a := rapid.IntRange(0, 255).Draw(t, "a")
		b := rapid.IntRange(0, 255).Draw(t, "b")
		c := rapid.IntRange(0, 255).Draw(t, "c")
		d := rapid.IntRange(0, 255).Draw(t, "d")
		return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
	})

	rapid.Check(t, func(t *rapid.T) {
		existingCount := rapid.IntRange(1, 4).Draw(t, "existing_count")
		seen := make(map[string]bool)
		var existingIPs []string
		for i := 0; i < existingCount; i++ {
			for attempt := 0; attempt < 20; attempt++ {
				ip := genRapidIPv4.Draw(t, fmt.Sprintf("existing_ip_%d_%d", i, attempt))
				if !seen[ip] {
					seen[ip] = true
					existingIPs = append(existingIPs, ip)
					break
				}
			}
		}

		var lines []string
		for _, ip := range existingIPs {
			lines = append(lines, "nameserver "+ip)
		}
		content := strings.Join(lines, "\n") + "\n"

		tmp, err := os.CreateTemp("", "resolv.conf")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		tmp.Close()

		originalData, err := os.ReadFile(tmp.Name())
		if err != nil {
			t.Fatalf("read original file: %v", err)
		}

		dupIdx := rapid.IntRange(0, len(existingIPs)-1).Draw(t, "dup_idx")
		dupIP := existingIPs[dupIdx]

		m := New(tmp.Name(), slog.Default())

		err = m.Add(dupIP)
		if !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("Add(%q) expected ErrAlreadyExists, got: %v", dupIP, err)
		}

		afterData, err := os.ReadFile(tmp.Name())
		if err != nil {
			t.Fatalf("read file after duplicate Add: %v", err)
		}
		if string(originalData) != string(afterData) {
			t.Fatalf("Add(%q) duplicate modified the file:\nbefore: %q\nafter:  %q",
				dupIP, string(originalData), string(afterData))
		}
	})
}

func TestProp_OperationsPreserveOtherLines(t *testing.T) {
	t.Parallel()

	genRapidIPv4 := rapid.Custom(func(t *rapid.T) string {
		a := rapid.IntRange(0, 255).Draw(t, "a")
		b := rapid.IntRange(0, 255).Draw(t, "b")
		c := rapid.IntRange(0, 255).Draw(t, "c")
		d := rapid.IntRange(0, 255).Draw(t, "d")
		return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
	})

	genNonNameserverLine := rapid.Custom(func(t *rapid.T) string {
		kind := rapid.IntRange(0, 1).Draw(t, "line_kind")
		switch kind {
		case 0:
			text := rapid.StringMatching(`[a-zA-Z0-9 ._-]{0,24}`).Draw(t, "comment_text")
			return "# " + text
		default:
			directives := []string{
				"search example.com",
				"domain local",
				"options ndots:5",
				"options timeout:2",
				"sortlist 130.155.160.0/255.255.240.0",
			}
			idx := rapid.IntRange(0, len(directives)-1).Draw(t, "directive_idx")
			return directives[idx]
		}
	})

	rapid.Check(t, func(t *rapid.T) {
		existingCount := rapid.IntRange(0, 3).Draw(t, "existing_count")
		seen := make(map[string]bool)
		var existingIPs []string
		for i := 0; i < existingCount; i++ {
			for attempt := 0; attempt < 20; attempt++ {
				ip := genRapidIPv4.Draw(t, fmt.Sprintf("existing_ip_%d_%d", i, attempt))
				if !seen[ip] {
					seen[ip] = true
					existingIPs = append(existingIPs, ip)
					break
				}
			}
		}

		otherCount := rapid.IntRange(0, 4).Draw(t, "other_count")
		var otherLines []string
		for i := 0; i < otherCount; i++ {
			l := genNonNameserverLine.Draw(t, fmt.Sprintf("other_%d", i))
			otherLines = append(otherLines, l)
		}

		var allLines []string
		for _, ip := range existingIPs {
			allLines = append(allLines, "nameserver "+ip)
		}
		allLines = append(allLines, otherLines...)
		content := strings.Join(allLines, "\n")
		if len(allLines) > 0 {
			content += "\n"
		}

		tmp, err := os.CreateTemp("", "resolv.conf")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		tmp.Close()

		m := New(tmp.Name(), slog.Default())

		var newIP string
		for attempt := 0; attempt < 50; attempt++ {
			candidate := genRapidIPv4.Draw(t, fmt.Sprintf("new_ip_%d", attempt))
			if !seen[candidate] {
				newIP = candidate
				break
			}
		}
		if newIP == "" {
			t.Skip("could not generate a unique new IP")
		}

		if err := m.Add(newIP); err != nil {
			t.Fatalf("Add(%q) unexpected error: %v", newIP, err)
		}

		afterData, err := os.ReadFile(tmp.Name())
		if err != nil {
			t.Fatalf("read file after Add: %v", err)
		}
		afterLines, err := parse(string(afterData))
		if err != nil {
			t.Fatalf("parse file after Add: %v", err)
		}

		afterRaws := make(map[string]int)
		for _, l := range afterLines {
			afterRaws[l.raw]++
		}

		for _, ol := range otherLines {
			if afterRaws[ol] == 0 {
				t.Fatalf("Add(%q) removed other line %q; after content: %q",
					newIP, ol, string(afterData))
			}
			afterRaws[ol]--
		}

		afterIPSet := make(map[string]bool)
		for _, l := range afterLines {
			if l.kind == lineNameserverIP {
				afterIPSet[l.ip] = true
			}
		}
		for _, ip := range existingIPs {
			if !afterIPSet[ip] {
				t.Fatalf("Add(%q) removed existing nameserver %q; after content: %q",
					newIP, ip, string(afterData))
			}
		}
	})
}

func TestProp_RemoveDisappearsFromList(t *testing.T) {
	t.Parallel()

	genRapidIPv4 := rapid.Custom(func(t *rapid.T) string {
		a := rapid.IntRange(0, 255).Draw(t, "a")
		b := rapid.IntRange(0, 255).Draw(t, "b")
		c := rapid.IntRange(0, 255).Draw(t, "c")
		d := rapid.IntRange(0, 255).Draw(t, "d")
		return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
	})

	dir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(1, 5).Draw(t, "count")
		seen := make(map[string]bool)
		var ips []string
		for i := 0; i < count; i++ {
			for attempt := 0; attempt < 20; attempt++ {
				ip := genRapidIPv4.Draw(t, fmt.Sprintf("ip_%d_%d", i, attempt))
				if !seen[ip] {
					seen[ip] = true
					ips = append(ips, ip)
					break
				}
			}
		}

		var lines []string
		for _, ip := range ips {
			lines = append(lines, "nameserver "+ip)
		}
		content := strings.Join(lines, "\n") + "\n"

		tmp, err := os.CreateTemp(dir, "resolv.conf")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		if _, err := tmp.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		tmp.Close()

		// Pick one IP to remove
		removeIdx := rapid.IntRange(0, len(ips)-1).Draw(t, "remove_idx")
		removeIP := ips[removeIdx]

		m := New(tmp.Name(), slog.Default())

		if err := m.Remove(removeIP); err != nil {
			t.Fatalf("Remove(%q) unexpected error: %v", removeIP, err)
		}

		got, err := m.List()
		if err != nil {
			t.Fatalf("List() unexpected error: %v", err)
		}

		for _, ip := range got {
			if ip == removeIP {
				t.Fatalf("Remove(%q) did not remove IP from list; got=%v", removeIP, got)
			}
		}
	})
}

// Feature: dns-manager, Property 7: Remove несуществующего IP возвращает ErrNotFound
// Validates: Requirements 3.3
func TestProp_RemoveAbsentReturnsNotFound(t *testing.T) {
	t.Parallel()

	genRapidIPv4 := rapid.Custom(func(t *rapid.T) string {
		a := rapid.IntRange(0, 255).Draw(t, "a")
		b := rapid.IntRange(0, 255).Draw(t, "b")
		c := rapid.IntRange(0, 255).Draw(t, "c")
		d := rapid.IntRange(0, 255).Draw(t, "d")
		return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
	})

	dir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		// Generate existing IPs
		count := rapid.IntRange(0, 4).Draw(t, "count")
		seen := make(map[string]bool)
		var existingIPs []string
		for i := 0; i < count; i++ {
			for attempt := 0; attempt < 20; attempt++ {
				ip := genRapidIPv4.Draw(t, fmt.Sprintf("existing_%d_%d", i, attempt))
				if !seen[ip] {
					seen[ip] = true
					existingIPs = append(existingIPs, ip)
					break
				}
			}
		}

		// Generate an IP not in the file
		var absentIP string
		for attempt := 0; attempt < 50; attempt++ {
			candidate := genRapidIPv4.Draw(t, fmt.Sprintf("absent_%d", attempt))
			if !seen[candidate] {
				absentIP = candidate
				break
			}
		}
		if absentIP == "" {
			t.Skip("could not generate an absent IP")
		}

		var lines []string
		for _, ip := range existingIPs {
			lines = append(lines, "nameserver "+ip)
		}
		content := strings.Join(lines, "\n")
		if len(lines) > 0 {
			content += "\n"
		}

		tmp, err := os.CreateTemp(dir, "resolv.conf")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		if _, err := tmp.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		tmp.Close()

		originalData, err := os.ReadFile(tmp.Name())
		if err != nil {
			t.Fatalf("read original file: %v", err)
		}

		m := New(tmp.Name(), slog.Default())

		err = m.Remove(absentIP)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Remove(%q) expected ErrNotFound, got: %v", absentIP, err)
		}

		// File must be unchanged
		afterData, err := os.ReadFile(tmp.Name())
		if err != nil {
			t.Fatalf("read file after Remove: %v", err)
		}
		if string(originalData) != string(afterData) {
			t.Fatalf("Remove(%q) absent IP modified the file:\nbefore: %q\nafter:  %q",
				absentIP, string(originalData), string(afterData))
		}
	})
}
